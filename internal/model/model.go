package model

import (
	"path/filepath"
	"sort"
	"strings"

	"asoul/internal/fsx"
)

// Catalog represents the workspace catalog.json format.
type Catalog struct {
	Version int                    `json:"version"`
	Skills  map[string]SkillRecord `json:"skills"`
}

// SkillRecord records source and resolution details of a managed skill.
type SkillRecord struct {
	Source   SourceSpec `json:"source"`
	Resolved Resolved   `json:"resolved"`
}

// SourceType represents the type of skill source.
type SourceType string

const (
	SourceTypeGit     SourceType = "git"
	SourceTypeLocal   SourceType = "local"
	SourceTypeManaged SourceType = "managed"
)

// SourceSpec defines where a skill comes from.
type SourceSpec struct {
	Type SourceType `json:"type"`
	URL  string     `json:"url,omitempty"`
	Ref  string     `json:"ref,omitempty"`
	Path string     `json:"path,omitempty"`
}

// Resolved stores the verified state of the skill.
type Resolved struct {
	Commit      string `json:"commit,omitempty"`
	ContentHash string `json:"contentHash"`
}

// ManagedStatus represents the state of a skill in the managed store.
type ManagedStatus string

const (
	StatusClean     ManagedStatus = "clean"
	StatusModified  ManagedStatus = "modified"
	StatusMissing   ManagedStatus = "missing"
	StatusInvalid   ManagedStatus = "invalid"
	StatusUnmanaged ManagedStatus = "unmanaged"
)

// UpstreamStatus represents the update status relative to the upstream source.
type UpstreamStatus string

const (
	UpstreamUpToDate        UpstreamStatus = "up-to-date"
	UpstreamUpdateAvailable UpstreamStatus = "update-available"
	UpstreamSourceChanged   UpstreamStatus = "source-changed"
	UpstreamUnreachable     UpstreamStatus = "unreachable"
	UpstreamInvalid         UpstreamStatus = "invalid"
	UpstreamNone            UpstreamStatus = "-"
)

// SkillStatus combines local managed state and upstream status.
type SkillStatus struct {
	ID             string         `json:"id"`
	Source         SourceSpec     `json:"source"`
	ManagedStatus  ManagedStatus  `json:"managedStatus"`
	CurrentHash    string         `json:"currentHash,omitempty"`
	RecordedHash   string         `json:"recordedHash,omitempty"`
	RecordedCommit string         `json:"recordedCommit,omitempty"`
	Error          string         `json:"error,omitempty"`
	Upstream       UpstreamStatus `json:"upstream,omitempty"`
	NewCommit      string         `json:"newCommit,omitempty"`
	NewHash        string         `json:"newHash,omitempty"`
}

// UpstreamInfo aggregates status and skills originating from an upstream source repository or directory.
type UpstreamInfo struct {
	URL             string         `json:"url"`
	Name            string         `json:"name,omitempty"`
	Type            SourceType     `json:"type"`
	Ref             string         `json:"ref,omitempty"`
	Skills          []string       `json:"skills"`
	AvailableSkills []string       `json:"availableSkills,omitempty"`
	Status          UpstreamStatus `json:"status"`
	Commit          string         `json:"commit,omitempty"`
	NewCommit       string         `json:"newCommit,omitempty"`
	NewHash         string         `json:"newHash,omitempty"`
	Error           string         `json:"error,omitempty"`
	CreatedAt       string         `json:"createdAt,omitempty"`
	UpdatedAt       string         `json:"updatedAt,omitempty"`
	CachePath       string         `json:"cachePath,omitempty"`
	CacheExists     bool           `json:"cacheExists"`
	Scanned         bool           `json:"scanned"`
	ScanError       string         `json:"scanError,omitempty"`
	ScanRoots       []string       `json:"scanRoots,omitempty"`
	ScanExclude     []string       `json:"scanExclude,omitempty"`
}

// SkillMetadata holds parsed information from SKILL.md.
type SkillMetadata struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Path        string `json:"path"`
}

// TargetType represents the category of deployment target.
type TargetType string

const (
	TargetTypeAgent   TargetType = "agent"   // Global AI agent client/channel (OpenCode, Claude, Codex, Cursor...)
	TargetTypeProject TargetType = "project" // Local codebase project repository
)

// TargetConfig defines an agent or project deployment target.
type TargetConfig struct {
	Type      TargetType        `json:"type,omitempty"`
	Channel   string            `json:"channel,omitempty"`
	ConfigDir string            `json:"config_dir,omitempty"`
	Paths     map[string]string `json:"paths,omitempty"`
	Enabled   *bool             `json:"enabled,omitempty"`
}

// IsEnabled returns whether the target is enabled (defaults to true if nil).
func (t *TargetConfig) IsEnabled() bool {
	if t.Enabled == nil {
		return true
	}
	return *t.Enabled
}

