package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"asoul/internal/agent"
	_ "asoul/internal/agent/opencode" // register built-in agent channels
	"asoul/internal/cache"
	"asoul/internal/catalog"
	"asoul/internal/config"
	"asoul/internal/deploy"
	"asoul/internal/fsx"
	"asoul/internal/gitx"
	"asoul/internal/hash"
	"asoul/internal/jsonc"
	"asoul/internal/lock"
	"asoul/internal/model"
	"asoul/internal/modelrules"
	"asoul/internal/modelsdev"
	"asoul/internal/progress"
	"asoul/internal/skill"
	"asoul/internal/source"
	"asoul/internal/source/git"
	"asoul/internal/source/local"
	"asoul/internal/source/managed"
	"asoul/internal/state"

	"github.com/gofrs/flock"
)

// Service defines the unified application interface for both CLI and TUI.
type Service struct {
	workspaceRoot string
	configMgr     *config.Manager
	stateMgr      *state.Manager
	cacheMgr      *cache.Manager
	gitClient     *gitx.Client
	deployer      *deploy.Deployer
	locker        *lock.Locker

	managedSource *managed.Source
	localSource   *local.Source
	gitSource     *git.Source
}

// NewService creates a new application service instance.
func NewService(workspaceRoot string, cfgMgr *config.Manager, stateMgr *state.Manager, cacheMgr *cache.Manager) *Service {
	gitClient := gitx.NewClient()
	deployer := deploy.NewDeployer(stateMgr)

	var locker *lock.Locker
	if workspaceRoot != "" {
		locker = lock.New(workspaceRoot)
	}

	svc := &Service{
		workspaceRoot: workspaceRoot,
		configMgr:     cfgMgr,
		stateMgr:      stateMgr,
		cacheMgr:      cacheMgr,
		gitClient:     gitClient,
		deployer:      deployer,
		locker:        locker,
		managedSource: managed.New(workspaceRoot),
		localSource:   local.New(workspaceRoot),
		gitSource:     git.New(gitClient, cacheMgr),
	}
	return svc
}

// WorkspaceRoot returns the current workspace root directory.
func (s *Service) WorkspaceRoot() string {
	return s.workspaceRoot
}

// SetWorkspaceRoot updates the workspace root.
func (s *Service) SetWorkspaceRoot(root string) {
	s.workspaceRoot = root
	s.locker = lock.New(root)
	s.managedSource = managed.New(root)
	s.localSource = local.New(root)
}

// InitWorkspace initializes a new workspace directory.
func (s *Service) InitWorkspace(ctx context.Context, dir string) (string, error) {
	expanded, err := fsx.ExpandUser(dir)
	if err != nil {
		expanded = dir
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	if err := catalog.InitWorkspace(abs); err != nil {
		return "", err
	}
	s.SetWorkspaceRoot(abs)
	return abs, nil
}

// Validate validates any directory as a valid skill.
func (s *Service) Validate(ctx context.Context, dirPath string) (*model.SkillMetadata, error) {
	expanded, err := fsx.ExpandUser(dirPath)
	if err != nil {
		return nil, err
	}
	return skill.ValidateSkillDir(expanded, "")
}

// Status returns the managed status and optionally upstream status of all skills.
func (s *Service) Status(ctx context.Context, refresh bool) ([]model.SkillStatus, error) {
	if s.workspaceRoot == "" {
		return nil, fmt.Errorf("no active workspace")
	}

	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return nil, err
	}

	skillsDir := catalog.SkillsDir(s.workspaceRoot)
	diskEntries, _ := os.ReadDir(skillsDir)
	diskMap := make(map[string]bool)
	for _, entry := range diskEntries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			diskMap[entry.Name()] = true
		}
	}

	var results []model.SkillStatus

	// 1. Process catalog records
	for id, record := range cat.Skills {
		stat := model.SkillStatus{
			ID:             id,
			Source:         record.Source,
			RecordedHash:   record.Resolved.ContentHash,
			RecordedCommit: record.Resolved.Commit,
			Upstream:       model.UpstreamNone,
		}
		if err := skill.ValidateID(id); err != nil {
			stat.ManagedStatus = model.StatusInvalid
			stat.Error = err.Error()
			results = append(results, stat)
			continue
		}

		skillDir := catalog.SkillDir(s.workspaceRoot, id)
		if !fsx.DirExists(skillDir) {
			stat.ManagedStatus = model.StatusMissing
		} else {
			delete(diskMap, id)
			_, valErr := skill.ValidateSkillDir(skillDir, id)
			if valErr != nil {
				stat.ManagedStatus = model.StatusInvalid
				stat.Error = valErr.Error()
			} else {
				currHash, hashErr := hash.HashDir(skillDir)
				if hashErr != nil {
					stat.ManagedStatus = model.StatusInvalid
					stat.Error = hashErr.Error()
				} else {
					stat.CurrentHash = currHash
					if currHash == record.Resolved.ContentHash {
						stat.ManagedStatus = model.StatusClean
					} else {
						stat.ManagedStatus = model.StatusModified
					}
				}
			}
		}

		results = append(results, stat)
	}

	// 2. Unmanaged directories on disk
	for unmanagedID := range diskMap {
		skillDir := catalog.SkillDir(s.workspaceRoot, unmanagedID)
		currHash, _ := hash.HashDir(skillDir)
		results = append(results, model.SkillStatus{
			ID: unmanagedID,
			Source: model.SourceSpec{
				Type: model.SourceTypeManaged,
			},
			ManagedStatus: model.StatusUnmanaged,
			CurrentHash:   currHash,
			Upstream:      model.UpstreamNone,
		})
	}

	// Sort by ID
	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	// 3. If refresh requested, check upstream
	if refresh {
		return s.Check(ctx, nil)
	}

	return results, nil
}

// List returns simple list of managed skills.
func (s *Service) List(ctx context.Context) ([]model.SkillStatus, error) {
	return s.Status(ctx, false)
}

// Show returns skill metadata and SKILL.md content.
func (s *Service) Show(ctx context.Context, id string) (*model.SkillStatus, string, error) {
	statuses, err := s.Status(ctx, false)
	if err != nil {
		return nil, "", err
	}

	var found *model.SkillStatus
	for _, st := range statuses {
		if st.ID == id {
			found = &st
			break
		}
	}

	if found == nil {
		return nil, "", fmt.Errorf("skill %q not found in workspace", id)
	}

	skillMDPath := filepath.Join(catalog.SkillDir(s.workspaceRoot, id), "SKILL.md")
	var body string
	if fsx.FileExists(skillMDPath) {
		data, _ := os.ReadFile(skillMDPath)
		body = string(data)
	}

	return found, body, nil
}

// NewSkill creates a new managed skill in the workspace.
func (s *Service) NewSkill(ctx context.Context, id string) error {
	if err := skill.ValidateID(id); err != nil {
		return err
	}

	unlock, err := s.locker.Lock(ctx, 5*time.Second)
	if err != nil {
		return err
	}
	defer unlock()

	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return err
	}

	if _, exists := cat.Skills[id]; exists {
		return fmt.Errorf("skill %q already exists in catalog", id)
	}

	skillDir := catalog.SkillDir(s.workspaceRoot, id)
	if fsx.DirExists(skillDir) {
		return fmt.Errorf("directory %s already exists on disk", skillDir)
	}

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory: %w", err)
	}

	mdContent := skill.GenerateSkillMD(id, "Agent skill for "+id)
	if err := fsx.AtomicWriteFile(filepath.Join(skillDir, "SKILL.md"), mdContent, 0644); err != nil {
		_ = os.RemoveAll(skillDir)
		return fmt.Errorf("failed to write SKILL.md: %w", err)
	}

	h, err := hash.HashDir(skillDir)
	if err != nil {
		_ = os.RemoveAll(skillDir)
		return err
	}

	cat.Skills[id] = model.SkillRecord{
		Source: model.SourceSpec{
			Type: model.SourceTypeManaged,
		},
		Resolved: model.Resolved{
			ContentHash: h,
		},
	}

	return catalog.Save(s.workspaceRoot, cat)
}

// Adopt imports an unmanaged skill directory into catalog.
func (s *Service) Adopt(ctx context.Context, id string) error {
	if err := skill.ValidateID(id); err != nil {
		return err
	}
	unlock, err := s.locker.Lock(ctx, 5*time.Second)
	if err != nil {
		return err
	}
	defer unlock()

	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return err
	}

	if _, exists := cat.Skills[id]; exists {
		return fmt.Errorf("skill %q is already managed in catalog", id)
	}

	skillDir := catalog.SkillDir(s.workspaceRoot, id)
	if !fsx.DirExists(skillDir) {
		return fmt.Errorf("unmanaged skill directory %s does not exist", skillDir)
	}

	if _, err := skill.ValidateSkillDir(skillDir, id); err != nil {
		return fmt.Errorf("cannot adopt invalid skill: %w", err)
	}

	h, err := hash.HashDir(skillDir)
	if err != nil {
		return err
	}

	cat.Skills[id] = model.SkillRecord{
		Source: model.SourceSpec{
			Type: model.SourceTypeManaged,
		},
		Resolved: model.Resolved{
			ContentHash: h,
		},
	}

	return catalog.Save(s.workspaceRoot, cat)
}

// RemoveBatchResult contains results of a batch skill removal.
type RemoveBatchResult struct {
	Matched  []string          `json:"matched"`
	Removed  []string          `json:"removed"`
	Skipped  []string          `json:"skipped,omitempty"`
	Warnings map[string]string `json:"warnings,omitempty"`
	Failed   map[string]string `json:"failed,omitempty"`
}

