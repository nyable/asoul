package agentmodel

import (
	"testing"
)

func TestDeepMerge(t *testing.T) {
	base := map[string]any{
		"name": "base-model",
		"options": map[string]any{
			"temp": 0.5,
			"url":  "http://local",
		},
		"variants": map[string]any{
			"low": map[string]any{"level": "low"},
		},
		"customProp": "keepMe",
	}

	overlay := map[string]any{
		"name": "overridden-model",
		"options": map[string]any{
			"temp":            1.0,
			"reasoningEffort": "high",
		},
		"variants": map[string]any{
			"high": map[string]any{"level": "high"},
		},
		"headers": map[string]any{
			"X-Auth": "token",
		},
	}

	merged := DeepMerge(base, overlay)

	if merged["name"] != "overridden-model" {
		t.Errorf("expected name overridden, got %v", merged["name"])
	}
	if merged["customProp"] != "keepMe" {
		t.Errorf("expected customProp to be preserved, got %v", merged["customProp"])
	}

	options := merged["options"].(map[string]any)
	if options["temp"] != 1.0 || options["url"] != "http://local" || options["reasoningEffort"] != "high" {
		t.Errorf("options not properly merged: %v", options)
	}

	variants := merged["variants"].(map[string]any)
	if _, ok := variants["low"]; !ok {
		t.Errorf("expected variant 'low' to be retained")
	}
	if _, ok := variants["high"]; !ok {
		t.Errorf("expected variant 'high' to be added")
	}

	headers := merged["headers"].(map[string]any)
	if headers["X-Auth"] != "token" {
		t.Errorf("expected headers to be added, got %v", headers)
	}
}

func TestFillMissing(t *testing.T) {
	base := map[string]any{
		"limit": map[string]any{
			"context": 128000,
		},
	}

	source := map[string]any{
		"name": "GPT-5",
		"limit": map[string]any{
			"context": 400000,
			"output":  16000,
		},
		"cost": map[string]any{
			"input": 5.0,
		},
	}

	filled := FillMissing(base, source)

	limit := filled["limit"].(map[string]any)
	if limit["context"] != 128000 {
		t.Errorf("expected existing context 128000 preserved, got %v", limit["context"])
	}
	if limit["output"] != 16000 {
		t.Errorf("expected missing output 16000 filled, got %v", limit["output"])
	}
	if filled["name"] != "GPT-5" {
		t.Errorf("expected missing name filled, got %v", filled["name"])
	}
}

func TestFromMapToMap(t *testing.T) {
	raw := map[string]any{
		"name":        "Test Model",
		"arbitrary":   "value123",
		"myNestedObj": map[string]any{"k": "v"},
	}

	spec, err := FromMap(raw)
	if err != nil {
		t.Fatalf("FromMap failed: %v", err)
	}
	if spec.Name != "Test Model" {
		t.Errorf("expected Name 'Test Model', got %s", spec.Name)
	}
	if spec.ExtraFields["arbitrary"] != "value123" {
		t.Errorf("expected extra field 'arbitrary' captured, got %v", spec.ExtraFields["arbitrary"])
	}

	out, err := spec.ToMap()
	if err != nil {
		t.Fatalf("ToMap failed: %v", err)
	}
	if out["arbitrary"] != "value123" {
		t.Errorf("expected ToMap to preserve extra fields, got %v", out["arbitrary"])
	}
}
