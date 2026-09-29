package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"asoul/internal/fsx"
	"github.com/gofrs/flock"
)

// DefaultProjectsStatePath returns the path to user machine projects state:
// ~/.local/state/asoul/projects.json (or OS equivalent following XDG_STATE_HOME)
func DefaultProjectsStatePath() (string, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to determine user home directory: %w", err)
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "asoul", "projects.json"), nil
}

// ProjectsState stores machine-local cached and enabled project paths.
type ProjectsState struct {
	Projects         []string `json:"projects,omitempty"`
	DisabledProjects []string `json:"disabled_projects,omitempty"`
}

// ProjectStateManager manages machine-local project state.
type ProjectStateManager struct {
	path string
}

// NewProjectStateManager creates a new project state manager.
func NewProjectStateManager(customPath string) (*ProjectStateManager, error) {
	if customPath != "" {
		expanded, err := fsx.ExpandUser(customPath)
		if err != nil {
			return nil, err
		}
		return &ProjectStateManager{path: expanded}, nil
	}
	defPath, err := DefaultProjectsStatePath()
	if err != nil {
		return nil, err
	}
	return &ProjectStateManager{path: defPath}, nil
}

// Path returns the path of the projects state file.
func (m *ProjectStateManager) Path() string {
	return m.path
}

// Load loads project state from disk.
func (m *ProjectStateManager) Load() (*ProjectsState, error) {
	st := &ProjectsState{}
	if !fsx.FileExists(m.path) {
		return st, nil
	}

	data, err := os.ReadFile(m.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read projects state file (%s): %w", m.path, err)
	}

	if err := json.Unmarshal(data, st); err != nil {
		return nil, fmt.Errorf("failed to parse projects state file: %w", err)
	}

	return st, nil
}

// Save writes project state atomically to disk.
func (m *ProjectStateManager) Save(st *ProjectsState) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode projects state: %w", err)
	}
	data = append(data, '\n')

	return fsx.AtomicWriteFile(m.path, data, 0644)
}

// mutate performs a locked read-modify-write of the projects state file.
func (m *ProjectStateManager) mutate(fn func(st *ProjectsState) error) error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return fmt.Errorf("failed to create projects state directory: %w", err)
	}

	fileLock := flock.New(m.path + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to lock projects state: %w", err)
	}
	defer fileLock.Unlock()

	st, err := m.Load()
	if err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	return m.Save(st)
}

// GetProjects returns all cached project paths.
func (m *ProjectStateManager) GetProjects() ([]string, error) {
	st, err := m.Load()
	if err != nil {
		return nil, err
	}
	return st.Projects, nil
}

// AddProject adds a project path to the machine-local projects state.
func (m *ProjectStateManager) AddProject(path string) error {
	expanded, err := fsx.ExpandUser(path)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(expanded)
	if err != nil {
		absPath = filepath.Clean(path)
	}

	return m.mutate(func(st *ProjectsState) error {
		var updated []string
		updated = append(updated, absPath)
		for _, p := range st.Projects {
			if !fsx.SamePath(p, absPath) {
				updated = append(updated, p)
			}
		}
		if len(updated) > 50 {
			updated = updated[:50]
		}
		st.Projects = updated
		return nil
	})
}

// RemoveProject removes a project path from machine-local projects state.
func (m *ProjectStateManager) RemoveProject(path string) error {
	return m.mutate(func(st *ProjectsState) error {
		norm := fsx.NormalizeWorkspacePath(path)
		var updated []string
		found := false
		for _, p := range st.Projects {
			if fsx.SamePath(p, norm) {
				found = true
				continue
			}
			updated = append(updated, p)
		}
		if !found {
			return fmt.Errorf("project %q not found in state", path)
		}
		st.Projects = updated
		return nil
	})
}

// IsProjectEnabled checks whether a project path is enabled.
func (m *ProjectStateManager) IsProjectEnabled(path string) (bool, error) {
	st, err := m.Load()
	if err != nil {
		return true, err
	}
	norm := fsx.NormalizeWorkspacePath(path)
	for _, p := range st.DisabledProjects {
		if fsx.SamePath(p, norm) {
			return false, nil
		}
	}
	return true, nil
}

// SetProjectEnabled sets the enabled state for a project path.
func (m *ProjectStateManager) SetProjectEnabled(path string, enabled bool) error {
	return m.mutate(func(st *ProjectsState) error {
		norm := fsx.NormalizeWorkspacePath(path)
		var updated []string
		for _, p := range st.DisabledProjects {
			if !fsx.SamePath(p, norm) {
				updated = append(updated, p)
			}
		}
		if !enabled {
			updated = append(updated, norm)
		}
		st.DisabledProjects = updated
		return nil
	})
}

var _ = errors.New