// RemoveBatch deletes multiple skills from catalog and managed store.
func (s *Service) RemoveBatch(ctx context.Context, skillIDs []string, force bool, onProgress ...progress.Func) (*RemoveBatchResult, error) {
	unlock, err := s.locker.Lock(ctx, 15*time.Second)
	if err != nil {
		return nil, err
	}
	defer unlock()

	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return nil, err
	}

	res := &RemoveBatchResult{
		Matched:  make([]string, 0, len(skillIDs)),
		Removed:  make([]string, 0),
		Skipped:  make([]string, 0),
		Warnings: make(map[string]string),
		Failed:   make(map[string]string),
	}

	// Deduplicate skillIDs while preserving input order
	seen := make(map[string]bool)
	var deduped []string
	for _, id := range skillIDs {
		if !seen[id] {
			seen[id] = true
			deduped = append(deduped, id)
		}
	}
	res.Matched = deduped

	skillsBaseDir := catalog.SkillsDir(s.workspaceRoot)
	var toDelete []string

	for _, id := range deduped {
		if err := skill.ValidateID(id); err != nil {
			res.Failed[id] = fmt.Sprintf("invalid skill ID: %v", err)
			continue
		}

		rec, exists := cat.Skills[id]
		if !exists {
			res.Failed[id] = fmt.Sprintf("skill %q not found in catalog", id)
			continue
		}

		skillDir := catalog.SkillDir(s.workspaceRoot, id)

		// Path safety boundary check
		rel, err := filepath.Rel(skillsBaseDir, skillDir)
		if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
			res.Failed[id] = "skill path escapes skills directory"
			continue
		}

		if fsx.DirExists(skillDir) {
			// Symlink boundary check
			if evalDir, err := filepath.EvalSymlinks(skillDir); err == nil {
				if evalBase, err := filepath.EvalSymlinks(skillsBaseDir); err == nil {
					if evalRel, err := filepath.Rel(evalBase, evalDir); err != nil || strings.HasPrefix(evalRel, "..") {
						res.Failed[id] = "skill directory symlink escapes skills root"
						continue
					}
				}
			}

			currHash, err := hash.HashDir(skillDir)
			if err != nil {
				res.Failed[id] = fmt.Sprintf("failed to verify skill %q before removal: %v", id, err)
				continue
			}
			if currHash != rec.Resolved.ContentHash && !force {
				res.Failed[id] = fmt.Sprintf("skill %q has local modifications. Use --force to remove", id)
				continue
			}
		}

		toDelete = append(toDelete, id)
	}

	cb := progress.First(onProgress...)
	for i, id := range toDelete {
		progress.Send(cb, progress.Update{Phase: progress.PhaseRemove, Current: i + 1, Total: len(toDelete), Text: id})
		skillDir := catalog.SkillDir(s.workspaceRoot, id)
		if fsx.DirExists(skillDir) {
			if err := os.RemoveAll(skillDir); err != nil {
				res.Failed[id] = fmt.Sprintf("failed to remove skill directory: %v", err)
				continue
			}
		}

		delete(cat.Skills, id)
		res.Removed = append(res.Removed, id)

		// Prune from configured profiles/groups if config manager is present
		if s.configMgr != nil {
			if err := s.configMgr.Update(func(cfg *model.Config) error {
				for pName, profile := range cfg.Profiles {
					var filtered []string
					changed := false
					for _, sk := range profile.Skills {
						if sk == id {
							changed = true
						} else {
							filtered = append(filtered, sk)
						}
					}
					if changed {
						profile.Skills = filtered
						cfg.Profiles[pName] = profile
					}
				}
				return nil
			}); err != nil {
				res.Warnings[id] = fmt.Sprintf("failed to update config groups: %v", err)
			}
		}
	}

	if len(res.Removed) > 0 {
		if err := catalog.Save(s.workspaceRoot, cat); err != nil {
			return res, fmt.Errorf("failed to save catalog: %w", err)
		}
	}

	return res, nil
}

// Remove deletes a single skill from catalog and managed store.
func (s *Service) Remove(ctx context.Context, id string, force bool, onProgress ...progress.Func) error {
	res, err := s.RemoveBatch(ctx, []string{id}, force, onProgress...)
	if err != nil {
		return err
	}
	if fErr, failed := res.Failed[id]; failed {
		return errors.New(fErr)
	}
	return nil
}

// SkillFilterOptions defines criteria for filtering skills.
type SkillFilterOptions struct {
	IDs        []string
	Pattern    string // regex pattern
	Source     string // Git URL or local path prefix or exact match
	SourceType string // "git", "local", "managed"
}

// FilterSkills filters workspace skills by ID list, regex pattern, source, or source type.
func (s *Service) FilterSkills(ctx context.Context, opts SkillFilterOptions) ([]model.SkillStatus, error) {
	var re *regexp.Regexp
	if opts.Pattern != "" {
		var err error
		re, err = regexp.Compile(opts.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern regex: %w", err)
		}
	}

	skills, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	idMap := make(map[string]bool, len(opts.IDs))
	for _, id := range opts.IDs {
		idMap[id] = true
	}

	var matched []model.SkillStatus
	for _, sk := range skills {
		if len(opts.IDs) > 0 && !idMap[sk.ID] {
			continue
		}

		if opts.SourceType != "" {
			if !strings.EqualFold(string(sk.Source.Type), opts.SourceType) {
				continue
			}
		}

		if opts.Source != "" {
			q := strings.ToLower(opts.Source)
			srcURL := strings.ToLower(sk.Source.URL)
			srcPath := strings.ToLower(sk.Source.Path)
			if !strings.Contains(srcURL, q) && !strings.Contains(srcPath, q) {
				continue
			}
		}

		if re != nil {
			matches := re.MatchString(sk.ID)
			if !matches {
				skillMDPath := filepath.Join(catalog.SkillDir(s.workspaceRoot, sk.ID), "SKILL.md")
				if meta, _, err := skill.ParseSkillMD(skillMDPath); err == nil && meta != nil && meta.Name != "" {
					matches = re.MatchString(meta.Name)
				}
			}
			if !matches {
				continue
			}
		}

		matched = append(matched, sk)
	}

	return matched, nil
}

// AllSkillDeployments returns a map from skill ID to all targets (agents/projects) where it is deployed.
func (s *Service) AllSkillDeployments() map[string][]string {
	res := make(map[string][]string)

	// 1. Agent targets
	if targets, err := s.TargetList(); err == nil {
		for name := range targets {
			deployed := s.TargetDeployedSkills(name)
			for _, sk := range deployed {
				res[sk] = append(res[sk], fmt.Sprintf("%s (agent)", name))
			}
		}
	}

	// 2. Project targets
	if projects, err := s.ProjectList(); err == nil {
		for _, proj := range projects {
			deployed := s.ProjectDeployedSkills(proj)
			for _, sk := range deployed {
				res[sk] = append(res[sk], fmt.Sprintf("%s (project)", proj))
			}
		}
	}

	for sk := range res {
		sort.Strings(res[sk])
	}
	return res
}

// SkillDeployments returns the target names/paths where skillID is actively deployed.
func (s *Service) SkillDeployments(skillID string) []string {
	all := s.AllSkillDeployments()
	return all[skillID]
}

// AddLocal adds a skill from a local filesystem path.
func (s *Service) AddLocal(ctx context.Context, localPath string, replaceSource bool) error {
	cand, err := s.localSource.Resolve(ctx, model.SourceSpec{
		Type: model.SourceTypeLocal,
		Path: localPath,
	})
	if err != nil {
		return err
	}
	defer cand.Cleanup()

	spec := model.SourceSpec{
		Type: model.SourceTypeLocal,
		Path: localPath,
	}

	return s.installCandidate(ctx, cand, spec, replaceSource, false)
}

// DiscoverGit discovers all skills in a git repository.
func (s *Service) DiscoverGit(ctx context.Context, url, ref string, onProgress ...progress.Func) ([]git.DiscoveredSkill, error) {
	return s.gitSource.DiscoverSkills(ctx, url, ref, onProgress...)
}

// BatchAddResult contains results of a batch skill addition.
type BatchAddResult struct {
	Added   []string          `json:"added"`
	Skipped []string          `json:"skipped,omitempty"`
	Failed  map[string]string `json:"failed,omitempty"`
}

type candidateWithSpec struct {
	candidate *source.Candidate
	spec      model.SourceSpec
}

// RenameSkill renames an existing managed skill from oldID to newID.
func (s *Service) RenameSkill(ctx context.Context, oldID, newID string) error {
	if err := skill.ValidateID(oldID); err != nil {
		return fmt.Errorf("invalid source skill ID: %w", err)
	}
	if err := skill.ValidateID(newID); err != nil {
		return fmt.Errorf("invalid target skill ID: %w", err)
	}
	if oldID == newID {
		return nil
	}

	unlock, err := s.locker.Lock(ctx, 10*time.Second)
	if err != nil {
		return err
	}
	defer unlock()

	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return err
	}

	rec, exists := cat.Skills[oldID]
	if !exists {
		return fmt.Errorf("skill %q not found in catalog", oldID)
	}
	if _, existsNew := cat.Skills[newID]; existsNew {
		return fmt.Errorf("skill %q already exists in catalog", newID)
	}

	skillsBaseDir := catalog.SkillsDir(s.workspaceRoot)
	oldDir := catalog.SkillDir(s.workspaceRoot, oldID)
	newDir := catalog.SkillDir(s.workspaceRoot, newID)

	if !fsx.DirExists(oldDir) {
		return fmt.Errorf("skill directory for %q does not exist: %s", oldID, oldDir)
	}
	if fsx.DirExists(newDir) {
		return fmt.Errorf("target directory for %q already exists on disk: %s", newID, newDir)
	}

	// Path safety
	rel, err := filepath.Rel(skillsBaseDir, newDir)
	if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
		return fmt.Errorf("target path escapes skills directory")
	}

	// 1. Update SKILL.md in oldDir
	skillMDPath := filepath.Join(oldDir, "SKILL.md")
	oldMDContent, err := os.ReadFile(skillMDPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", skillMDPath, err)
	}

	if err := skill.RewriteSkillMDName(skillMDPath, newID); err != nil {
		return fmt.Errorf("failed to update SKILL.md: %w", err)
	}

	// 2. Rename directory
	if err := os.Rename(oldDir, newDir); err != nil {
		_ = fsx.AtomicWriteFilePreservePerm(skillMDPath, oldMDContent)
		return fmt.Errorf("failed to rename skill directory: %w", err)
	}

	// 3. Compute new content hash
	newHash, err := hash.HashDir(newDir)
	if err != nil {
		_ = os.Rename(newDir, oldDir)
		_ = fsx.AtomicWriteFilePreservePerm(skillMDPath, oldMDContent)
		return fmt.Errorf("failed to compute content hash: %w", err)
	}

	// 4. Update catalog
	oldCatCopy := &model.Catalog{
		Version: cat.Version,
		Skills:  make(map[string]model.SkillRecord, len(cat.Skills)),
	}
	for id, r := range cat.Skills {
		oldCatCopy.Skills[id] = r
	}

	delete(cat.Skills, oldID)
	rec.Resolved.ContentHash = newHash
	cat.Skills[newID] = rec

	if err := catalog.Save(s.workspaceRoot, cat); err != nil {
		_ = os.Rename(newDir, oldDir)
		_ = fsx.AtomicWriteFilePreservePerm(skillMDPath, oldMDContent)
		_ = catalog.Save(s.workspaceRoot, oldCatCopy)
		return fmt.Errorf("failed to save catalog: %w", err)
	}

	return nil
}

// AddGit adds a skill from a git repository.
func (s *Service) AddGit(ctx context.Context, url, ref, subpath string, replaceSource bool, onProgress ...progress.Func) error {
	return s.AddGitCustom(ctx, url, ref, subpath, "", replaceSource, false, onProgress...)
}

// AddGitCustom adds a skill from a git repository with an optional custom ID (rename during import).
func (s *Service) AddGitCustom(ctx context.Context, url, ref, subpath, customID string, replaceSource, force bool, onProgress ...progress.Func) error {
	aliases := make(map[string]string)
	if customID != "" {
		aliases[subpath] = customID
	}
	res, err := s.AddGitBatchWithOptions(ctx, url, ref, []string{subpath}, aliases, nil, replaceSource, force, true, onProgress...)
	if err != nil {
		return err
	}
	if len(res.Failed) > 0 {
		for _, errMsg := range res.Failed {
			return fmt.Errorf("%s", errMsg)
		}
	}
	return nil
}

// AddGitBatch adds multiple skills from a git repository in a single batch operation.
// If fetch is true, it clones or fetches from remote; if false, it uses existing repository cache.
func (s *Service) AddGitBatch(ctx context.Context, url, ref string, subpaths []string, replaceSource, force, fetch bool, onProgress ...progress.Func) (*BatchAddResult, error) {
	return s.AddGitBatchWithOptions(ctx, url, ref, subpaths, nil, nil, replaceSource, force, fetch, onProgress...)
}

