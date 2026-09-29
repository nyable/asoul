package modelsdev

// Catalog maps provider_id -> ProviderData
type Catalog map[string]ProviderData

// ProviderData contains provider-level details and available models.
type ProviderData struct {
	ID     string               `json:"id"`
	Name   string               `json:"name"`
	API    string               `json:"api"`
	Doc    string               `json:"doc"`
	NPM    string               `json:"npm"`
	ENV    []string             `json:"env"`
	Models map[string]ModelData `json:"models"`
}

// ModelData represents a single model from models.dev.
type ModelData struct {
	ID               string          `json:"id"`
	ProviderID       string          `json:"provider_id,omitempty"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Family           string          `json:"family"`
	Attachment       bool            `json:"attachment"`
	Reasoning        bool            `json:"reasoning"`
	ReasoningOptions []ReasoningOpt  `json:"reasoning_options,omitempty"`
	ToolCall         bool            `json:"tool_call"`
	StructuredOutput bool            `json:"structured_output"`
	Temperature      bool            `json:"temperature"`
	ReleaseDate      string          `json:"release_date"`
	LastUpdated      string          `json:"last_updated"`
	Modalities       *ModalitiesData `json:"modalities,omitempty"`
	OpenWeights      bool            `json:"open_weights"`
	Limit            *LimitData      `json:"limit,omitempty"`
	Cost             *CostData       `json:"cost,omitempty"`
	Status           string          `json:"status,omitempty"`
}

// ReasoningOpt specifies reasoning capability variants (e.g., effort, budget_tokens, toggle).
type ReasoningOpt struct {
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
}

// ModalitiesData contains input and output media types.
type ModalitiesData struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// LimitData defines token boundaries.
type LimitData struct {
	Context int `json:"context"`
	Input   int `json:"input,omitempty"`
	Output  int `json:"output"`
}

// CostData defines token pricing.
type CostData struct {
	Input           float64          `json:"input"`
	Output          float64          `json:"output"`
	CacheRead       float64          `json:"cache_read,omitempty"`
	CacheWrite      float64          `json:"cache_write,omitempty"`
	ContextOver200k *ContextOver200k `json:"context_over_200k,omitempty"`
}

// ContextOver200k defines tiered pricing above 200k context.
type ContextOver200k struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}
