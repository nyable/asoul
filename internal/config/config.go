package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"asoul/internal/fsx"
	"asoul/internal/jsonc"
	"asoul/internal/model"
	"asoul/internal/state"

	"github.com/gofrs/flock"
)

// DefaultConfigPath returns the default path for asoul user config: ~/.config/asoul/config.jsonc (or OS equivalent)
func DefaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user config directory: %w", err)
	}
	return filepath.Join(base, "asoul", "config.jsonc"), nil
}

// DefaultWorkspacePath returns the default path for asoul workspace: ~/.config/asoul (or OS equivalent)
func DefaultWorkspacePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user config directory: %w", err)
	}
	return filepath.Join(base, "asoul"), nil
}

// DefaultWorkspacePathCompact returns the compact default workspace path (~/.config/asoul).
func DefaultWorkspacePathCompact() string {
	p, err := DefaultWorkspacePath()
	if err != nil {
		return "~/.config/asoul"
	}
	return fsx.CompactUser(p)
}

// Manager manages user config reading and writing.
type Manager struct {
	configPath      string
	projectStateMgr *state.ProjectStateManager
}

// NewManager creates a config manager with an optional custom path.
func NewManager(customPath string) (*Manager, error) {
	var cfgPath string
	var psmPath string

	if customPath != "" {
		expanded, err := fsx.ExpandUser(customPath)
		if err != nil {
			return nil, err
		}
		cfgPath = expanded
		psmPath = filepath.Join(filepath.Dir(expanded), "projects.json")
	} else {
		defPath, err := DefaultConfigPath()
		if err != nil {
			return nil, err
		}
		cfgPath = defPath
		defPSM, err := state.DefaultProjectsStatePath()
		if err == nil {
			psmPath = defPSM
		}
	}

	var psm *state.ProjectStateManager
	if psmPath != "" {
		var err error
		psm, err = state.NewProjectStateManager(psmPath)
		if err != nil {
			return nil, err
		}
	}

	return &Manager{
		configPath:      cfgPath,
		projectStateMgr: psm,
	}, nil
}

// SetProjectStateManager sets a custom ProjectStateManager.
func (m *Manager) SetProjectStateManager(psm *state.ProjectStateManager) {
	m.projectStateMgr = psm
}

// ProjectStateManager returns the underlying ProjectStateManager.
func (m *Manager) ProjectStateManager() *state.ProjectStateManager {
	return m.projectStateMgr
}

// Path returns the path of the config file.
func (m *Manager) Path() string {
	return m.configPath
}

// Load loads the configuration from disk. If the file does not exist, an empty default config is returned.
func (m *Manager) Load() (*model.Config, error) {
	cfg := &model.Config{
		Version:  1,
		Targets:  make(map[string]model.TargetConfig),
		Profiles: make(map[string]model.ProfileConfig),
	}

	defWS := DefaultWorkspacePathCompact()

	if !fsx.FileExists(m.configPath) {
		if cfg.DefaultRoot != "" {
			cfg.Workspaces = []string{cfg.DefaultRoot}
		} else {
			cfg.Workspaces = []string{defWS}
		}
		return cfg, nil
	}

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file (%s): %w", m.configPath, err)
	}

	if err := jsonc.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file (%s): %w", m.configPath, err)
	}

	// Check for optional config.local.jsonc overlay
	localPath := filepath.Join(filepath.Dir(m.configPath), "config.local.jsonc")
	if fsx.FileExists(localPath) {
		if localData, err := os.ReadFile(localPath); err == nil {
			_ = jsonc.Unmarshal(localData, cfg)
		}
	}

	if cfg.Targets == nil {
		cfg.Targets = make(map[string]model.TargetConfig)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = make(map[string]model.ProfileConfig)
	}
	rawWorkspaces := cfg.Workspaces
	if cfg.DefaultRoot != "" {
		cfg.DefaultRoot = fsx.NormalizeWorkspacePath(cfg.DefaultRoot)
	}

	var deduped []string
	for _, w := range rawWorkspaces {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		norm := fsx.NormalizeWorkspacePath(w)
		exists := false
		for _, ex := range deduped {
			if fsx.SamePath(ex, norm) {
				exists = true
				break
			}
		}
		if !exists {
			deduped = append(deduped, norm)
		}
	}
	if len(deduped) == 0 {
		if cfg.DefaultRoot != "" {
			deduped = []string{cfg.DefaultRoot}
		} else {
			deduped = []string{defWS}
		}
	}

	cfg.Workspaces = deduped
	return cfg, nil
}

// errSkipSave is returned by a mutate callback to indicate the config was not
// changed and therefore must not be persisted.
var errSkipSave = errors.New("config: no change")