// AddGitBatchWithOptions adds multiple skills from a git repository with custom alias and per-skill overwrite mapping.
func (s *Service) AddGitBatchWithOptions(ctx context.Context, url, ref string, subpaths []string, aliases map[string]string, replaceSkills map[string]bool, replaceSource, force, fetch bool, onProgress ...progress.Func) (*BatchAddResult, error) {
	spec := model.SourceSpec{
		Type: model.SourceTypeGit,
		URL:  url,
		Ref:  ref,
	}

	candidates, failures, err := s.gitSource.ResolveBatch(ctx, spec, subpaths, fetch, onProgress...)
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, cand := range candidates {
			cand.Cleanup()
		}
	}()

	var items []candidateWithSpec
	for _, cand := range candidates {
		if aliases != nil {
			targetID := aliases[cand.Subpath]
			if targetID == "" {
				targetID = aliases[cand.SkillID]
			}
			if targetID != "" && targetID != cand.SkillID {
				if err := cand.OverrideID(targetID); err != nil {
					failures[cand.Subpath] = fmt.Errorf("failed to rename skill to %s: %w", targetID, err)
					continue
				}
			}
		}

		candSpec := spec
		candSpec.Path = cand.Subpath
		items = append(items, candidateWithSpec{
			candidate: cand,
			spec:      candSpec,
		})
	}

	res, err := s.installCandidatesBatch(ctx, items, replaceSkills, replaceSource, force)
	if err != nil {
		return nil, err
	}

	for sub, fErr := range failures {
		res.Failed[sub] = fErr.Error()
	}

	return res, nil
}

// installCandidatesBatch performs safe staged replacement into workspace for multiple candidates in a single transaction.
func (s *Service) installCandidatesBatch(ctx context.Context, items []candidateWithSpec, replaceSkills map[string]bool, replaceSource, force bool) (*BatchAddResult, error) {
	result := &BatchAddResult{
		Added:   make([]string, 0, len(items)),
		Skipped: make([]string, 0),
		Failed:  make(map[string]string),
	}

	if len(items) == 0 {
		return result, nil
	}

	unlock, err := s.locker.Lock(ctx, 15*time.Second)
	if err != nil {
		return nil, err
	}
	defer unlock()

	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return nil, err
	}

	oldCatCopy := &model.Catalog{
		Version: cat.Version,
		Skills:  make(map[string]model.SkillRecord, len(cat.Skills)),
	}
	for id, record := range cat.Skills {
		oldCatCopy.Skills[id] = record
	}

	skillsBaseDir := catalog.SkillsDir(s.workspaceRoot)

	type installTask struct {
		cand        *source.Candidate
		spec        model.SourceSpec
		managedDir  string
		hadExisting bool
		backupDir   string
		tmpStage    string
		tmpTarget   string
	}

	var tasks []installTask
	seenBatchIDs := make(map[string]string)

	for _, it := range items {
		cand := it.candidate
		spec := it.spec

		if err := skill.ValidateID(cand.SkillID); err != nil {
			result.Failed[cand.SkillID] = fmt.Sprintf("invalid skill ID: %v", err)
			continue
		}

		if prevSub, seen := seenBatchIDs[cand.SkillID]; seen {
			result.Failed[cand.SkillID] = fmt.Sprintf("duplicate skill ID %q in batch (conflicts with %s); use rename or select only one", cand.SkillID, prevSub)
			continue
		}
		seenBatchIDs[cand.SkillID] = cand.Subpath

		managedDir := catalog.SkillDir(s.workspaceRoot, cand.SkillID)
		rel, err := filepath.Rel(skillsBaseDir, managedDir)
		if err != nil || strings.HasPrefix(rel, "..") || rel == "." {
			result.Failed[cand.SkillID] = fmt.Sprintf("path %s escapes skills directory", managedDir)
			continue
		}

		existingRec, exists := cat.Skills[cand.SkillID]
		if exists {
			isReplacingThis := replaceSource || (replaceSkills != nil && (replaceSkills[cand.SkillID] || replaceSkills[cand.Subpath]))

			// Conflict detection
			if existingRec.Source.Type == spec.Type && existingRec.Source.URL == spec.URL && existingRec.Source.Path == spec.Path {
				// Idempotent: same ID + same source
				if !isReplacingThis && !force && existingRec.Resolved.Commit == cand.Commit && existingRec.Resolved.ContentHash == cand.Hash {
					result.Skipped = append(result.Skipped, cand.SkillID)
					continue
				}
			} else {
				// Same ID, different source
				if !isReplacingThis {
					result.Failed[cand.SkillID] = fmt.Sprintf("conflict: skill %q already exists from another source (%s). Mark for overwrite or use --replace-source to replace", cand.SkillID, existingRec.Source.Type)
					continue
				}
			}
		}

		// Local modification check on existing managed directory
		if fsx.DirExists(managedDir) {
			currHash, hashErr := hash.HashDir(managedDir)
			if hashErr != nil {
				result.Failed[cand.SkillID] = fmt.Sprintf("failed to verify existing skill %q: %v", cand.SkillID, hashErr)
				continue
			}
			if !exists && !force {
				result.Failed[cand.SkillID] = fmt.Sprintf("collision: unmanaged directory already exists for skill %q; adopt, rename, or use --force to overwrite", cand.SkillID)
				continue
			}
			if exists && currHash != existingRec.Resolved.ContentHash && !force {
				result.Failed[cand.SkillID] = fmt.Sprintf("local modifications detected in %q. Use --force to overwrite", cand.SkillID)
				continue
			}
		}

		tasks = append(tasks, installTask{
			cand:        cand,
			spec:        spec,
			managedDir:  managedDir,
			hadExisting: fsx.DirExists(managedDir),
		})
	}

	if len(tasks) == 0 {
		return result, nil
	}

	var movedTasks []installTask
	rollback := func() {
		for i := len(movedTasks) - 1; i >= 0; i-- {
			t := movedTasks[i]
			_ = os.RemoveAll(t.managedDir)
			if t.hadExisting && t.backupDir != "" && fsx.DirExists(t.backupDir) {
				_ = os.Rename(t.backupDir, t.managedDir)
			}
		}
		for _, t := range tasks {
			if t.tmpStage != "" {
				_ = os.RemoveAll(t.tmpStage)
			}
			if t.backupDir != "" {
				_ = os.RemoveAll(t.backupDir)
			}
		}
		_ = catalog.Save(s.workspaceRoot, oldCatCopy)
	}

	var installErr error
	for i := range tasks {
		t := &tasks[i]

		// 1. Stage copy to temporary sibling directory
		tmpStage, err := os.MkdirTemp(skillsBaseDir, fmt.Sprintf(".asoul-tmp-%s-*", t.cand.SkillID))
		if err != nil {
			installErr = fmt.Errorf("failed to create temporary stage directory for %s: %w", t.cand.SkillID, err)
			break
		}
		t.tmpStage = tmpStage

		tmpTarget := filepath.Join(tmpStage, t.cand.SkillID)
		if err := fsx.CopyDir(t.cand.TempDir, tmpTarget); err != nil {
			installErr = fmt.Errorf("failed to copy candidate %s to stage: %w", t.cand.SkillID, err)
			break
		}
		t.tmpTarget = tmpTarget

		// 2. Backup existing if any
		if t.hadExisting {
			backupDir, err := os.MkdirTemp(skillsBaseDir, fmt.Sprintf(".asoul-backup-%s-*", t.cand.SkillID))
			if err != nil {
				installErr = fmt.Errorf("failed to reserve backup directory for %s: %w", t.cand.SkillID, err)
				break
			}
			if err := os.Remove(backupDir); err != nil {
				installErr = fmt.Errorf("failed to prepare backup directory for %s: %w", t.cand.SkillID, err)
				break
			}
			if err := os.Rename(t.managedDir, backupDir); err != nil {
				installErr = fmt.Errorf("failed to backup existing skill directory for %s: %w", t.cand.SkillID, err)
				break
			}
			t.backupDir = backupDir
		}

		// 3. Move staged into place
		if err := os.Rename(tmpTarget, t.managedDir); err != nil {
			installErr = fmt.Errorf("failed to move staged skill %s into workspace: %w", t.cand.SkillID, err)
			break
		}

		movedTasks = append(movedTasks, *t)

		// 4. Update catalog entry
		cat.Skills[t.cand.SkillID] = model.SkillRecord{
			Source: t.spec,
			Resolved: model.Resolved{
				Commit:      t.cand.Commit,
				ContentHash: t.cand.Hash,
			},
		}
	}

	if installErr != nil {
		rollback()
		return nil, installErr
	}

	// 5. Commit catalog
	if err := catalog.Save(s.workspaceRoot, cat); err != nil {
		rollback()
		return nil, fmt.Errorf("failed to update catalog.json: %w", err)
	}

	// 6. Cleanup backups and temp stages
	for _, t := range tasks {
		if t.tmpStage != "" {
			_ = os.RemoveAll(t.tmpStage)
		}
		if t.backupDir != "" {
			_ = os.RemoveAll(t.backupDir)
		}
		result.Added = append(result.Added, t.cand.SkillID)
	}

	return result, nil
}

// installCandidate performs safe staged replacement into workspace for a single candidate.
func (s *Service) installCandidate(ctx context.Context, cand *source.Candidate, spec model.SourceSpec, replaceSource, force bool) error {
	res, err := s.installCandidatesBatch(ctx, []candidateWithSpec{{candidate: cand, spec: spec}}, nil, replaceSource, force)
	if err != nil {
		return err
	}
	if len(res.Failed) > 0 {
		for _, errMsg := range res.Failed {
			return fmt.Errorf("%s", errMsg)
		}
	}
	return nil
}

// Check checks upstream status for the given skill IDs (or all if empty).
// Optimizes git repos by fetching each repository only once.
func (s *Service) Check(ctx context.Context, skillIDs []string, onProgress ...progress.Func) ([]model.SkillStatus, error) {
	statuses, err := s.Status(ctx, false)
	if err != nil {
		return nil, err
	}

	filterMap := make(map[string]bool)
	for _, id := range skillIDs {
		filterMap[id] = true
	}

	total := 0
	for i := range statuses {
		if len(filterMap) == 0 || filterMap[statuses[i].ID] {
			total++
		}
	}
	cb := progress.First(onProgress...)
	done := 0

	// Batch git fetch by repository URL
	fetchedRepos := make(map[string]bool)
	fetchErrors := make(map[string]error)
	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return nil, err
	}

	for i := range statuses {
		st := &statuses[i]
		if len(filterMap) > 0 && !filterMap[st.ID] {
			continue
		}
		done++
		progress.Send(cb, progress.Update{Phase: progress.PhaseCheck, Current: done, Total: total, Text: st.ID})

		switch st.Source.Type {
		case model.SourceTypeGit:
			// Optimization: fetch repo once per check batch
			repoURL := st.Source.URL
			if repoURL != "" && !fetchedRepos[repoURL] {
				_, fetchErrors[repoURL] = s.gitSource.EnsureRepo(ctx, repoURL, true)
				fetchedRepos[repoURL] = true
			}
			if fetchErr := fetchErrors[repoURL]; fetchErr != nil {
				st.Upstream = model.UpstreamUnreachable
				st.Error = fetchErr.Error()
				continue
			}

			rec := cat.Skills[st.ID]
			res, err := s.gitSource.CheckCached(ctx, st.Source, rec.Resolved)
			if err != nil {
				st.Upstream = model.UpstreamInvalid
				st.Error = err.Error()
			} else {
				st.Upstream = res.Upstream
				st.NewCommit = res.Commit
			}

		case model.SourceTypeLocal:
			rec := cat.Skills[st.ID]
			res, err := s.localSource.Check(ctx, st.Source, rec.Resolved)
			if err != nil {
				st.Upstream = model.UpstreamInvalid
				st.Error = err.Error()
			} else {
				st.Upstream = res.Upstream
				st.NewHash = res.Hash
			}

		case model.SourceTypeManaged:
			st.Upstream = model.UpstreamNone
		}
	}

	return statuses, nil
}

