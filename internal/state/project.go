package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"asoul/internal/fsx"
	"asoul/internal/model"
	"github.com/gofrs/flock"
)

const (
	ProjectAgentsDir = ".agents"
	ProjectStateFile = ".asoul-state.json"
)

// ProjectStatePath returns the path to <projectRoot>/.agents/.asoul-state.json
func ProjectStatePath(projectRoot string) string {
	return filepath.Join(projectRoot, ProjectAgentsDir, ProjectStateFile)
}

// LoadProjectState loads the project deployment state from <projectRoot>/.agents/.asoul-state.json
func LoadProjectState(projectRoot string) (*model.ProjectState, error) {
	st := &model.ProjectState{
		Version: 1,
		Skills:  make(map[string]model.ProjectSkillRecord),
	}

	statePath := ProjectStatePath(projectRoot)
	if !fsx.FileExists(statePath) {
		return st, nil
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read project state file (%s): %w", statePath, err)
	}

	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("failed to parse project state file (%s): %w", statePath, err)
	}

	if st.Skills == nil {
		st.Skills = make(map[string]model.ProjectSkillRecord)
	}

	return st, nil
}

// SaveProjectState writes project deployment state atomically to <projectRoot>/.agents/.asoul-state.json
func SaveProjectState(projectRoot string, st *model.ProjectState) error {
	if st.Skills == nil {
		st.Skills = make(map[string]model.ProjectSkillRecord)
	}
	if st.Version == 0 {
		st.Version = 1
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode project state: %w", err)
	}
	data = append(data, '\n')

	statePath := ProjectStatePath(projectRoot)
	return fsx.AtomicWriteFile(statePath, data, 0644)
}

// RecordProjectDeployment records or updates deployment info for a skill and format in a project.
func RecordProjectDeployment(projectRoot, skillID, format, destPath, contentHash string) error {
	if err := os.MkdirAll(filepath.Dir(ProjectStatePath(projectRoot)), 0755); err != nil {
		return fmt.Errorf("failed to create project state directory: %w", err)
	}
	fileLock := flock.New(ProjectStatePath(projectRoot) + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to lock project deployment state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := LoadProjectState(projectRoot)
	if err != nil {
		return err
	}

	skillRec, ok := st.Skills[skillID]
	if !ok {
		skillRec = model.ProjectSkillRecord{
			Formats: make(map[string]model.ProjectDeploymentRecord),
		}
	}
	if skillRec.Formats == nil {
		skillRec.Formats = make(map[string]model.ProjectDeploymentRecord)
	}

	skillRec.Formats[format] = model.ProjectDeploymentRecord{
		Format:      format,
		DestPath:    destPath,
		ContentHash: contentHash,
		DeployedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	st.Skills[skillID] = skillRec

	return SaveProjectState(projectRoot, st)
}

// GetProjectRecord returns the deployment record for a specific skill and format in a project.
func GetProjectRecord(projectRoot, skillID, format string) (*model.ProjectDeploymentRecord, bool, error) {
	st, err := LoadProjectState(projectRoot)
	if err != nil {
		return nil, false, err
	}

	skillRec, ok := st.Skills[skillID]
	if !ok || skillRec.Formats == nil {
		return nil, false, nil
	}

	rec, ok := skillRec.Formats[format]
	if !ok {
		return nil, false, nil
	}

	return &rec, true, nil
}

// GetProjectSkillRecord returns all format deployments for a skill in a project.
func GetProjectSkillRecord(projectRoot, skillID string) (*model.ProjectSkillRecord, bool, error) {
	st, err := LoadProjectState(projectRoot)
	if err != nil {
		return nil, false, err
	}

	skillRec, ok := st.Skills[skillID]
	if !ok {
		return nil, false, nil
	}

	return &skillRec, true, nil
}

// RemoveProjectDeployment removes a specific format deployment for a skill in a project.
func RemoveProjectDeployment(projectRoot, skillID, format string) error {
	if err := os.MkdirAll(filepath.Dir(ProjectStatePath(projectRoot)), 0755); err != nil {
		return fmt.Errorf("failed to create project state directory: %w", err)
	}
	fileLock := flock.New(ProjectStatePath(projectRoot) + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to lock project deployment state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := LoadProjectState(projectRoot)
	if err != nil {
		return err
	}

	skillRec, ok := st.Skills[skillID]
	if ok && skillRec.Formats != nil {
		delete(skillRec.Formats, format)
		if len(skillRec.Formats) == 0 {
			delete(st.Skills, skillID)
		} else {
			st.Skills[skillID] = skillRec
		}
	}

	return SaveProjectState(projectRoot, st)
}

// RemoveAllProjectDeployments removes all format deployments for a skill in a project, returning the removed formats.
func RemoveAllProjectDeployments(projectRoot, skillID string) ([]string, error) {
	statePath := ProjectStatePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create project state directory: %w", err)
	}
	fileLock := flock.New(statePath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return nil, fmt.Errorf("failed to lock project deployment state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := LoadProjectState(projectRoot)
	if err != nil {
		return nil, err
	}

	skillRec, ok := st.Skills[skillID]
	if !ok || skillRec.Formats == nil {
		return nil, nil
	}

	var formats []string
	for fmtID := range skillRec.Formats {
		formats = append(formats, fmtID)
	}

	delete(st.Skills, skillID)
	if err := SaveProjectState(projectRoot, st); err != nil {
		return nil, err
	}

	return formats, nil
}

// PruneStaleProjectDeployments removes project deployment records where dest path no longer exists.
func PruneStaleProjectDeployments(projectRoot string, isStillPresent func(skillID, format string, rec model.ProjectDeploymentRecord) bool) (*model.ProjectState, error) {
	statePath := ProjectStatePath(projectRoot)
	if !fsx.FileExists(statePath) {
		return &model.ProjectState{Version: 1, Skills: make(map[string]model.ProjectSkillRecord)}, nil
	}

	fileLock := flock.New(statePath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return nil, fmt.Errorf("failed to lock project deployment state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := LoadProjectState(projectRoot)
	if err != nil {
		return nil, err
	}
	if st.Skills == nil {
		return st, nil
	}

	changed := false
	for skID, sRec := range st.Skills {
		if sRec.Formats != nil {
			for fmtID, fRec := range sRec.Formats {
				if isStillPresent != nil && !isStillPresent(skID, fmtID, fRec) {
					delete(sRec.Formats, fmtID)
					changed = true
				}
			}
		}
		if len(sRec.Formats) == 0 {
			delete(st.Skills, skID)
			changed = true
		}
	}

	if changed {
		if err := SaveProjectState(projectRoot, st); err != nil {
			return st, err
		}
	}
	return st, nil
}