// mutate performs a locked read-modify-write of the config file. The lock is
// shared across processes so concurrent asoul invocations cannot overwrite each
// other's updates.
func (m *Manager) mutate(fn func(cfg *model.Config) error) error {
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	fileLock := flock.New(m.configPath + ".lock")
	if err := fileLock.Lock(); err != nil {
		return fmt.Errorf("failed to lock config file: %w", err)
	}
	defer fileLock.Unlock()

	cfg, err := m.Load()
	if err != nil {
		return err
	}
	if err := fn(cfg); err != nil {
		if errors.Is(err, errSkipSave) {
			return nil
		}
		return err
	}
	return m.Save(cfg)
}

// Update performs a locked read-modify-write of the configuration. It is the
// exported entry point for callers that need custom mutations that are not
// covered by the built-in setters.
func (m *Manager) Update(fn func(cfg *model.Config) error) error {
	return m.mutate(fn)
}

// Save writes the configuration atomically to disk in formatted JSON. When the
// file already exists its permission bits are preserved so a private config
// (e.g. 0600) is never widened.
func (m *Manager) Save(cfg *model.Config) error {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config to json: %w", err)
	}
	data = append(data, '\n')

	perm := os.FileMode(0644)
	if info, statErr := os.Stat(m.configPath); statErr == nil {
		perm = info.Mode().Perm()
	}

	return fsx.AtomicWriteFile(m.configPath, data, perm)
}

// AddTarget adds or updates a target in config.
func (m *Manager) AddTarget(name string, target model.TargetConfig) error {
	return m.mutate(func(cfg *model.Config) error {
		cfg.Targets[name] = target
		return nil
	})
}

// AddTargetSimple adds or updates an agent target with standard default skills path.
func (m *Manager) AddTargetSimple(name, path string) error {
	configDir := strings.TrimSuffix(strings.TrimSuffix(path, "/"), "\\")
	if strings.HasSuffix(configDir, "/skills") {
		configDir = strings.TrimSuffix(configDir, "/skills")
	} else if strings.HasSuffix(configDir, "\\skills") {
		configDir = strings.TrimSuffix(configDir, "\\skills")
	}

	return m.AddTarget(name, model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   name,
		ConfigDir: configDir,
		Paths: map[string]string{
			"skills": "${config_dir}/skills",
		},
	})
}

// RemoveTarget removes a target by name.
func (m *Manager) RemoveTarget(name string) error {
	return m.mutate(func(cfg *model.Config) error {
		if _, exists := cfg.Targets[name]; !exists {
			return fmt.Errorf("target %q not found", name)
		}
		delete(cfg.Targets, name)
		return nil
	})
}

// GetTarget returns a target config by name, and its resolved skills directory path.
func (m *Manager) GetTarget(name string) (*model.TargetConfig, string, error) {
	cfg, err := m.Load()
	if err != nil {
		return nil, "", err
	}
	tgt, exists := cfg.Targets[name]
	if !exists {
		return nil, "", fmt.Errorf("target %q not configured", name)
	}
	return &tgt, tgt.SkillsDir(), nil
}

// SetTargetEnabled sets the enabled status of a target in config.jsonc.
func (m *Manager) SetTargetEnabled(name string, enabled bool) error {
	return m.mutate(func(cfg *model.Config) error {
		tgt, exists := cfg.Targets[name]
		if !exists {
			return fmt.Errorf("target %q not configured", name)
		}
		tgt.Enabled = &enabled
		cfg.Targets[name] = tgt
		return nil
	})
}

// SetProjectEnabled sets the enabled status of a project path in config.jsonc or project state.
func (m *Manager) SetProjectEnabled(path string, enabled bool) error {
	norm := fsx.NormalizeWorkspacePath(path)
	_ = m.mutate(func(cfg *model.Config) error {
		for name, tgt := range cfg.Targets {
			if tgt.Type == model.TargetTypeProject {
				pDir := tgt.ResolveConfigDir()
				if pDir == "" {
					pDir = tgt.SkillsDir()
				}
				if fsx.SamePath(pDir, norm) {
					tgt.Enabled = &enabled
					cfg.Targets[name] = tgt
				}
			}
		}
		return nil
	})

	if m.projectStateMgr != nil {
		return m.projectStateMgr.SetProjectEnabled(path, enabled)
	}
	return nil
}