// Diff generates diff between current managed skill and incoming upstream candidate.
func (s *Service) Diff(ctx context.Context, id string) (string, error) {
	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return "", err
	}

	rec, exists := cat.Skills[id]
	if !exists {
		return "", fmt.Errorf("skill %q not found in catalog", id)
	}

	managedDir := catalog.SkillDir(s.workspaceRoot, id)
	if !fsx.DirExists(managedDir) {
		return "", fmt.Errorf("managed skill directory %s not found", managedDir)
	}

	var cand *source.Candidate
	switch rec.Source.Type {
	case model.SourceTypeGit:
		cand, err = s.gitSource.Resolve(ctx, rec.Source)
	case model.SourceTypeLocal:
		cand, err = s.localSource.Resolve(ctx, rec.Source)
	default:
		return "", fmt.Errorf("managed skills do not have an upstream source to diff against")
	}

	if err != nil {
		return "", fmt.Errorf("failed to resolve upstream candidate for diff: %w", err)
	}
	defer cand.Cleanup()

	if cand.SkillID != id {
		_ = cand.OverrideID(id)
	}

	return s.gitClient.DiffNoIndex(ctx, managedDir, cand.TempDir)
}

// Update updates a skill to upstream candidate.
func (s *Service) Update(ctx context.Context, id string, force, replaceSource bool, onProgress ...progress.Func) (*model.SkillStatus, error) {
	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return nil, err
	}

	rec, exists := cat.Skills[id]
	if !exists {
		return nil, fmt.Errorf("skill %q not found in catalog", id)
	}

	var cand *source.Candidate
	switch rec.Source.Type {
	case model.SourceTypeGit:
		cand, err = s.gitSource.Resolve(ctx, rec.Source, onProgress...)
	case model.SourceTypeLocal:
		cand, err = s.localSource.Resolve(ctx, rec.Source)
	case model.SourceTypeManaged:
		return nil, fmt.Errorf("skill %q is managed locally and has no upstream to update", id)
	default:
		return nil, fmt.Errorf("unsupported source type %q", rec.Source.Type)
	}

	if err != nil {
		return nil, err
	}
	defer cand.Cleanup()

	if cand.SkillID != id {
		if err := cand.OverrideID(id); err != nil {
			return nil, err
		}
	}

	if err := s.installCandidate(ctx, cand, rec.Source, replaceSource, force); err != nil {
		return nil, err
	}

	showStatus, _, err := s.Show(ctx, id)
	return showStatus, err
}

// UpstreamList aggregates upstream sources (git repos and local sources) tracked by managed skills.
func (s *Service) UpstreamList(ctx context.Context) ([]model.UpstreamInfo, error) {
	statuses, err := s.Status(ctx, false)
	if err != nil {
		return nil, err
	}
	return s.buildUpstreamList(ctx, statuses)
}

func (s *Service) buildUpstreamList(ctx context.Context, statuses []model.SkillStatus) ([]model.UpstreamInfo, error) {
	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return nil, err
	}

	statusMap := make(map[string]model.SkillStatus, len(statuses))
	for _, st := range statuses {
		statusMap[st.ID] = st
	}

	type upstreamKey struct {
		srcType model.SourceType
		url     string
		ref     string
	}

	groups := make(map[upstreamKey]*model.UpstreamInfo)
	var keyOrder []upstreamKey

	for id, rec := range cat.Skills {
		if rec.Source.Type == model.SourceTypeManaged {
			continue
		}

		rawURL := rec.Source.URL
		if rawURL == "" && rec.Source.Type == model.SourceTypeLocal {
			rawURL = rec.Source.Path
		}
		if rawURL == "" {
			continue
		}

		k := upstreamKey{
			srcType: rec.Source.Type,
			url:     rawURL,
			ref:     rec.Source.Ref,
		}

		info, exists := groups[k]
		if !exists {
			info = &model.UpstreamInfo{
				URL:    rawURL,
				Type:   rec.Source.Type,
				Ref:    rec.Source.Ref,
				Status: model.UpstreamUpToDate,
			}
			groups[k] = info
			keyOrder = append(keyOrder, k)
		}

		info.Skills = append(info.Skills, id)
		if st, ok := statusMap[id]; ok {
			if st.RecordedCommit != "" && info.Commit == "" {
				info.Commit = st.RecordedCommit
			}
			if st.NewCommit != "" && info.NewCommit == "" {
				info.NewCommit = st.NewCommit
			}
			if st.NewHash != "" && info.NewHash == "" {
				info.NewHash = st.NewHash
			}
			if st.Error != "" && info.Error == "" {
				info.Error = st.Error
			}

			switch st.Upstream {
			case model.UpstreamUnreachable:
				info.Status = model.UpstreamUnreachable
			case model.UpstreamInvalid:
				if info.Status != model.UpstreamUnreachable {
					info.Status = model.UpstreamInvalid
				}
			case model.UpstreamUpdateAvailable:
				if info.Status != model.UpstreamUnreachable && info.Status != model.UpstreamInvalid {
					info.Status = model.UpstreamUpdateAvailable
				}
			case model.UpstreamSourceChanged:
				if info.Status != model.UpstreamUnreachable && info.Status != model.UpstreamInvalid && info.Status != model.UpstreamUpdateAvailable {
					info.Status = model.UpstreamSourceChanged
				}
			}
		}
	}

	// Also include configured upstreams from config.jsonc
	if s.configMgr != nil {
		if configured, err := s.configMgr.ListUpstreams(); err == nil {
			for _, cfgUp := range configured {
				if cfgUp.URL == "" {
					continue
				}
				k := upstreamKey{
					srcType: cfgUp.Type,
					url:     cfgUp.URL,
					ref:     cfgUp.Ref,
				}
				if info, exists := groups[k]; exists {
					if cfgUp.Name != "" && info.Name == "" {
						info.Name = cfgUp.Name
					}
					if cfgUp.CreatedAt != "" && info.CreatedAt == "" {
						info.CreatedAt = cfgUp.CreatedAt
					}
					if cfgUp.UpdatedAt != "" && info.UpdatedAt == "" {
						info.UpdatedAt = cfgUp.UpdatedAt
					}
				} else {
					info := &model.UpstreamInfo{
						URL:       cfgUp.URL,
						Name:      cfgUp.Name,
						Type:      cfgUp.Type,
						Ref:       cfgUp.Ref,
						Status:    model.UpstreamUpToDate,
						Skills:    []string{},
						CreatedAt: cfgUp.CreatedAt,
						UpdatedAt: cfgUp.UpdatedAt,
					}
					groups[k] = info
					keyOrder = append(keyOrder, k)
				}
			}
		}
	}

	var result []model.UpstreamInfo
	for _, k := range keyOrder {
		info := groups[k]
		sort.Strings(info.Skills)

		// Inspect local cache and discover available skills from cache without network
		if info.Type == model.SourceTypeGit && s.cacheMgr != nil && s.gitSource != nil {
			info.CachePath = s.cacheMgr.RepoDir(info.URL)
			info.CacheExists = s.cacheMgr.RepoExists(info.URL)
			if info.CacheExists {
				if info.Commit == "" {
					if commit, err := s.gitSource.ResolveCommit(ctx, info.URL, info.Ref); err == nil {
						info.Commit = commit
					}
				}
				if disc, err := s.gitSource.DiscoverSkills(ctx, info.URL, info.Ref); err == nil {
					var avail []string
					for _, d := range disc {
						avail = append(avail, d.ID)
					}
					sort.Strings(avail)
					info.AvailableSkills = avail
				}
				if info.UpdatedAt == "" {
					if meta := s.cacheMgr.GetRepoMeta(info.URL); meta != nil && !meta.LastUsedAt.IsZero() {
						info.UpdatedAt = meta.LastUsedAt.Local().Format("2006-01-02 15:04:05")
					}
				}
			}
		} else if info.Type == model.SourceTypeLocal {
			expanded, err := fsx.ExpandUser(info.URL)
			if err == nil && fsx.DirExists(expanded) {
				info.CachePath = expanded
				info.CacheExists = true
				if disc, err := s.UpstreamDiscover(ctx, info.URL, info.Ref); err == nil {
					var avail []string
					for _, d := range disc {
						avail = append(avail, d.ID)
					}
					sort.Strings(avail)
					info.AvailableSkills = avail
				}
			}
		}

		if info.CreatedAt == "" {
			var earliest time.Time
			for _, skID := range info.Skills {
				skDir := filepath.Join(s.workspaceRoot, catalog.SkillsDirName, skID)
				if fi, err := os.Stat(skDir); err == nil {
					if earliest.IsZero() || fi.ModTime().Before(earliest) {
						earliest = fi.ModTime()
					}
				}
			}
			if !earliest.IsZero() {
				info.CreatedAt = earliest.Local().Format("2006-01-02 15:04:05")
			}
		}
		if info.UpdatedAt == "" && info.CreatedAt != "" {
			info.UpdatedAt = info.CreatedAt
		}

		result = append(result, *info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].URL < result[j].URL
	})

	return result, nil
}

// UpstreamRefresh refreshes upstream sources status and available skills from the local cache and workspace state.
func (s *Service) UpstreamRefresh(ctx context.Context) ([]model.UpstreamInfo, error) {
	return s.UpstreamList(ctx)
}

// UpstreamAdd registers an upstream source in user configuration.
func (s *Service) UpstreamAdd(ctx context.Context, rawURL, ref, name string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("upstream URL or path cannot be empty")
	}

	srcType := model.SourceTypeGit
	if !strings.HasPrefix(rawURL, "http://") &&
		!strings.HasPrefix(rawURL, "https://") &&
		!strings.HasPrefix(rawURL, "git@") &&
		!strings.HasPrefix(rawURL, "ssh://") &&
		!strings.HasSuffix(rawURL, ".git") {
		expanded, err := fsx.ExpandUser(rawURL)
		if err == nil && fsx.DirExists(expanded) {
			srcType = model.SourceTypeLocal
			rawURL = expanded
		}
	}

	return s.configMgr.AddUpstream(model.UpstreamConfig{
		URL:  rawURL,
		Type: srcType,
		Ref:  ref,
		Name: name,
	})
}

// UpstreamRemove unregisters an upstream source. If removeSkills is true, also removes tracked skills from this upstream.
func (s *Service) UpstreamRemove(ctx context.Context, targetURL string, removeSkills bool) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return fmt.Errorf("upstream URL or path cannot be empty")
	}

	// Remove from config.jsonc
	_ = s.configMgr.RemoveUpstream(targetURL)

	// If removeSkills is requested, remove all skills linked to this upstream
	if removeSkills {
		cat, err := catalog.Load(s.workspaceRoot)
		if err == nil {
			for id, rec := range cat.Skills {
				if rec.Source.URL == targetURL || rec.Source.Path == targetURL {
					_ = s.Remove(ctx, id, true)
				}
			}
		}
	}

	return nil
}

