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

// DefaultStatePath returns the path to user state: ~/.local/state/asoul/deployments.json (or OS equivalent)
func DefaultStatePath() (string, error) {
	// Follow XDG_STATE_HOME or fallback to ~/.local/state/asoul
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to determine user home directory: %w", err)
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "asoul", "deployments.json"), nil
}

// Manager manages deployment state persistence.
type Manager struct {
	statePath string
}

// NewManager creates a deployment state manager.
func NewManager(customPath string) (*Manager, error) {
	if customPath != "" {
		expanded, err := fsx.ExpandUser(customPath)
		if err != nil {
			return nil, err
		}
		return &Manager{statePath: expanded}, nil
	}
	defPath, err := DefaultStatePath()
	if err != nil {
		return nil, err
	}
	return &Manager{statePath: defPath}, nil
}

// Path returns the path of the deployment state file.
func (m *Manager) Path() string {
	return m.statePath
}

// Load loads deployment state from disk.
func (m *Manager) Load() (*model.DeploymentState, error) {
	st := &model.DeploymentState{
		Targets: make(map[string]map[string]model.DeploymentRecord),
	}

	if !fsx.FileExists(m.statePath) {
		return st, nil
	}

	data, err := os.ReadFile(m.statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deployment state file (%s): %w", m.statePath, err)
	}

	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("failed to parse deployment state file: %w", err)
	}

	if st.Targets == nil {
		st.Targets = make(map[string]map[string]model.DeploymentRecord)
	}

	return st, nil
}

// Save writes deployment state atomically to disk.
func (m *Manager) Save(st *model.DeploymentState) error {
	if st.Targets == nil {
		st.Targets = make(map[string]map[string]model.DeploymentRecord)
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode deployment state: %w", err)
	}
	data = append(data, '\n')

	return fsx.AtomicWriteFile(m.statePath, data, 0644)
}

// RecordDeployment records a successful deployment.
func (m *Manager) RecordDeployment(targetName, skillID, contentHash string, meta ...string) error {
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0755); err != nil {
		return fmt.Errorf("failed to create deployment state directory: %w", err)
	}
	fileLock := flock.New(m.statePath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to lock deployment state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := m.Load()
	if err != nil {
		return err
	}

	if _, ok := st.Targets[targetName]; !ok {
		st.Targets[targetName] = make(map[string]model.DeploymentRecord)
	}

	rec := model.DeploymentRecord{
		ContentHash: contentHash,
		DeployedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if len(meta) > 0 && meta[0] != "" {
		rec.TargetPath = meta[0]
	}
	if len(meta) > 1 && meta[1] != "" {
		rec.Workspace = meta[1]
	}

	st.Targets[targetName][skillID] = rec

	return m.Save(st)
}

// RemoveDeployment removes a deployment record.
func (m *Manager) RemoveDeployment(targetName, skillID string) error {
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0755); err != nil {
		return fmt.Errorf("failed to create deployment state directory: %w", err)
	}
	fileLock := flock.New(m.statePath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to lock deployment state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := m.Load()
	if err != nil {
		return err
	}

	if skills, ok := st.Targets[targetName]; ok {
		delete(skills, skillID)
		if len(skills) == 0 {
			delete(st.Targets, targetName)
		}
	}

	return m.Save(st)
}

// PruneStaleDeployments removes deployment records for which isStillPresent returns false.
func (m *Manager) PruneStaleDeployments(targetName string, isStillPresent func(skillID string, rec model.DeploymentRecord) bool) error {
	if !fsx.FileExists(m.statePath) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0755); err != nil {
		return fmt.Errorf("failed to create deployment state directory: %w", err)
	}
	fileLock := flock.New(m.statePath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to lock deployment state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := m.Load()
	if err != nil {
		return err
	}
	if st.Targets == nil {
		return nil
	}

	skills, ok := st.Targets[targetName]
	if !ok {
		return nil
	}

	changed := false
	for skID, rec := range skills {
		if isStillPresent != nil && !isStillPresent(skID, rec) {
			delete(skills, skID)
			changed = true
		}
	}
	if len(skills) == 0 {
		delete(st.Targets, targetName)
		changed = true
	}

	if changed {
		return m.Save(st)
	}
	return nil
}

// GetRecord returns deployment record for a target and skill.
func (m *Manager) GetRecord(targetName, skillID string) (*model.DeploymentRecord, bool, error) {
	st, err := m.Load()
	if err != nil {
		return nil, false, err
	}

	skills, ok := st.Targets[targetName]
	if !ok {
		return nil, false, nil
	}

	rec, ok := skills[skillID]
	if !ok {
		return nil, false, nil
	}

	return &rec, true, nil
}