// IsProjectEnabled checks whether a project path is enabled in targets or project state.
func (m *Manager) IsProjectEnabled(path string) (bool, error) {
	cfg, err := m.Load()
	if err != nil {
		return true, err
	}
	norm := fsx.NormalizeWorkspacePath(path)
	for _, tgt := range cfg.Targets {
		if tgt.Type == model.TargetTypeProject {
			pDir := tgt.ResolveConfigDir()
			if pDir == "" {
				pDir = tgt.SkillsDir()
			}
			if fsx.SamePath(pDir, norm) {
				return tgt.IsEnabled(), nil
			}
		}
	}

	if m.projectStateMgr != nil {
		return m.projectStateMgr.IsProjectEnabled(path)
	}
	return true, nil
}

// AddRecentProject caches a project root path in machine-local projects state.
func (m *Manager) AddRecentProject(path string) error {
	if m.projectStateMgr == nil {
		return nil
	}
	return m.projectStateMgr.AddProject(path)
}

// GetRecentProjects returns all cached project paths from machine-local projects state.
func (m *Manager) GetRecentProjects() ([]string, error) {
	if m.projectStateMgr == nil {
		return nil, nil
	}
	return m.projectStateMgr.GetProjects()
}

// RemoveRecentProject removes a project path from machine-local projects state.
func (m *Manager) RemoveRecentProject(path string) error {
	if m.projectStateMgr == nil {
		return fmt.Errorf("project %q not found in state", path)
	}
	return m.projectStateMgr.RemoveProject(path)
}

// Group operations
func (m *Manager) GroupList() (map[string]model.ProfileConfig, error) {
	cfg, err := m.Load()
	if err != nil {
		return nil, err
	}
	return cfg.Profiles, nil
}

func (m *Manager) GroupGet(name string) (*model.ProfileConfig, error) {
	cfg, err := m.Load()
	if err != nil {
		return nil, err
	}
	group, exists := cfg.Profiles[name]
	if !exists {
		return nil, fmt.Errorf("group %q not found", name)
	}
	return &group, nil
}

func (m *Manager) GroupCreate(name string) error {
	return m.mutate(func(cfg *model.Config) error {
		if cfg.Profiles == nil {
			cfg.Profiles = make(map[string]model.ProfileConfig)
		}
		if _, exists := cfg.Profiles[name]; exists {
			return fmt.Errorf("group %q already exists", name)
		}
		cfg.Profiles[name] = model.ProfileConfig{
			Skills: []string{},
		}
		return nil
	})
}

func (m *Manager) GroupDelete(name string) error {
	return m.mutate(func(cfg *model.Config) error {
		if _, exists := cfg.Profiles[name]; !exists {
			return fmt.Errorf("group %q not found", name)
		}
		delete(cfg.Profiles, name)
		return nil
	})
}

func (m *Manager) GroupAddSkill(name, skillID string) error {
	return m.mutate(func(cfg *model.Config) error {
		group, exists := cfg.Profiles[name]
		if !exists {
			return fmt.Errorf("group %q not found", name)
		}
		for _, id := range group.Skills {
			if id == skillID {
				return errSkipSave
			}
		}
		group.Skills = append(group.Skills, skillID)
		cfg.Profiles[name] = group
		return nil
	})
}

func (m *Manager) GroupRemoveSkill(name, skillID string) error {
	return m.mutate(func(cfg *model.Config) error {
		group, exists := cfg.Profiles[name]
		if !exists {
			return fmt.Errorf("group %q not found", name)
		}
		var updated []string
		found := false
		for _, id := range group.Skills {
			if id == skillID {
				found = true
				continue
			}
			updated = append(updated, id)
		}
		if !found {
			return fmt.Errorf("skill %q not found in group %q", skillID, name)
		}
		group.Skills = updated
		cfg.Profiles[name] = group
		return nil
	})
}

func (m *Manager) GroupSetSkills(name string, skillIDs []string) error {
	return m.mutate(func(cfg *model.Config) error {
		group, exists := cfg.Profiles[name]
		if !exists {
			group = model.ProfileConfig{}
		}
		group.Skills = skillIDs
		cfg.Profiles[name] = group
		return nil
	})
}

// GetWorkspaces returns the list of registered workspace paths.
func (m *Manager) GetWorkspaces() []string {
	cfg, err := m.Load()
	if err != nil || len(cfg.Workspaces) == 0 {
		return []string{DefaultWorkspacePathCompact()}
	}
	return cfg.Workspaces
}

// AddWorkspace adds a workspace path to config.jsonc.
func (m *Manager) AddWorkspace(path string) error {
	return m.mutate(func(cfg *model.Config) error {
		norm := fsx.NormalizeWorkspacePath(path)
		for _, w := range cfg.Workspaces {
			if fsx.SamePath(w, norm) {
				return errSkipSave
			}
		}
		cfg.Workspaces = append(cfg.Workspaces, norm)
		if cfg.DefaultRoot == "" {
			cfg.DefaultRoot = norm
		}
		return nil
	})
}