// UpstreamEdit updates configuration (ref/name) for an upstream source.
func (s *Service) UpstreamEdit(ctx context.Context, targetURL, newRef, newName string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return fmt.Errorf("upstream URL or path cannot be empty")
	}

	upstreams, err := s.configMgr.ListUpstreams()
	if err != nil {
		return err
	}
	for _, u := range upstreams {
		if strings.TrimSpace(u.URL) == targetURL {
			u.Ref = newRef
			u.Name = newName
			return s.configMgr.UpdateUpstream(u)
		}
	}

	// If it was not in config.jsonc yet (only in catalog.json), add it to config.jsonc now
	srcType := model.SourceTypeGit
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") && !strings.HasPrefix(targetURL, "git@") && !strings.HasSuffix(targetURL, ".git") {
		srcType = model.SourceTypeLocal
	}
	return s.configMgr.AddUpstream(model.UpstreamConfig{
		URL:  targetURL,
		Type: srcType,
		Ref:  newRef,
		Name: newName,
	})
}

// UpstreamCheck fetches and checks upstream source status. If targetURL is provided, only that source is checked.
func (s *Service) UpstreamCheck(ctx context.Context, targetURL string, onProgress ...progress.Func) ([]model.UpstreamInfo, error) {
	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return nil, err
	}

	var skillIDs []string
	if targetURL != "" {
		for id, rec := range cat.Skills {
			u := rec.Source.URL
			if u == "" && rec.Source.Type == model.SourceTypeLocal {
				u = rec.Source.Path
			}
			if u == targetURL {
				skillIDs = append(skillIDs, id)
			}
		}
		if len(skillIDs) == 0 {
			if strings.HasPrefix(targetURL, "http://") || strings.HasPrefix(targetURL, "https://") ||
				strings.HasPrefix(targetURL, "git@") || strings.HasPrefix(targetURL, "ssh://") ||
				strings.HasSuffix(targetURL, ".git") {
				_, _ = s.gitSource.EnsureRepo(ctx, targetURL, true, onProgress...)
			}
		}
	}

	statuses, err := s.Check(ctx, skillIDs, onProgress...)
	if err != nil {
		return nil, err
	}

	return s.buildUpstreamList(ctx, statuses)
}

// UpstreamDiscover discovers all skills available in the upstream repository or directory.
func (s *Service) UpstreamDiscover(ctx context.Context, targetURL, ref string, onProgress ...progress.Func) ([]git.DiscoveredSkill, error) {
	isGit := strings.HasPrefix(targetURL, "http://") ||
		strings.HasPrefix(targetURL, "https://") ||
		strings.HasPrefix(targetURL, "git@") ||
		strings.HasPrefix(targetURL, "ssh://") ||
		strings.HasSuffix(targetURL, ".git")

	if !isGit {
		expanded, err := fsx.ExpandUser(targetURL)
		if err == nil && fsx.DirExists(expanded) {
			var discovered []git.DiscoveredSkill
			err := filepath.Walk(expanded, func(path string, info os.FileInfo, walkErr error) error {
				if walkErr != nil || info == nil || info.IsDir() {
					return nil
				}
				if filepath.Base(path) == "SKILL.md" {
					subDir := filepath.Dir(path)
					meta, err := skill.ValidateSkillDir(subDir, "")
					if err == nil && meta.Name != "" {
						rel, relErr := filepath.Rel(expanded, subDir)
						if relErr != nil {
							rel = meta.Name
						}
						discovered = append(discovered, git.DiscoveredSkill{
							ID:          meta.Name,
							Path:        rel,
							Description: meta.Description,
						})
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			return discovered, nil
		}
	}

	return s.DiscoverGit(ctx, targetURL, ref, onProgress...)
}

// UpstreamPull pulls or updates skills from upstream into the managed workspace store.
func (s *Service) UpstreamPull(ctx context.Context, targetURL, ref string, subpaths []string, aliases map[string]string, overwriteSkills map[string]bool, force bool, onProgress ...progress.Func) (*BatchAddResult, error) {
	return s.AddGitBatchWithOptions(ctx, targetURL, ref, subpaths, aliases, overwriteSkills, true, force, true, onProgress...)
}

// Deploy deploys a managed skill to a target.
func (s *Service) Deploy(ctx context.Context, skillID, targetName string, force bool) error {
	return s.DeployGlobal(ctx, skillID, targetName, force)
}

// Undeploy removes a deployed skill from a target.
func (s *Service) Undeploy(ctx context.Context, skillID, targetName string, force bool) error {
	return s.UndeployGlobal(ctx, skillID, targetName, force)
}

// DeployGlobal deploys a managed skill to a global program directory.
func (s *Service) DeployGlobal(ctx context.Context, skillID, appName string, force bool) error {
	appName = strings.ToLower(strings.TrimSpace(appName))
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}

	tgt, globalPath, err := s.configMgr.GetTarget(appName)
	if err != nil || globalPath == "" {
		return fmt.Errorf("target %q not configured (use 'asoul target add %s <config_dir>' or check config.jsonc)", appName, appName)
	}
	if !tgt.IsEnabled() && !force {
		return fmt.Errorf("target %q is disabled in configuration (use --force to deploy anyway)", appName)
	}

	return s.deployer.DeployGlobal(ctx, s.workspaceRoot, appName, globalPath, skillID, force)
}

// UndeployGlobal removes a deployed skill from a global program directory.
func (s *Service) UndeployGlobal(ctx context.Context, skillID, appName string, force bool) error {
	appName = strings.ToLower(strings.TrimSpace(appName))
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}

	_, globalPath, err := s.configMgr.GetTarget(appName)
	if err != nil || globalPath == "" {
		return fmt.Errorf("target %q not configured", appName)
	}

	return s.deployer.UndeployGlobal(ctx, appName, globalPath, skillID, force)
}

// DeployProject deploys a managed skill to a project directory with specified formats.
func (s *Service) DeployProject(ctx context.Context, skillID, projectPath string, formats []string, force bool) ([]string, bool, error) {
	if projectPath == "" {
		return nil, false, fmt.Errorf("project path cannot be empty")
	}

	expanded, err := fsx.ExpandUser(projectPath)
	if err != nil {
		return nil, false, err
	}

	absPath, err := filepath.Abs(expanded)
	if err != nil {
		absPath = filepath.Clean(expanded)
	}

	if !fsx.DirExists(absPath) {
		return nil, false, fmt.Errorf("project directory does not exist: %s", absPath)
	}

	if s.configMgr != nil && !force {
		if en, _ := s.configMgr.IsProjectEnabled(absPath); !en {
			return nil, false, fmt.Errorf("project %q is disabled in configuration (use --force to deploy anyway)", absPath)
		}
	}

	// Cache project path
	if s.configMgr != nil {
		_ = s.configMgr.AddRecentProject(absPath)
	}

	return s.deployer.DeployProject(ctx, s.workspaceRoot, absPath, formats, skillID, force)
}

// UndeployProject removes a deployed skill from a project directory.
func (s *Service) UndeployProject(ctx context.Context, skillID, projectPath string, formats []string, force bool) ([]string, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("project path cannot be empty")
	}

	expanded, err := fsx.ExpandUser(projectPath)
	if err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(expanded)
	if err != nil {
		absPath = filepath.Clean(expanded)
	}

	if !fsx.DirExists(absPath) {
		return nil, fmt.Errorf("project directory does not exist: %s", absPath)
	}

	return s.deployer.UndeployProject(ctx, absPath, formats, skillID, force)
}

// DiffTarget generates a unified diff between the workspace managed skill and a target skill directory.
func (s *Service) DiffTarget(ctx context.Context, skillID, targetName, projectPath, format string) (string, error) {
	if err := skill.ValidateID(skillID); err != nil {
		return "", err
	}
	managedDir := catalog.SkillDir(s.workspaceRoot, skillID)
	if !fsx.DirExists(managedDir) {
		return "", fmt.Errorf("managed skill directory %s not found", managedDir)
	}

	var targetSkillDir string
	if projectPath != "" {
		expanded, err := fsx.ExpandUser(projectPath)
		if err != nil {
			return "", err
		}
		absProject, err := filepath.Abs(expanded)
		if err != nil {
			return "", err
		}
		if format == "" {
			format = model.FormatStandard
		}
		fDef, ok := model.LookupFormat(format)
		if !ok {
			return "", fmt.Errorf("unsupported format %q", format)
		}
		targetBase := filepath.Join(absProject, fDef.Subpath)
		targetSkillDir, err = deploy.SafeSkillPath(targetBase, skillID)
		if err != nil {
			return "", err
		}
	} else {
		targetName = strings.ToLower(strings.TrimSpace(targetName))
		if s.configMgr == nil {
			return "", fmt.Errorf("config manager not initialized")
		}
		_, globalPath, err := s.configMgr.GetTarget(targetName)
		if err != nil || globalPath == "" {
			return "", fmt.Errorf("target %q not configured", targetName)
		}
		targetSkillDir, err = deploy.SafeSkillPath(globalPath, skillID)
		if err != nil {
			return "", err
		}
	}

	if !fsx.DirExists(targetSkillDir) {
		return "", fmt.Errorf("target skill directory does not exist: %s", targetSkillDir)
	}

	return s.gitClient.DiffNoIndex(ctx, managedDir, targetSkillDir)
}

// AdoptFromTarget syncs modifications made in a target skill directory back into the workspace store,
// safely updating the workspace catalog and state records.
func (s *Service) AdoptFromTarget(ctx context.Context, skillID, targetName, projectPath, format string) error {
	if err := skill.ValidateID(skillID); err != nil {
		return err
	}

	if s.locker != nil {
		unlockWS, err := s.locker.Lock(ctx, 10*time.Second)
		if err != nil {
			return fmt.Errorf("failed to lock workspace: %w", err)
		}
		defer unlockWS()
	}

	var targetBase string
	var targetSkillDir string
	var isProject bool
	var relDest string

	if projectPath != "" {
		isProject = true
		expanded, err := fsx.ExpandUser(projectPath)
		if err != nil {
			return err
		}
		absProject, err := filepath.Abs(expanded)
		if err != nil {
			return err
		}
		if format == "" {
			format = model.FormatStandard
		}
		fDef, ok := model.LookupFormat(format)
		if !ok {
			return fmt.Errorf("unsupported format %q", format)
		}
		targetBase = filepath.Join(absProject, fDef.Subpath)
		targetSkillDir, err = deploy.SafeSkillPath(targetBase, skillID)
		if err != nil {
			return err
		}
		relDest = filepath.Join(fDef.Subpath, skillID)
	} else {
		targetName = strings.ToLower(strings.TrimSpace(targetName))
		if s.configMgr == nil {
			return fmt.Errorf("config manager not initialized")
		}
		_, globalPath, err := s.configMgr.GetTarget(targetName)
		if err != nil || globalPath == "" {
			return fmt.Errorf("target %q not configured", targetName)
		}
		targetBase = globalPath
		targetSkillDir, err = deploy.SafeSkillPath(globalPath, skillID)
		if err != nil {
			return err
		}
	}

	if !fsx.DirExists(targetSkillDir) {
		return fmt.Errorf("target skill directory does not exist: %s", targetSkillDir)
	}

	if _, err := skill.ValidateSkillDir(targetSkillDir, skillID); err != nil {
		return fmt.Errorf("target skill directory is invalid: %w", err)
	}

	unlockTarget, err := deploy.LockTarget(ctx, targetBase)
	if err != nil {
		return err
	}
	defer unlockTarget()

	newHash, err := hash.HashDir(targetSkillDir)
	if err != nil {
		return fmt.Errorf("failed to hash target skill: %w", err)
	}

	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		return fmt.Errorf("failed to load catalog: %w", err)
	}

	rec, exists := cat.Skills[skillID]
	if !exists {
		return fmt.Errorf("skill %q not found in workspace catalog", skillID)
	}

	managedDir := catalog.SkillDir(s.workspaceRoot, skillID)
	skillsBase := catalog.SkillsDir(s.workspaceRoot)

	swap, err := deploy.PromoteDirectory(targetSkillDir, managedDir, skillsBase, skillID)
	if err != nil {
		return fmt.Errorf("failed to promote target skill to workspace: %w", err)
	}

	rec.Resolved.ContentHash = newHash
	cat.Skills[skillID] = rec

	if err := catalog.Save(s.workspaceRoot, cat); err != nil {
		_ = swap.Rollback()
		return fmt.Errorf("failed to update catalog: %w", err)
	}

	if isProject {
		if err := state.RecordProjectDeployment(projectPath, skillID, format, relDest, newHash); err != nil {
			_ = swap.Rollback()
			return fmt.Errorf("failed to record project deployment: %w", err)
		}
	} else {
		targetCanon := fsx.CanonicalPath(targetBase)
		wsCanon := fsx.CanonicalPath(s.workspaceRoot)
		if err := s.stateMgr.RecordDeployment(targetName, skillID, newHash, targetCanon, wsCanon); err != nil {
			_ = swap.Rollback()
			return fmt.Errorf("failed to record deployment state: %w", err)
		}
	}

	return swap.Commit()
}

