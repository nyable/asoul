package agent

import (
	"context"

	"asoul/internal/modelrules"
	"asoul/internal/modelsdev"
)

// EnrichOptions specifies parameters for enriching agent config.
type EnrichOptions struct {
	FilePath          string // Explicit config file path (optional)
	ProviderID        string // Target specific provider (optional, empty means all custom providers)
	Override          bool   // Overwrite existing non-empty metadata fields
	DryRun            bool   // Output diff preview without modifying files
	NoBackup          bool   // Skip creating backup file
	BackupMaxVersions *int   // Maximum backup versions to retain. 0 means no backup, nil means unlimited.
	RulesPath         string // Custom path to rule file (optional)
	RulesOnly         bool   // Apply custom rules only, skipping models.dev metadata
	Workspace         string // Workspace root path
	ConfigDir         string // Target agent configuration root directory (e.g. from TargetConfig)
	RefreshCache      bool   // Force refresh models.dev cache
}

// ModelEnrichResult contains details about the enriched models.
type ModelEnrichResult struct {
	ProviderID      string   `json:"providerId"`
	ModelID         string   `json:"modelId"`
	MatchedDev      string   `json:"matchedDev,omitempty"`
	MatchedProvider string   `json:"matchedProvider,omitempty"`
	MatchedRules    []string `json:"matchedRules,omitempty"`
	FieldsAdded     []string `json:"fieldsAdded,omitempty"`
}

// EnrichSummary summarizes the enrichment operation.
type EnrichSummary struct {
	ConfigFile   string              `json:"configFile"`
	BackupFile   string              `json:"backupFile,omitempty"`
	Modified     bool                `json:"modified"`
	Results      []ModelEnrichResult `json:"results"`
	DiffText     string              `json:"diffText,omitempty"`
	Candidate    []byte              `json:"-"`
	OriginalHash string              `json:"-"`
}

// AgentAdapter abstracts an agent's configuration loading, parsing, and updating.
type AgentAdapter interface {
	AgentName() string
	DetectConfigFiles(workspaceRoot string, customConfigDirs ...string) ([]string, error)
	Enrich(ctx context.Context, matcher *modelsdev.Matcher, rules []modelrules.Rule, opts EnrichOptions) (*EnrichSummary, error)
}
