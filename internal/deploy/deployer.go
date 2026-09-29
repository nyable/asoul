package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"asoul/internal/catalog"
	"asoul/internal/fsx"
	"asoul/internal/hash"
	"asoul/internal/model"
	"asoul/internal/skill"
	"asoul/internal/state"
	"github.com/gofrs/flock"
)

// Deployer handles distributing skills from managed store to deployment targets.
type Deployer struct {
	stateManager *state.Manager
}

// TargetConflictError represents a deployment conflict where the destination
// directory already contains a skill that either lacks a deployment record (collision)
// or has been modified since the last deployment.
type TargetConflictError struct {
	SkillID     string
	Target      string
	Format      string
	TargetDir   string
	RelDest     string
	IsUntracked bool
	IsProject   bool
}

func (e *TargetConflictError) Error() string {
	if e.IsProject {
		if e.IsUntracked {
			return fmt.Sprintf("collision: %s already exists in project format %q (%s) without deployment record. Use --force to overwrite", e.SkillID, e.Format, e.RelDest)
		}
		return fmt.Sprintf("target %s in project format %q has been modified since last deployment. Use --force to overwrite", e.SkillID, e.Format)
	}
	if e.IsUntracked {
		return fmt.Sprintf("collision: %s already exists in global %q without deployment record. Use --force to overwrite", e.SkillID, e.Target)
	}
	return fmt.Sprintf("target %s in global %q has been modified since last deployment. Use --force to overwrite", e.SkillID, e.Target)
}

// NewDeployer creates a new Deployer.
func NewDeployer(stateManager *state.Manager) *Deployer {
	return &Deployer{stateManager: stateManager}
}

// DetectProjectFormats probes the project directory for known agent format folders.
func DetectProjectFormats(projectRoot string) []string {
	var detected []string
	for _, fDef := range model.BuiltinFormats {
		for _, probe := range fDef.ProbeDirs {
			probePath := filepath.Join(projectRoot, probe)
			if fsx.DirExists(probePath) {
				detected = append(detected, fDef.ID)
				break
			}
		}
	}
	return detected
}

// ResolveProjectFormats resolves requested formats (handling "auto" probing or validation).
// Returns resolved format IDs, whether fallback to standard occurred, and any error.
func ResolveProjectFormats(projectRoot string, requestedFormats []string) ([]string, bool, error) {
	if len(requestedFormats) == 0 {
		return []string{model.FormatStandard}, false, nil
	}

	isAuto := false
	for _, f := range requestedFormats {
		if strings.EqualFold(strings.TrimSpace(f), model.FormatAuto) {
			isAuto = true
			break
		}
	}

	if isAuto {
		detected := DetectProjectFormats(projectRoot)
		if len(detected) == 0 {
			// Fallback to standard
			return []string{model.FormatStandard}, true, nil
		}
		return detected, false, nil
	}

	// Validate requested explicit formats
	var resolved []string
	seen := make(map[string]bool)
	for _, raw := range requestedFormats {
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			fID := strings.TrimSpace(strings.ToLower(p))
			if fID == "" {
				continue
			}
			if _, ok := model.LookupFormat(fID); !ok {
				return nil, false, fmt.Errorf("unsupported format %q. Available formats: %s", fID, strings.Join(model.AvailableFormatIDs(), ", "))
			}
			if !seen[fID] {
				seen[fID] = true
				resolved = append(resolved, fID)
			}
		}
	}

	if len(resolved) == 0 {
		resolved = append(resolved, model.FormatStandard)
	}

	return resolved, false, nil
}