// ProjectList returns cached project paths.
func (s *Service) ProjectList() ([]string, error) {
	if s.configMgr == nil {
		return nil, nil
	}
	return s.configMgr.GetRecentProjects()
}

// ProjectAdd validates and adds a project path to cache.
func (s *Service) ProjectAdd(path string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	expanded, err := fsx.ExpandUser(path)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(expanded)
	if err != nil {
		absPath = filepath.Clean(expanded)
	}
	if !fsx.DirExists(absPath) {
		return fmt.Errorf("project directory does not exist: %s", absPath)
	}
	return s.configMgr.AddRecentProject(absPath)
}

// ProjectRemove removes a cached project path.
func (s *Service) ProjectRemove(path string) error {
	if s.configMgr == nil {
		return nil
	}
	return s.configMgr.RemoveRecentProject(path)
}

// ProjectStatus returns the deployment state of a project, reconciling stale records against disk.
func (s *Service) ProjectStatus(projectPath string) (*model.ProjectState, error) {
	expanded, err := fsx.ExpandUser(projectPath)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(expanded)
	if err != nil {
		absPath = filepath.Clean(expanded)
	}

	_, _ = state.PruneStaleProjectDeployments(absPath, func(skID, format string, rec model.ProjectDeploymentRecord) bool {
		dest := rec.DestPath
		if dest == "" {
			fDef, ok := model.LookupFormat(format)
			if ok {
				dest = filepath.Join(absPath, fDef.Subpath, skID)
			}
		} else if !filepath.IsAbs(dest) {
			dest = filepath.Join(absPath, dest)
		}
		return dest != "" && fsx.DirExists(dest)
	})

	return state.LoadProjectState(absPath)
}

// Target operations delegating to configMgr
func (s *Service) TargetAdd(name, path string) error {
	return s.configMgr.AddTargetSimple(name, path)
}

func (s *Service) TargetAddConfig(name string, target model.TargetConfig) error {
	return s.configMgr.AddTarget(name, target)
}

func (s *Service) TargetRemove(name string) error {
	return s.configMgr.RemoveTarget(name)
}

func (s *Service) TargetList() (map[string]model.TargetConfig, error) {
	cfg, err := s.configMgr.Load()
	if err != nil {
		return nil, err
	}
	return cfg.Targets, nil
}

// Config returns the loaded user configuration.
func (s *Service) Config() (*model.Config, error) {
	if s.configMgr == nil {
		return nil, nil
	}
	return s.configMgr.Load()
}

// Profiles returns the profiles defined in user configuration.
func (s *Service) Profiles() map[string]model.ProfileConfig {
	cfg, err := s.configMgr.Load()
	if err != nil || cfg.Profiles == nil {
		return nil
	}
	return cfg.Profiles
}

// GroupList returns all configured skill groups.
func (s *Service) GroupList() (map[string]model.ProfileConfig, error) {
	if s.configMgr == nil {
		return make(map[string]model.ProfileConfig), nil
	}
	return s.configMgr.GroupList()
}

// GroupGet returns a group configuration by name.
func (s *Service) GroupGet(name string) (*model.ProfileConfig, error) {
	if s.configMgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.GroupGet(name)
}

// GroupCreate creates a new empty skill group.
func (s *Service) GroupCreate(name string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.GroupCreate(name)
}

// GroupDelete removes a skill group.
func (s *Service) GroupDelete(name string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.GroupDelete(name)
}

// GroupAddSkill adds a skill to a group.
func (s *Service) GroupAddSkill(name, skillID string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.GroupAddSkill(name, skillID)
}

// GroupRemoveSkill removes a skill from a group.
func (s *Service) GroupRemoveSkill(name, skillID string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.GroupRemoveSkill(name, skillID)
}

// GroupSetSkills sets all skill IDs for a group.
func (s *Service) GroupSetSkills(name string, skillIDs []string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.GroupSetSkills(name, skillIDs)
}

func (s *Service) TargetShow(name string) (*model.TargetConfig, string, error) {
	return s.configMgr.GetTarget(name)
}

// TargetDeployedSkills returns the list of deployed skill IDs for a given target,
// reconciling records against disk reality and pruning any stale records.
func (s *Service) TargetDeployedSkills(name string) []string {
	var skillsDir string
	if s.configMgr != nil {
		tgt, globalPath, err := s.configMgr.GetTarget(name)
		if err == nil && tgt != nil {
			skillsDir = globalPath
			if skillsDir == "" {
				skillsDir = tgt.SkillsDir()
			}
		}
	}

	if skillsDir == "" && s.stateMgr != nil {
		if st, _ := s.stateMgr.Load(); st != nil && st.Targets != nil {
			if skMap, ok := st.Targets[name]; ok {
				for _, rec := range skMap {
					if rec.TargetPath != "" {
						skillsDir = rec.TargetPath
						break
					}
				}
			}
		}
	}

	// Prune stale records if skillsDir is known
	if s.stateMgr != nil && skillsDir != "" {
		_ = s.stateMgr.PruneStaleDeployments(name, func(skID string, rec model.DeploymentRecord) bool {
			destDir := ""
			if rec.TargetPath != "" && fsx.DirExists(rec.TargetPath) {
				if filepath.Base(rec.TargetPath) == skID {
					destDir = rec.TargetPath
				} else {
					destDir = filepath.Join(rec.TargetPath, skID)
				}
			} else {
				destDir = filepath.Join(skillsDir, skID)
			}
			return destDir != "" && fsx.DirExists(destDir)
		})
	}

	skillSet := make(map[string]bool)

	// 1. Collect from disk in skillsDir
	if skillsDir != "" && fsx.DirExists(skillsDir) {
		entries, _ := os.ReadDir(skillsDir)
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				skillSet[e.Name()] = true
			}
		}
	}

	// 2. Collect from deployment state (for any valid records)
	if s.stateMgr != nil {
		st, err := s.stateMgr.Load()
		if err == nil && st != nil && st.Targets != nil {
			if skMap, ok := st.Targets[name]; ok {
				for skID, rec := range skMap {
					destDir := ""
					if rec.TargetPath != "" && fsx.DirExists(rec.TargetPath) {
						if filepath.Base(rec.TargetPath) == skID {
							destDir = rec.TargetPath
						} else {
							destDir = filepath.Join(rec.TargetPath, skID)
						}
					} else if skillsDir != "" {
						destDir = filepath.Join(skillsDir, skID)
					}
					if destDir != "" && fsx.DirExists(destDir) {
						skillSet[skID] = true
					}
				}
			}
		}
	}

	res := make([]string, 0, len(skillSet))
	for skID := range skillSet {
		res = append(res, skID)
	}
	sort.Strings(res)
	return res
}

// ProjectDeployedSkills returns the list of deployed skill IDs for a project directory,
// reconciling records against disk reality and pruning any stale records.
func (s *Service) ProjectDeployedSkills(projectRoot string) []string {
	expanded, err := fsx.ExpandUser(projectRoot)
	if err == nil {
		projectRoot = expanded
	}
	absPath, err := filepath.Abs(projectRoot)
	if err == nil {
		projectRoot = absPath
	}

	// Prune stale records in project state
	pst, _ := state.PruneStaleProjectDeployments(projectRoot, func(skID, format string, rec model.ProjectDeploymentRecord) bool {
		dest := rec.DestPath
		if dest == "" {
			fDef, ok := model.LookupFormat(format)
			if ok {
				dest = filepath.Join(projectRoot, fDef.Subpath, skID)
			}
		} else if !filepath.IsAbs(dest) {
			dest = filepath.Join(projectRoot, dest)
		}
		return dest != "" && fsx.DirExists(dest)
	})

	skillSet := make(map[string]bool)

	// 1. Scan known project format directories on disk
	for _, fDef := range model.BuiltinFormats {
		targetBase := filepath.Join(projectRoot, fDef.Subpath)
		if fsx.DirExists(targetBase) {
			entries, _ := os.ReadDir(targetBase)
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					skillSet[e.Name()] = true
				}
			}
		}
	}

	// 2. Add tracked skills from project state
	if pst != nil && pst.Skills != nil {
		for skID := range pst.Skills {
			skillSet[skID] = true
		}
	}

	res := make([]string, 0, len(skillSet))
	for skID := range skillSet {
		res = append(res, skID)
	}
	sort.Strings(res)
	return res
}

// SetTargetEnabled sets whether a target is enabled in configuration.
func (s *Service) SetTargetEnabled(name string, enabled bool) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.SetTargetEnabled(name, enabled)
}

// SetProjectEnabled sets whether a project is enabled in configuration.
func (s *Service) SetProjectEnabled(path string, enabled bool) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.SetProjectEnabled(path, enabled)
}

// IsTargetEnabled returns whether a target is enabled in configuration.
func (s *Service) IsTargetEnabled(name string) bool {
	if s.configMgr == nil {
		return true
	}
	tgt, _, err := s.configMgr.GetTarget(name)
	if err != nil || tgt == nil {
		return true
	}
	return tgt.IsEnabled()
}

// IsProjectEnabled returns whether a project is enabled in configuration.
func (s *Service) IsProjectEnabled(path string) bool {
	if s.configMgr == nil {
		return true
	}
	en, err := s.configMgr.IsProjectEnabled(path)
	if err != nil {
		return true
	}
	return en
}

// Cache operations delegating to cacheMgr
func (s *Service) CacheList() ([]cache.CacheEntry, error) {
	return s.cacheMgr.List()
}