// RemoveWorkspace removes a workspace path from config.jsonc.
func (m *Manager) RemoveWorkspace(path string) error {
	path = strings.TrimSpace(path)
	return m.mutate(func(cfg *model.Config) error {
		var updated []string
		for _, w := range cfg.Workspaces {
			if !fsx.SamePath(w, path) {
				updated = append(updated, w)
			}
		}
		cfg.Workspaces = updated

		if fsx.SamePath(cfg.DefaultRoot, path) {
			if len(updated) > 0 {
				cfg.DefaultRoot = updated[0]
			} else {
				cfg.DefaultRoot = ""
			}
		}

		return nil
	})
}

// SetOfficialProviders sets custom official providers in config.jsonc.
func (m *Manager) SetOfficialProviders(providers map[string][]string) error {
	return m.mutate(func(cfg *model.Config) error {
		if cfg.Model == nil {
			cfg.Model = &model.ModelConfig{}
		}
		cfg.Model.OfficialProviders = providers
		cfg.Model.AuthoritativeProviders = nil // clean legacy field
		return nil
	})
}

// SetAuthoritativeProviders is a backwards-compatible alias for SetOfficialProviders.
func (m *Manager) SetAuthoritativeProviders(providers map[string][]string) error {
	return m.SetOfficialProviders(providers)
}

// InitOfficialProviders persists the default official providers into config.jsonc.
func (m *Manager) InitOfficialProviders() error {
	return m.SetOfficialProviders(model.DefaultOfficialProviders())
}

// InitAuthoritativeProviders is a backwards-compatible alias for InitOfficialProviders.
func (m *Manager) InitAuthoritativeProviders() error {
	return m.InitOfficialProviders()
}

// ListUpstreams returns all upstream configurations stored in config.jsonc.
func (m *Manager) ListUpstreams() ([]model.UpstreamConfig, error) {
	cfg, err := m.Load()
	if err != nil {
		return nil, err
	}
	return cfg.Upstreams, nil
}

// AddUpstream adds or updates an upstream configuration in config.jsonc.
func (m *Manager) AddUpstream(upstream model.UpstreamConfig) error {
	trimmedURL := strings.TrimSpace(upstream.URL)
	if trimmedURL == "" {
		return fmt.Errorf("upstream URL cannot be empty")
	}
	nowStr := time.Now().Format("2006-01-02 15:04:05")

	return m.mutate(func(cfg *model.Config) error {
		upstream.URL = trimmedURL

		updated := false
		for i, u := range cfg.Upstreams {
			if strings.TrimSpace(u.URL) == trimmedURL {
				if upstream.CreatedAt == "" {
					upstream.CreatedAt = u.CreatedAt
				}
				upstream.UpdatedAt = nowStr
				cfg.Upstreams[i] = upstream
				updated = true
				break
			}
		}
		if !updated {
			if upstream.CreatedAt == "" {
				upstream.CreatedAt = nowStr
			}
			if upstream.UpdatedAt == "" {
				upstream.UpdatedAt = nowStr
			}
			cfg.Upstreams = append(cfg.Upstreams, upstream)
		}
		return nil
	})
}

// RemoveUpstream removes an upstream by its URL from config.jsonc.
func (m *Manager) RemoveUpstream(url string) error {
	trimmedURL := strings.TrimSpace(url)
	return m.mutate(func(cfg *model.Config) error {
		var filtered []model.UpstreamConfig
		found := false
		for _, u := range cfg.Upstreams {
			if strings.TrimSpace(u.URL) == trimmedURL {
				found = true
				continue
			}
			filtered = append(filtered, u)
		}
		if !found {
			return fmt.Errorf("upstream %q not found in configuration", url)
		}
		cfg.Upstreams = filtered
		return nil
	})
}

// UpdateUpstream updates an existing upstream configuration.
func (m *Manager) UpdateUpstream(upstream model.UpstreamConfig) error {
	return m.AddUpstream(upstream)
}

// TouchUpstream updates the UpdatedAt timestamp for an upstream configuration.
func (m *Manager) TouchUpstream(url string) error {
	trimmedURL := strings.TrimSpace(url)
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	return m.mutate(func(cfg *model.Config) error {
		for i, u := range cfg.Upstreams {
			if strings.TrimSpace(u.URL) == trimmedURL {
				cfg.Upstreams[i].UpdatedAt = nowStr
				return nil
			}
		}
		return errSkipSave
	})
}