// DeployGlobal distributes a managed skill into a global program directory.
func (d *Deployer) DeployGlobal(ctx context.Context, workspaceRoot, appName, globalPath, skillID string, force bool) error {
	if err := skill.ValidateID(skillID); err != nil {
		return err
	}
	managedDir := catalog.SkillDir(workspaceRoot, skillID)
	if !fsx.DirExists(managedDir) {
		return fmt.Errorf("managed skill %q not found in workspace (%s)", skillID, managedDir)
	}

	if _, err := skill.ValidateSkillDir(managedDir, skillID); err != nil {
		return fmt.Errorf("managed skill %q is invalid: %w", skillID, err)
	}

	managedHash, err := hash.HashDir(managedDir)
	if err != nil {
		return fmt.Errorf("failed to compute managed skill hash: %w", err)
	}

	if err := os.MkdirAll(globalPath, 0755); err != nil {
		return fmt.Errorf("failed to create global base directory %s: %w", globalPath, err)
	}
	unlock, err := LockTarget(ctx, globalPath)
	if err != nil {
		return err
	}
	defer unlock()

	destDir, err := SafeSkillPath(globalPath, skillID)
	if err != nil {
		return err
	}
	if fsx.DirExists(destDir) {
		currentTargetHash, err := hash.HashDir(destDir)
		if err != nil {
			return fmt.Errorf("failed to compute target skill hash at %s: %w", destDir, err)
		}

		rec, hasRecord, err := d.stateManager.GetRecord(appName, skillID)
		if err != nil {
			return fmt.Errorf("failed to read deployment state: %w", err)
		}

		if !hasRecord {
			if !force {
				return &TargetConflictError{
					SkillID:     skillID,
					Target:      appName,
					TargetDir:   destDir,
					IsUntracked: true,
					IsProject:   false,
				}
			}
		} else if currentTargetHash != rec.ContentHash {
			if !force {
				return &TargetConflictError{
					SkillID:     skillID,
					Target:      appName,
					TargetDir:   destDir,
					IsUntracked: false,
					IsProject:   false,
				}
			}
		}

	}

	swap, err := PromoteDirectory(managedDir, destDir, globalPath, skillID)
	if err != nil {
		return err
	}
	targetCanon := fsx.CanonicalPath(globalPath)
	wsCanon := fsx.CanonicalPath(workspaceRoot)
	if err := d.stateManager.RecordDeployment(appName, skillID, managedHash, targetCanon, wsCanon); err != nil {
		if rollbackErr := swap.Rollback(); rollbackErr != nil {
			return fmt.Errorf("failed to record deployment state: %v; rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("failed to record deployment state: %w", err)
	}
	return swap.Commit()
}

// UndeployGlobal removes a deployed skill from a global program directory.
func (d *Deployer) UndeployGlobal(ctx context.Context, appName, globalPath, skillID string, force bool) error {
	if err := skill.ValidateID(skillID); err != nil {
		return err
	}
	destDir, err := SafeSkillPath(globalPath, skillID)
	if err != nil {
		return err
	}
	if fsx.DirExists(globalPath) {
		unlock, lockErr := LockTarget(ctx, globalPath)
		if lockErr != nil {
			return lockErr
		}
		defer unlock()
	}
	rec, hasRecord, err := d.stateManager.GetRecord(appName, skillID)
	if err != nil {
		return fmt.Errorf("failed to read deployment state: %w", err)
	}
	if !fsx.DirExists(destDir) {
		if !hasRecord {
			return nil
		}
		return d.stateManager.RemoveDeployment(appName, skillID)
	}

	if !hasRecord && !force {
		return fmt.Errorf("refusing to remove untracked skill %s from global %q; use --force to remove it", skillID, appName)
	}
	currentTargetHash, err := hash.HashDir(destDir)
	if err != nil {
		return fmt.Errorf("failed to compute target skill hash at %s: %w", destDir, err)
	}
	if hasRecord && currentTargetHash != rec.ContentHash && !force {
		return fmt.Errorf("skill %s in global %q has been modified since last deployment. Use --force to undeploy", skillID, appName)
	}

	swap, err := stageRemoval(destDir, globalPath, skillID)
	if err != nil {
		return err
	}
	if err := d.stateManager.RemoveDeployment(appName, skillID); err != nil {
		if rollbackErr := swap.Rollback(); rollbackErr != nil {
			return fmt.Errorf("failed to remove deployment state: %v; rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("failed to remove deployment state: %w", err)
	}
	return swap.Commit()
}

// DeployProject distributes a managed skill into a project with specified format(s).
// Returns the list of successfully deployed formats.
func (d *Deployer) DeployProject(ctx context.Context, workspaceRoot, projectRoot string, formats []string, skillID string, force bool) ([]string, bool, error) {
	if err := skill.ValidateID(skillID); err != nil {
		return nil, false, err
	}
	managedDir := catalog.SkillDir(workspaceRoot, skillID)
	if !fsx.DirExists(managedDir) {
		return nil, false, fmt.Errorf("managed skill %q not found in workspace (%s)", skillID, managedDir)
	}

	if _, err := skill.ValidateSkillDir(managedDir, skillID); err != nil {
		return nil, false, fmt.Errorf("managed skill %q is invalid: %w", skillID, err)
	}

	managedHash, err := hash.HashDir(managedDir)
	if err != nil {
		return nil, false, fmt.Errorf("failed to compute managed skill hash: %w", err)
	}

	resolvedFormats, fallback, err := ResolveProjectFormats(projectRoot, formats)
	if err != nil {
		return nil, false, err
	}

	var deployed []string
	for _, fmtID := range resolvedFormats {
		fDef, ok := model.LookupFormat(fmtID)
		if !ok {
			continue
		}

		targetBase := filepath.Join(projectRoot, fDef.Subpath)
		if err := os.MkdirAll(targetBase, 0755); err != nil {
			return deployed, fallback, fmt.Errorf("failed to create directory %s: %w", targetBase, err)
		}
		unlock, err := LockTarget(ctx, targetBase)
		if err != nil {
			return deployed, fallback, err
		}

		destDir, err := SafeSkillPath(targetBase, skillID)
		if err != nil {
			_ = unlock()
			return deployed, fallback, err
		}
		relDest := filepath.Join(fDef.Subpath, skillID)

		if fsx.DirExists(destDir) {
			currentTargetHash, err := hash.HashDir(destDir)
			if err != nil {
				_ = unlock()
				return deployed, fallback, fmt.Errorf("failed to compute target skill hash at %s: %w", destDir, err)
			}

			rec, hasRecord, err := state.GetProjectRecord(projectRoot, skillID, fmtID)
			if err != nil {
				_ = unlock()
				return deployed, fallback, fmt.Errorf("failed to read project deployment state: %w", err)
			}

			if !hasRecord {
				if !force {
					_ = unlock()
					return deployed, fallback, &TargetConflictError{
						SkillID:     skillID,
						Target:      projectRoot,
						Format:      fmtID,
						TargetDir:   destDir,
						RelDest:     relDest,
						IsUntracked: true,
						IsProject:   true,
					}
				}
			} else if currentTargetHash != rec.ContentHash {
				if !force {
					_ = unlock()
					return deployed, fallback, &TargetConflictError{
						SkillID:     skillID,
						Target:      projectRoot,
						Format:      fmtID,
						TargetDir:   destDir,
						RelDest:     relDest,
						IsUntracked: false,
						IsProject:   true,
					}
				}
			}

		}

		swap, err := PromoteDirectory(managedDir, destDir, targetBase, skillID)
		if err != nil {
			_ = unlock()
			return deployed, fallback, err
		}
		if err := state.RecordProjectDeployment(projectRoot, skillID, fmtID, relDest, managedHash); err != nil {
			if rollbackErr := swap.Rollback(); rollbackErr != nil {
				_ = unlock()
				return deployed, fallback, fmt.Errorf("failed to record project deployment state: %v; rollback failed: %w", err, rollbackErr)
			}
			_ = unlock()
			return deployed, fallback, fmt.Errorf("failed to record project deployment state: %w", err)
		}
		if err := swap.Commit(); err != nil {
			_ = unlock()
			return deployed, fallback, err
		}
		if err := unlock(); err != nil {
			return deployed, fallback, err
		}

		deployed = append(deployed, fmtID)
	}

	return deployed, fallback, nil
}

// UndeployProject removes a deployed skill from a project for specified format(s).
// If formats is empty, all recorded formats for this skill in the project are undeployed.
func (d *Deployer) UndeployProject(ctx context.Context, projectRoot string, formats []string, skillID string, force bool) ([]string, error) {
	if err := skill.ValidateID(skillID); err != nil {
		return nil, err
	}
	skillRec, hasRecord, err := state.GetProjectSkillRecord(projectRoot, skillID)
	if err != nil {
		return nil, fmt.Errorf("failed to read project deployment state: %w", err)
	}

	var targetFormats []string
	if len(formats) == 0 {
		if hasRecord && skillRec != nil {
			for fmtID := range skillRec.Formats {
				targetFormats = append(targetFormats, fmtID)
			}
		}
		if len(targetFormats) == 0 {
			return nil, fmt.Errorf("no deployment record found for skill %q in project", skillID)
		}
	} else {
		for _, raw := range formats {
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				fmtID := strings.TrimSpace(strings.ToLower(p))
				if fmtID == "" {
					continue
				}
				if (fmtID == "all" || fmtID == model.FormatAuto) && hasRecord && skillRec != nil {
					for id := range skillRec.Formats {
						targetFormats = append(targetFormats, id)
					}
					continue
				}
				if _, ok := model.LookupFormat(fmtID); !ok {
					return nil, fmt.Errorf("unsupported format %q", fmtID)
				}
				targetFormats = append(targetFormats, fmtID)
			}
		}
	}

	var undeployed []string
	for _, fmtID := range targetFormats {
		fDef, ok := model.LookupFormat(fmtID)
		if !ok {
			continue
		}

		var rec model.ProjectDeploymentRecord
		recorded := false
		if hasRecord && skillRec != nil {
			rec, recorded = skillRec.Formats[fmtID]
		}
		if !recorded && !force {
			return undeployed, fmt.Errorf("refusing to remove untracked skill %s from project format %q; use --force to remove it", skillID, fmtID)
		}
		targetBase := filepath.Join(projectRoot, fDef.Subpath)
		if !fsx.DirExists(targetBase) {
			if recorded {
				if err := state.RemoveProjectDeployment(projectRoot, skillID, fmtID); err != nil {
					return undeployed, err
				}
				undeployed = append(undeployed, fmtID)
			}
			continue
		}
		unlock, err := LockTarget(ctx, targetBase)
		if err != nil {
			return undeployed, err
		}
		destDir, err := SafeSkillPath(targetBase, skillID)
		if err != nil {
			_ = unlock()
			return undeployed, err
		}
		if fsx.DirExists(destDir) {
			currentTargetHash, err := hash.HashDir(destDir)
			if err != nil {
				_ = unlock()
				return undeployed, fmt.Errorf("failed to compute target skill hash at %s: %w", destDir, err)
			}
			if recorded && currentTargetHash != rec.ContentHash && !force {
				_ = unlock()
				return undeployed, fmt.Errorf("skill %s in project format %q has been modified since last deployment. Use --force to undeploy", skillID, fmtID)
			}
			swap, err := stageRemoval(destDir, targetBase, skillID)
			if err != nil {
				_ = unlock()
				return undeployed, err
			}
			if err := state.RemoveProjectDeployment(projectRoot, skillID, fmtID); err != nil {
				_ = swap.Rollback()
				_ = unlock()
				return undeployed, err
			}
			if err := swap.Commit(); err != nil {
				_ = unlock()
				return undeployed, err
			}
		} else if err := state.RemoveProjectDeployment(projectRoot, skillID, fmtID); err != nil {
			_ = unlock()
			return undeployed, err
		}
		if err := unlock(); err != nil {
			return undeployed, err
		}
		undeployed = append(undeployed, fmtID)
	}

	return undeployed, nil
}

// Deploy distributes a managed skill into a target directory (compatibility wrapper).
func (d *Deployer) Deploy(ctx context.Context, workspaceRoot, targetName, targetPath, skillID string, force bool) error {
	return d.DeployGlobal(ctx, workspaceRoot, targetName, targetPath, skillID, force)
}

// Undeploy removes a deployed skill from a target directory (compatibility wrapper).
func (d *Deployer) Undeploy(ctx context.Context, targetName, targetPath, skillID string, force bool) error {
	return d.UndeployGlobal(ctx, targetName, targetPath, skillID, force)
}

// DirectorySwap holds the state of an in-progress directory deployment or removal,
// supporting atomic commit or rollback.
type DirectorySwap struct {
	destDir     string
	backupDir   string
	hadExisting bool
}

// SafeSkillPath resolves and verifies that a skill destination remains strictly
// bounded within baseDir, preventing directory traversal.
func SafeSkillPath(baseDir, skillID string) (string, error) {
	if err := skill.ValidateID(skillID); err != nil {
		return "", err
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	if resolvedBase, resolveErr := filepath.EvalSymlinks(absBase); resolveErr == nil {
		absBase = resolvedBase
	}
	dest := filepath.Join(absBase, skillID)
	rel, err := filepath.Rel(absBase, dest)
	if err != nil || rel != skillID {
		return "", fmt.Errorf("skill path escapes target directory: %q", skillID)
	}
	return dest, nil
}

// PromoteDirectory stages a source directory into destDir via targetBase,
// backing up existing content and returning a DirectorySwap.
func PromoteDirectory(managedDir, destDir, targetBase, skillID string) (*DirectorySwap, error) {
	tmpDir, err := os.MkdirTemp(targetBase, ".asoul-deploy-tmp-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp deploy dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	stageDir := filepath.Join(tmpDir, skillID)
	if err := fsx.CopyDir(managedDir, stageDir); err != nil {
		return nil, fmt.Errorf("failed to stage deploy copy: %w", err)
	}

	swap := &DirectorySwap{destDir: destDir, hadExisting: fsx.DirExists(destDir)}
	if swap.hadExisting {
		backupDir, err := reserveBackupPath(targetBase, skillID)
		if err != nil {
			return nil, err
		}
		swap.backupDir = backupDir
		if err := os.Rename(destDir, backupDir); err != nil {
			return nil, fmt.Errorf("failed to backup existing target directory: %w", err)
		}
	}

	if err := os.Rename(stageDir, destDir); err != nil {
		_ = swap.Rollback()
		return nil, fmt.Errorf("failed to replace target directory: %w", err)
	}
	return swap, nil
}

func stageRemoval(destDir, targetBase, skillID string) (*DirectorySwap, error) {
	backupDir, err := reserveBackupPath(targetBase, skillID)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(destDir, backupDir); err != nil {
		return nil, fmt.Errorf("failed to stage deployed skill removal: %w", err)
	}
	return &DirectorySwap{destDir: destDir, backupDir: backupDir, hadExisting: true}, nil
}

func reserveBackupPath(targetBase, skillID string) (string, error) {
	backupDir, err := os.MkdirTemp(targetBase, ".asoul-deploy-backup-"+skillID+"-*")
	if err != nil {
		return "", fmt.Errorf("failed to reserve backup path: %w", err)
	}
	if err := os.Remove(backupDir); err != nil {
		return "", fmt.Errorf("failed to prepare backup path: %w", err)
	}
	return backupDir, nil
}

// Rollback restores the previous directory state if a backup exists and removes staged files.
func (s *DirectorySwap) Rollback() error {
	if err := os.RemoveAll(s.destDir); err != nil {
		return err
	}
	if s.hadExisting && s.backupDir != "" {
		return os.Rename(s.backupDir, s.destDir)
	}
	return nil
}

// Commit removes the backup directory after successful state recording.
func (s *DirectorySwap) Commit() error {
	if s.backupDir == "" {
		return nil
	}
	if err := os.RemoveAll(s.backupDir); err != nil {
		return fmt.Errorf("deployment succeeded but backup cleanup failed: %w", err)
	}
	return nil
}

// LockTarget locks the targetBase directory using flock to protect against concurrent deployments.
func LockTarget(ctx context.Context, targetBase string) (func() error, error) {
	lockCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	targetLock := flock.New(filepath.Join(targetBase, ".asoul-deploy.lock"))
	ok, err := targetLock.TryLockContext(lockCtx, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("failed to lock deployment target %s: %w", targetBase, err)
	}
	if !ok {
		return nil, fmt.Errorf("deployment target is locked: %s", targetBase)
	}
	return targetLock.Unlock, nil
}