func (s *Service) CachePrune(ctx context.Context) (int, int64, error) {
	activeKeys := make(map[string]bool)
	workspaceRoots := s.Workspaces()
	if s.workspaceRoot != "" {
		workspaceRoots = append(workspaceRoots, s.workspaceRoot)
	}
	for _, root := range workspaceRoots {
		canon := fsx.CanonicalPath(root)
		if canon == "" {
			continue
		}
		cat, err := catalog.Load(canon)
		if err == nil {
			for _, rec := range cat.Skills {
				if rec.Source.Type == model.SourceTypeGit && rec.Source.URL != "" {
					key := cache.KeyForURL(rec.Source.URL)
					activeKeys[key] = true
				}
			}
		}
	}
	return s.cacheMgr.Prune(activeKeys)
}

func (s *Service) CacheClean() (int, int64, error) {
	return s.cacheMgr.Clean()
}

// ConfigPath returns the path to the configuration file.
func (s *Service) ConfigPath() string {
	if s.configMgr == nil {
		return ""
	}
	return s.configMgr.Path()
}

// ConfigExists checks if the configuration file exists.
func (s *Service) ConfigExists() bool {
	if s.configMgr == nil {
		return false
	}
	return s.configMgr.Exists()
}

// ConfigSave saves user configuration under the config file lock.
func (s *Service) ConfigSave(cfg *model.Config) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.Update(func(current *model.Config) error {
		*current = *cfg
		return nil
	})
}

// ConfigSet sets a configuration key value and saves it.
func (s *Service) ConfigSet(key, val string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return s.configMgr.Update(func(cfg *model.Config) error {
		switch key {
		case "default_root":
			cfg.DefaultRoot = val
		case "language":
			cfg.Language = val
		case "model_enrich_mode":
			if val != model.ModelEnrichModeRemote && val != model.ModelEnrichModeIncremental {
				return fmt.Errorf("invalid model enrich mode %q", val)
			}
			cfg.SetModelEnrichMode(val)
		default:
			return fmt.Errorf("cannot set unsupported config key %q", key)
		}
		return nil
	})
}

// ConfigResetDefaults resets the user configuration to recommended defaults.
func (s *Service) ConfigResetDefaults() error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	return config.CreateDefaultConfig(s.configMgr)
}

// SwitchWorkspaceRoot switches the active workspace root.
func (s *Service) SwitchWorkspaceRoot(root string) error {
	canon := fsx.CanonicalPath(root)
	if canon == "" {
		return fmt.Errorf("invalid workspace path")
	}
	s.SetWorkspaceRoot(canon)
	return nil
}

// WorkspaceInitialized checks if the current workspace root has catalog.json.
func (s *Service) WorkspaceInitialized() bool {
	if s.workspaceRoot == "" {
		return false
	}
	return fsx.FileExists(catalog.CatalogPath(s.workspaceRoot))
}

// Workspaces returns the list of registered workspaces from configuration, without duplicates.
// Workspaces are only added through explicit user action (CLI command or TUI).
func (s *Service) Workspaces() []string {
	var list []string
	if s.configMgr != nil {
		list = s.configMgr.GetWorkspaces()
	}
	if len(list) == 0 {
		return []string{config.DefaultWorkspacePathCompact()}
	}
	// Deduplicate strictly by SamePath
	var deduped []string
	for _, w := range list {
		norm := fsx.NormalizeWorkspacePath(w)
		already := false
		for _, ex := range deduped {
			if fsx.SamePath(ex, norm) {
				already = true
				break
			}
		}
		if !already {
			deduped = append(deduped, norm)
		}
	}
	return deduped
}

// AddWorkspace adds a workspace to configuration.
func (s *Service) AddWorkspace(path string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	norm := fsx.NormalizeWorkspacePath(path)
	return s.configMgr.AddWorkspace(norm)
}

// RemoveWorkspace removes a workspace from configuration.
func (s *Service) RemoveWorkspace(path string) error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	if err := s.configMgr.RemoveWorkspace(path); err != nil {
		return err
	}
	// If the removed workspace was the current active workspace, switch to remaining or default
	cfg, _ := s.configMgr.Load()
	if cfg != nil && cfg.DefaultRoot != "" {
		_ = s.SwitchWorkspaceRoot(cfg.DefaultRoot)
	}
	return nil
}