// FeatureKeys returns the keys configured in paths (e.g. ["rules", "skills"]).
// If paths is empty, returns ["skills"].
func (t *TargetConfig) FeatureKeys() []string {
	if len(t.Paths) == 0 {
		return []string{"skills"}
	}
	keys := make([]string, 0, len(t.Paths))
	for k := range t.Paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FeaturesString returns a comma-separated string of supported feature keys.
func (t *TargetConfig) FeaturesString() string {
	return strings.Join(t.FeatureKeys(), ", ")
}

// ResolveConfigDir expands ~ and returns the absolute path of the configuration root directory.
func (t *TargetConfig) ResolveConfigDir() string {
	if t.ConfigDir == "" {
		return ""
	}
	exp, err := fsx.ExpandUser(t.ConfigDir)
	if err != nil {
		exp = t.ConfigDir
	}
	abs, err := filepath.Abs(exp)
	if err == nil {
		return abs
	}
	return exp
}

// ResolvePath resolves an artifact path (e.g. "skills", "rules"), replacing ${config_dir} with the configuration root directory.
func (t *TargetConfig) ResolvePath(artifactKey string) string {
	if t.Paths == nil {
		return ""
	}
	raw := t.Paths[artifactKey]
	if raw == "" {
		return ""
	}
	if t.ConfigDir != "" {
		raw = strings.ReplaceAll(raw, "${config_dir}", t.ConfigDir)
	}
	exp, err := fsx.ExpandUser(raw)
	if err != nil {
		exp = raw
	}
	if !filepath.IsAbs(exp) && t.ConfigDir != "" {
		expBase, _ := fsx.ExpandUser(t.ConfigDir)
		exp = filepath.Join(expBase, exp)
	}
	abs, err := filepath.Abs(exp)
	if err == nil {
		return abs
	}
	return exp
}

// SkillsDir returns the resolved skills directory path.
func (t *TargetConfig) SkillsDir() string {
	if p := t.ResolvePath("skills"); p != "" {
		return p
	}
	if cfgDir := t.ResolveConfigDir(); cfgDir != "" {
		return filepath.Join(cfgDir, "skills")
	}
	return ""
}

// ProfileConfig defines a named grouping of skills and targets.
type ProfileConfig struct {
	Skills  []string `json:"skills"`
	Targets []string `json:"targets,omitempty"`
}

// UpstreamConfig represents an upstream source recorded in user-level configuration.
type UpstreamConfig struct {
	URL       string      `json:"url"`
	Type      SourceType  `json:"type,omitempty"`
	Ref       string      `json:"ref,omitempty"`
	Name      string      `json:"name,omitempty"`
	Scan      *ScanConfig `json:"scan,omitempty"`
	CreatedAt string      `json:"createdAt,omitempty"`
	UpdatedAt string      `json:"updatedAt,omitempty"`
}

// EffectiveScan returns the canonical scan scope for this upstream, applying the default
// when the source does not configure one or configures an invalid scope.
func (u UpstreamConfig) EffectiveScan() ScanConfig {
	if u.Scan == nil {
		return DefaultScanConfig()
	}
	return u.Scan.Normalized()
}

// EditorConfig stores an executable and arguments without shell parsing.
type EditorConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// Config represents user-level configuration stored in config.jsonc.
type Config struct {
	Version           int                      `json:"version"`
	Language          string                   `json:"language,omitempty"`
	Editor            *EditorConfig            `json:"editor,omitempty"`
	DefaultRoot       string                   `json:"default_root,omitempty"`
	Workspaces        []string                 `json:"workspaces,omitempty"`
	Upstreams         []UpstreamConfig         `json:"upstreams,omitempty"`
	Targets           map[string]TargetConfig  `json:"targets,omitempty"`
	Profiles          map[string]ProfileConfig `json:"profiles,omitempty"`
	Projects          []string                 `json:"-"`
	DisabledProjects  []string                 `json:"-"`
	BackupMaxVersions *int                     `json:"backup_max_versions,omitempty"`
	Model             *ModelConfig             `json:"model,omitempty"`
}

// IsProjectEnabled returns whether a project path is enabled.
func (c *Config) IsProjectEnabled(path string) bool {
	if c == nil {
		return true
	}
	norm := fsx.NormalizeWorkspacePath(path)
	for _, p := range c.DisabledProjects {
		if fsx.SamePath(p, norm) {
			return false
		}
	}
	if c.Targets != nil {
		for _, tgt := range c.Targets {
			if tgt.Type == TargetTypeProject && !tgt.IsEnabled() {
				pDir := tgt.ResolveConfigDir()
				if pDir == "" {
					pDir = tgt.SkillsDir()
				}
				if fsx.SamePath(pDir, norm) {
					return false
				}
			}
		}
	}
	return true
}

// SetProjectEnabled sets the enabled state for a project path.
func (c *Config) SetProjectEnabled(path string, enabled bool) {
	norm := fsx.NormalizeWorkspacePath(path)
	var updated []string
	for _, p := range c.DisabledProjects {
		if !fsx.SamePath(p, norm) {
			updated = append(updated, p)
		}
	}
	if !enabled {
		updated = append(updated, norm)
	}
	c.DisabledProjects = updated

	if c.Targets != nil {
		for name, tgt := range c.Targets {
			if tgt.Type == TargetTypeProject {
				pDir := tgt.ResolveConfigDir()
				if pDir == "" {
					pDir = tgt.SkillsDir()
				}
				if fsx.SamePath(pDir, norm) {
					tgt.Enabled = &enabled
					c.Targets[name] = tgt
				}
			}
		}
	}
}

// ModelConfig holds model enrichment specific preferences.
type ModelConfig struct {
	BackupMaxVersions      *int                `json:"backup_max_versions,omitempty"`
	EnrichMode             string              `json:"enrich_mode,omitempty"`
	OfficialProviders      map[string][]string `json:"official_providers,omitempty"`
	AuthoritativeProviders map[string][]string `json:"authoritative_providers,omitempty"` // Deprecated backwards-compatible alias
}

// Model enrich modes.
const (
	// ModelEnrichModeRemote pulls models.dev data and overwrites existing values.
	ModelEnrichModeRemote = "remote"
	// ModelEnrichModeIncremental only fills missing values, keeping local edits.
	ModelEnrichModeIncremental = "incremental"
)

// GetModelEnrichMode returns the last used enrich mode, defaulting to remote.
func (c *Config) GetModelEnrichMode() string {
	if c != nil && c.Model != nil && c.Model.EnrichMode == ModelEnrichModeIncremental {
		return ModelEnrichModeIncremental
	}
	return ModelEnrichModeRemote
}

// SetModelEnrichMode stores the enrich mode preference.
func (c *Config) SetModelEnrichMode(mode string) {
	if mode != ModelEnrichModeIncremental && mode != ModelEnrichModeRemote {
		mode = ModelEnrichModeRemote
	}
	if c.Model == nil {
		c.Model = &ModelConfig{}
	}
	c.Model.EnrichMode = mode
}

// GetBackupMaxVersions returns the configured maximum backup versions, or nil if unlimited.
func (c *Config) GetBackupMaxVersions() *int {
	if c == nil {
		return nil
	}
	if c.BackupMaxVersions != nil {
		return c.BackupMaxVersions
	}
	if c.Model != nil && c.Model.BackupMaxVersions != nil {
		return c.Model.BackupMaxVersions
	}
	return nil
}

// GetOfficialProviders returns configured official providers, or default ones if not configured.
func (c *Config) GetOfficialProviders() map[string][]string {
	if c != nil && c.Model != nil {
		if c.Model.OfficialProviders != nil {
			return c.Model.OfficialProviders
		}
		if c.Model.AuthoritativeProviders != nil {
			return c.Model.AuthoritativeProviders
		}
	}
	return DefaultOfficialProviders()
}

// GetAuthoritativeProviders is a backwards-compatible alias for GetOfficialProviders.
func (c *Config) GetAuthoritativeProviders() map[string][]string {
	return c.GetOfficialProviders()
}

// DefaultOfficialProviders returns the standard built-in mapping of official creators to model regex patterns.
func DefaultOfficialProviders() map[string][]string {
	return map[string][]string{
		"openai":    {"(?i)^gpt-", "(?i)^o[134]-", "(?i)^chatgpt-"},
		"anthropic": {"(?i)^claude-"},
		"deepseek":  {"(?i)^deepseek-"},
		"google":    {"(?i)^gemini-"},
		"zhipuai":   {"(?i)^glm-"},
		"xiaomi":    {"(?i)^mimo-"},
		"minimax":   {"(?i)^minimax"},
		"alibaba":   {"(?i)^qwen-"},
		"mistral":   {"(?i)^(mistral|codestral|pixtral)-"},
		"meta":      {"(?i)^llama-"},
		"xai":       {"(?i)^grok-"},
		"cohere":    {"(?i)^command-"},
	}
}

// DefaultAuthoritativeProviders is a backwards-compatible alias for DefaultOfficialProviders.
func DefaultAuthoritativeProviders() map[string][]string {
	return DefaultOfficialProviders()
}

// DeploymentRecord records metadata for a deployed skill in a target.
type DeploymentRecord struct {
	ContentHash string `json:"contentHash"`
	DeployedAt  string `json:"deployedAt"`
	TargetPath  string `json:"targetPath,omitempty"`
	Workspace   string `json:"workspace,omitempty"`
}

// DeploymentState stores target deployment records.
type DeploymentState struct {
	Targets map[string]map[string]DeploymentRecord `json:"targets"`
}

// ProjectDeploymentRecord records deployment info for a specific format in a project.
type ProjectDeploymentRecord struct {
	Format      string `json:"format"`
	DestPath    string `json:"destPath"`
	ContentHash string `json:"contentHash"`
	DeployedAt  string `json:"deployedAt"`
}

// ProjectSkillRecord stores format deployments for a skill within a project.
type ProjectSkillRecord struct {
	Formats map[string]ProjectDeploymentRecord `json:"formats"`
}

// ProjectState stores project-level deployment records (.agents/.asoul-state.json).
type ProjectState struct {
	Version int                           `json:"version"`
	Skills  map[string]ProjectSkillRecord `json:"skills"`
}

// Candidate represents an extracted/prepared skill ready for inspection or installation.
type Candidate struct {
	TempDir string
	SkillID string
	Commit  string
	Hash    string
}
