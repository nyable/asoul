package agentmodel

import (
	"encoding/json"
)

// ModelSpec represents the configuration of a single LLM model.
// It supports all standard and custom fields for OpenCode and other agents.
type ModelSpec struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Family       string `json:"family,omitempty"`
	ReleaseDate  string `json:"release_date,omitempty"`
	Attachment   *bool  `json:"attachment,omitempty"`
	Reasoning    *bool  `json:"reasoning,omitempty"`
	Temperature  *bool  `json:"temperature,omitempty"`
	ToolCall     *bool  `json:"tool_call,omitempty"`
	Interleaved  any    `json:"interleaved,omitempty"`
	Status       string `json:"status,omitempty"`
	Experimental *bool  `json:"experimental,omitempty"`

	Limit      *LimitSpec      `json:"limit,omitempty"`
	Cost       *CostSpec       `json:"cost,omitempty"`
	Modalities *ModalitiesSpec `json:"modalities,omitempty"`
	Provider   *ProviderRef    `json:"provider,omitempty"`

	Options  map[string]any `json:"options,omitempty"`
	Variants map[string]any `json:"variants,omitempty"`
	Headers  map[string]any `json:"headers,omitempty"`

	// ExtraFields preserves any arbitrary/unknown fields not explicitly captured above
	ExtraFields map[string]any `json:"-"`
}

// LimitSpec defines token limit boundaries.
type LimitSpec struct {
	Context *int `json:"context,omitempty"`
	Input   *int `json:"input,omitempty"`
	Output  *int `json:"output,omitempty"`
}

// CostSpec defines token pricing details in USD per million tokens.
type CostSpec struct {
	Input           *float64         `json:"input,omitempty"`
	Output          *float64         `json:"output,omitempty"`
	CacheRead       *float64         `json:"cache_read,omitempty"`
	CacheWrite      *float64         `json:"cache_write,omitempty"`
	ContextOver200k *ContextOver200k `json:"context_over_200k,omitempty"`
}

// ContextOver200k defines tiered pricing for tokens over 200k context.
type ContextOver200k struct {
	Input      *float64 `json:"input,omitempty"`
	Output     *float64 `json:"output,omitempty"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
}

// ModalitiesSpec defines supported input and output modalities.
type ModalitiesSpec struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// ProviderRef defines provider package or api bindings.
type ProviderRef struct {
	NPM string `json:"npm,omitempty"`
	API string `json:"api,omitempty"`
}

// ToMap converts a ModelSpec to a map[string]any, preserving all fields and extra custom fields.
func (m *ModelSpec) ToMap() (map[string]any, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var res map[string]any
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	if res == nil {
		res = make(map[string]any)
	}
	for k, v := range m.ExtraFields {
		if _, exists := res[k]; !exists {
			res[k] = v
		}
	}
	return res, nil
}

// FromMap populates a ModelSpec from a map[string]any, capturing known and extra fields.
func FromMap(raw map[string]any) (*ModelSpec, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var spec ModelSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}

	knownKeys := map[string]bool{
		"id": true, "name": true, "family": true, "release_date": true,
		"attachment": true, "reasoning": true, "temperature": true, "tool_call": true,
		"interleaved": true, "status": true, "experimental": true,
		"limit": true, "cost": true, "modalities": true, "provider": true,
		"options": true, "variants": true, "headers": true,
	}

	spec.ExtraFields = make(map[string]any)
	for k, v := range raw {
		if !knownKeys[k] {
			spec.ExtraFields[k] = v
		}
	}

	return &spec, nil
}