// SelectWorkspace activates a workspace and sets it as the default.
func (s *Service) SelectWorkspace(path string) error {
	norm := fsx.NormalizeWorkspacePath(path)
	if err := s.SwitchWorkspaceRoot(norm); err != nil {
		return err
	}
	if s.configMgr != nil {
		if err := s.configMgr.Update(func(cfg *model.Config) error {
			cfg.DefaultRoot = norm
			found := false
			for _, w := range cfg.Workspaces {
				if fsx.SamePath(w, norm) {
					found = true
					break
				}
			}
			if !found {
				cfg.Workspaces = append(cfg.Workspaces, norm)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// WorkspacePathInitialized checks if a specific workspace path has catalog.json.
func (s *Service) WorkspacePathInitialized(path string) bool {
	canon := fsx.CanonicalPath(path)
	if canon == "" {
		return false
	}
	return fsx.FileExists(catalog.CatalogPath(canon))
}

// InitWorkspacePath initializes catalog.json for a specific workspace path.
func (s *Service) InitWorkspacePath(path string) error {
	canon := fsx.CanonicalPath(path)
	if canon == "" {
		return fmt.Errorf("invalid workspace path")
	}
	if err := os.MkdirAll(canon, 0755); err != nil {
		return err
	}
	return catalog.InitWorkspace(canon)
}

// EnrichModelConfig orchestrates model enrichment for a specified agent.
func (s *Service) EnrichModelConfig(ctx context.Context, agentName string, opts agent.EnrichOptions) (*agent.EnrichSummary, error) {
	if agentName == "" {
		agentName = "opencode"
	}

	var adapter agent.AgentAdapter
	adapter, err := agent.NewAdapter(agentName)
	if err != nil {
		return nil, err
	}

	opts.Workspace = s.workspaceRoot

	// Reuse config_dir from target if not explicitly provided
	if opts.ConfigDir == "" && s.configMgr != nil {
		if cfg, err := s.configMgr.Load(); err == nil && cfg != nil {
			for name, tgt := range cfg.Targets {
				if tgt.Channel == agentName || name == agentName {
					opts.ConfigDir = tgt.ResolveConfigDir()
					break
				}
			}
		}
	}

	// Load backup retention limit from config if not explicitly provided
	if opts.BackupMaxVersions == nil && s.configMgr != nil {
		if cfg, err := s.configMgr.Load(); err == nil {
			opts.BackupMaxVersions = cfg.GetBackupMaxVersions()
		}
	}

	// 1. Fetch models.dev catalog unless only applying custom rules
	var matcher *modelsdev.Matcher
	if !opts.RulesOnly {
		devClient, err := modelsdev.NewClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create models.dev client: %w", err)
		}
		cat, err := devClient.FetchCatalog(ctx, opts.RefreshCache)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch models.dev catalog: %w", err)
		}
		officialMap, _, _, _ := s.GetOfficialProviders()
		matcher = modelsdev.NewMatcher(cat, officialMap)
	}

	// 2. Load agent rules
	rules, rulesPath, err := s.GetModelRules(agentName, opts.RulesPath)
	if err != nil {
		return nil, err
	}
	if opts.RulesOnly && len(rules) == 0 {
		hint := ""
		if rulesPath == "" {
			hint = fmt.Sprintf(" (no rule file found for channel %q)", agentName)
		}
		return nil, fmt.Errorf("rules-only mode requires a rule file, but none is configured%s", hint)
	}

	// 3. Delegate to agent adapter
	return adapter.Enrich(ctx, matcher, rules, opts)
}

// QueryModel queries metadata and close suggestions for a model from models.dev.
func (s *Service) QueryModel(ctx context.Context, modelID string, refresh bool) (*modelsdev.ModelData, []modelsdev.ModelData, error) {
	devClient, err := modelsdev.NewClient()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create models.dev client: %w", err)
	}
	cat, err := devClient.FetchCatalog(ctx, refresh)
	if err != nil {
		return nil, nil, err
	}
	officialMap, _, _, _ := s.GetOfficialProviders()
	matcher := modelsdev.NewMatcher(cat, officialMap)

	if m, found := matcher.FindModel(modelID); found {
		return m, nil, nil
	}
	suggestions := matcher.SuggestCandidates(modelID, 5)
	return nil, suggestions, nil
}

// GetOfficialProviders returns the active official providers map and metadata.
func (s *Service) GetOfficialProviders() (providers map[string][]string, configPath string, isCustom bool, err error) {
	if s.configMgr == nil {
		return model.DefaultOfficialProviders(), "", false, nil
	}
	configPath = s.configMgr.Path()
	cfg, err := s.configMgr.Load()
	if err != nil {
		return model.DefaultOfficialProviders(), configPath, false, err
	}
	if cfg != nil && cfg.Model != nil {
		if cfg.Model.OfficialProviders != nil {
			return cfg.Model.OfficialProviders, configPath, true, nil
		}
		if cfg.Model.AuthoritativeProviders != nil {
			return cfg.Model.AuthoritativeProviders, configPath, true, nil
		}
	}
	return model.DefaultOfficialProviders(), configPath, false, nil
}

// GetAuthoritativeProviders is a backwards-compatible alias for GetOfficialProviders.
func (s *Service) GetAuthoritativeProviders() (providers map[string][]string, configPath string, isCustom bool, err error) {
	return s.GetOfficialProviders()
}

// InitOfficialProviders persists the default official providers into config.jsonc.
func (s *Service) InitOfficialProviders() error {
	if s.configMgr == nil {
		return fmt.Errorf("config manager not initialized")
	}
	cfg, err := s.configMgr.Load()
	if err != nil {
		return err
	}
	if cfg.Model == nil {
		cfg.Model = &model.ModelConfig{}
	}
	cfg.Model.OfficialProviders = model.DefaultOfficialProviders()
	cfg.Model.AuthoritativeProviders = nil // clean legacy field
	return s.configMgr.Save(cfg)
}

// InitAuthoritativeProviders is a backwards-compatible alias for InitOfficialProviders.
func (s *Service) InitAuthoritativeProviders() error {
	return s.InitOfficialProviders()
}

// GetModelRules loads rules for the specified channel. The channel is selected
// by the rule file name (<channel>.json|jsonc) or by an explicit rules path.
// Returns empty rules if no rule file is configured.
func (s *Service) GetModelRules(agentName string, explicitPath string) ([]modelrules.Rule, string, error) {
	ruleFile, err := modelrules.FindRuleFile(agentName, explicitPath, s.workspaceRoot)
	if err != nil {
		return nil, "", err
	}

	if ruleFile == "" {
		return nil, "", nil
	}

	rf, err := modelrules.LoadRuleFile(ruleFile)
	if err != nil {
		return nil, "", err
	}

	return rf.Rules, ruleFile, nil
}

// AgentModelItem represents a model entry displayed in TUI or CLI.
type AgentModelItem struct {
	ProviderID       string
	ModelID          string
	Name             string
	ContextLimit     int
	InputLimit       int
	OutputLimit      int
	PriceInput       float64
	PriceOutput      float64
	PriceCacheRead   float64
	PriceCacheWrite  float64
	Reasoning        bool
	ToolCall         bool
	Attachment       bool
	StructuredOutput bool
	Temperature      bool
	Variants         []string
	ModalitiesInput  []string
	ModalitiesOutput []string
	IsEnriched       bool
	SourceProvider   string
	RawJSON          string
}

// ListAgentModels reads the agent's configuration and extracts custom models.
func (s *Service) ListAgentModels(ctx context.Context, agentName string) ([]AgentModelItem, string, error) {
	if agentName == "" {
		agentName = "opencode"
	}
	adapter, err := agent.NewAdapter(agentName)
	if err != nil {
		return nil, "", err
	}

	var customConfigDir string
	if s.configMgr != nil {
		if cfg, err := s.configMgr.Load(); err == nil && cfg != nil {
			for name, tgt := range cfg.Targets {
				if tgt.Channel == agentName || name == agentName {
					customConfigDir = tgt.ResolveConfigDir()
					break
				}
			}
		}
	}

	files, err := adapter.DetectConfigFiles(s.workspaceRoot, customConfigDir)
	if err != nil {
		return nil, "", err
	}
	if len(files) == 0 {
		return nil, "", nil
	}
	targetFile := files[0]

	data, err := os.ReadFile(targetFile)
	if err != nil {
		return nil, targetFile, err
	}

	var root map[string]any
	if err := jsonc.Unmarshal(data, &root); err != nil {
		return nil, targetFile, err
	}

	providers, ok := root["provider"].(map[string]any)
	if !ok {
		return nil, targetFile, nil
	}

	// Try loading cached catalog to identify source provider
	var matcher *modelsdev.Matcher
	if devClient, err := modelsdev.NewClient(); err == nil {
		if cat, err := devClient.ReadCachedCatalog(); err == nil && cat != nil {
			officialMap, _, _, _ := s.GetOfficialProviders()
			matcher = modelsdev.NewMatcher(cat, officialMap)
		}
	}

	var items []AgentModelItem
	for pID, pVal := range providers {
		pMap, ok := pVal.(map[string]any)
		if !ok {
			continue
		}
		models, ok := pMap["models"].(map[string]any)
		if !ok {
			continue
		}

		for mID, mVal := range models {
			item := AgentModelItem{
				ProviderID: pID,
				ModelID:    mID,
			}
			if matcher != nil {
				if devModel, found := matcher.FindModel(mID); found {
					item.SourceProvider = devModel.ProviderID
				}
			}
			if mVal != nil {
				if rawBytes, err := json.MarshalIndent(mVal, "", "  "); err == nil {
					item.RawJSON = string(rawBytes)
				}
				if mObj, ok := mVal.(map[string]any); ok {
					if n, ok := mObj["name"].(string); ok {
						item.Name = n
					}
					if l, ok := mObj["limit"].(map[string]any); ok {
						if c, ok := l["context"].(float64); ok {
							item.ContextLimit = int(c)
						}
						if inLimit, ok := l["input"].(float64); ok {
							item.InputLimit = int(inLimit)
						}
						if o, ok := l["output"].(float64); ok {
							item.OutputLimit = int(o)
						}
					}
					if c, ok := mObj["cost"].(map[string]any); ok {
						if in, ok := c["input"].(float64); ok {
							item.PriceInput = in
						}
						if out, ok := c["output"].(float64); ok {
							item.PriceOutput = out
						}
						if cr, ok := c["cache_read"].(float64); ok {
							item.PriceCacheRead = cr
						}
						if cw, ok := c["cache_write"].(float64); ok {
							item.PriceCacheWrite = cw
						}
					}
					if r, ok := mObj["reasoning"].(bool); ok {
						item.Reasoning = r
					}
					if t, ok := mObj["tool_call"].(bool); ok {
						item.ToolCall = t
					}
					if a, ok := mObj["attachment"].(bool); ok {
						item.Attachment = a
					}
					if s, ok := mObj["structured_output"].(bool); ok {
						item.StructuredOutput = s
					}
					if temp, ok := mObj["temperature"].(bool); ok {
						item.Temperature = temp
					}
					if mod, ok := mObj["modalities"].(map[string]any); ok {
						if inMods, ok := mod["input"].([]any); ok {
							for _, im := range inMods {
								if s, ok := im.(string); ok {
									item.ModalitiesInput = append(item.ModalitiesInput, s)
								}
							}
						}
						if outMods, ok := mod["output"].([]any); ok {
							for _, om := range outMods {
								if s, ok := om.(string); ok {
									item.ModalitiesOutput = append(item.ModalitiesOutput, s)
								}
							}
						}
					}
					if v, ok := mObj["variants"].(map[string]any); ok {
						for vk := range v {
							item.Variants = append(item.Variants, vk)
						}
						sort.Strings(item.Variants)
					}
				}
			}
			if item.ContextLimit > 0 || item.PriceInput > 0 || len(item.Variants) > 0 || item.Reasoning || item.ToolCall || item.Attachment || item.StructuredOutput {
				item.IsEnriched = true
			}
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].ProviderID != items[j].ProviderID {
			return items[i].ProviderID < items[j].ProviderID
		}
		return items[i].ModelID < items[j].ModelID
	})

	return items, targetFile, nil
}

// AgentProviderItem represents a provider entry and its key settings.
type AgentProviderItem struct {
	ProviderID string
	Name       string
	NPM        string
	Options    map[string]any
	ModelCount int
	RawJSON    string
}

// resolveAgentConfigFile detects the active configuration file for a channel.
func (s *Service) resolveAgentConfigFile(agentName string) (string, error) {
	if agentName == "" {
		agentName = "opencode"
	}
	adapter, err := agent.NewAdapter(agentName)
	if err != nil {
		return "", err
	}

	var customConfigDir string
	if s.configMgr != nil {
		if cfg, err := s.configMgr.Load(); err == nil && cfg != nil {
			for name, tgt := range cfg.Targets {
				if tgt.Channel == agentName || name == agentName {
					customConfigDir = tgt.ResolveConfigDir()
					break
				}
			}
		}
	}

	files, err := adapter.DetectConfigFiles(s.workspaceRoot, customConfigDir)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	return files[0], nil
}

// ListAgentProviders extracts provider-level entries from the agent configuration.
func (s *Service) ListAgentProviders(ctx context.Context, agentName string) ([]AgentProviderItem, string, error) {
	cfgFile, err := s.resolveAgentConfigFile(agentName)
	if err != nil {
		return nil, "", err
	}
	if cfgFile == "" {
		return nil, "", nil
	}

	data, err := os.ReadFile(cfgFile)
	if err != nil {
		return nil, cfgFile, err
	}
	var root map[string]any
	if err := jsonc.Unmarshal(data, &root); err != nil {
		return nil, cfgFile, err
	}

	providers, ok := root["provider"].(map[string]any)
	if !ok {
		return nil, cfgFile, nil
	}

	var items []AgentProviderItem
	for pID, pVal := range providers {
		pMap, ok := pVal.(map[string]any)
		if !ok {
			continue
		}
		item := AgentProviderItem{ProviderID: pID}
		if n, ok := pMap["name"].(string); ok {
			item.Name = n
		}
		if npm, ok := pMap["npm"].(string); ok {
			item.NPM = npm
		}
		if opts, ok := pMap["options"].(map[string]any); ok {
			item.Options = opts
		}
		if models, ok := pMap["models"].(map[string]any); ok {
			item.ModelCount = len(models)
		}
		if raw, err := json.MarshalIndent(pVal, "", "  "); err == nil {
			item.RawJSON = string(raw)
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].ProviderID < items[j].ProviderID })
	return items, cfgFile, nil
}

// AgentConfigEdit is the result of previewing or applying a single config change.
type AgentConfigEdit struct {
	ConfigFile string
	BackupFile string
	Modified   bool
	DiffText   string
}

// PreviewAgentConfigValue computes the diff of setting (or removing) a config
// field without writing to disk.
func (s *Service) PreviewAgentConfigValue(ctx context.Context, agentName, cfgFile string, fieldPath []string, value any, remove bool) (*AgentConfigEdit, error) {
	return s.editAgentConfigValue(agentName, cfgFile, fieldPath, value, remove, true)
}

// ApplyAgentConfigValue writes the changed field after creating a backup.
func (s *Service) ApplyAgentConfigValue(ctx context.Context, agentName, cfgFile string, fieldPath []string, value any, remove bool) (*AgentConfigEdit, error) {
	return s.editAgentConfigValue(agentName, cfgFile, fieldPath, value, remove, false)
}

func (s *Service) editAgentConfigValue(agentName, cfgFile string, fieldPath []string, value any, remove, dryRun bool) (*AgentConfigEdit, error) {
	if len(fieldPath) == 0 {
		return nil, fmt.Errorf("empty config field path")
	}

	path, err := s.resolveAgentConfigPath(agentName, cfgFile)
	if err != nil {
		return nil, err
	}

	fileLock := flock.New(path + ".lock")
	ok, err := fileLock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("failed to lock config file %s: %w", path, err)
	}
	if !ok {
		return nil, fmt.Errorf("config file is locked by another process: %s", path)
	}
	defer func() { _ = fileLock.Unlock() }()

	doc, err := agent.LoadConfigDoc(path)
	if err != nil {
		return nil, err
	}
	raw := doc.Raw()

	root, err := doc.RootMap()
	if err != nil {
		return nil, err
	}

	if err := setNestedValue(root, fieldPath, value, remove); err != nil {
		return nil, err
	}

	if err := doc.Apply(root); err != nil {
		return nil, err
	}

	updated := doc.Bytes()
	modified := string(raw) != string(updated)
	edit := &AgentConfigEdit{ConfigFile: path, Modified: modified}
	if !modified {
		return edit, nil
	}
	if len(updated) > 0 && updated[len(updated)-1] != '\n' {
		updated = append(updated, '\n')
	}
	edit.DiffText = agent.UnifiedDiff(string(raw), string(updated))

	if dryRun {
		return edit, nil
	}

	var maxVersions *int
	if s.configMgr != nil {
		if cfg, err := s.configMgr.Load(); err == nil {
			maxVersions = cfg.GetBackupMaxVersions()
		}
	}
	backup, err := agent.CreateConfigFileBackup(path, raw, maxVersions)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup file: %w", err)
	}
	edit.BackupFile = backup

	if err := fsx.AtomicWriteFile(path, updated, doc.Perm()); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", path, err)
	}
	return edit, nil
}

// resolveAgentConfigPath resolves the config file to edit, honoring an explicit path.
func (s *Service) resolveAgentConfigPath(agentName, cfgFile string) (string, error) {
	if cfgFile != "" {
		expanded, err := fsx.ExpandUser(cfgFile)
		if err != nil {
			return "", err
		}
		if !fsx.FileExists(expanded) {
			return "", fmt.Errorf("specified config file not found: %s", cfgFile)
		}
		return expanded, nil
	}
	detected, err := s.resolveAgentConfigFile(agentName)
	if err != nil {
		return "", err
	}
	if detected == "" {
		return "", fmt.Errorf("no %s configuration file found", agentName)
	}
	return detected, nil
}

// setNestedValue creates intermediate objects as needed and sets or removes a
// nested value in place.
func setNestedValue(root map[string]any, path []string, value any, remove bool) error {
	if len(path) == 0 {
		return fmt.Errorf("empty config field path")
	}
	cur := root
	for _, key := range path[:len(path)-1] {
		next, ok := cur[key].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[key] = next
		}
		cur = next
	}
	last := path[len(path)-1]
	if remove {
		delete(cur, last)
		return nil
	}
	cur[last] = value
	return nil
}
