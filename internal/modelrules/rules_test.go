package modelrules

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyRules(t *testing.T) {
	rules := []Rule{
		{
			Name:    "gpt-rule",
			Pattern: `^gpt-.+`,
			Override: map[string]any{
				"options": map[string]any{
					"reasoningEffort": "high",
				},
				"variants": map[string]any{
					"high": map[string]any{"reasoningEffort": "high"},
				},
				"headers": map[string]any{
					"X-Custom-Header": "value123",
				},
				"customArbitraryField": 999,
			},
		},
	}

	base := map[string]any{
		"name": "GPT-5",
		"limit": map[string]any{
			"context": 400000,
		},
	}

	// Test matched
	res, matched, err := ApplyRules("gpt-5-preview", rules, base, Context{})
	if err != nil {
		t.Fatalf("ApplyRules returned error: %v", err)
	}
	if len(matched) != 1 || matched[0] != "gpt-rule" {
		t.Fatalf("expected gpt-rule to match, got %v", matched)
	}

	options, ok := res["options"].(map[string]any)
	if !ok || options["reasoningEffort"] != "high" {
		t.Errorf("options reasoningEffort not injected: %v", res["options"])
	}

	variants, ok := res["variants"].(map[string]any)
	if !ok || variants["high"] == nil {
		t.Errorf("variants high not injected: %v", res["variants"])
	}

	headers, ok := res["headers"].(map[string]any)
	if !ok || headers["X-Custom-Header"] != "value123" {
		t.Errorf("headers not injected: %v", res["headers"])
	}

	if res["customArbitraryField"] != 999 {
		t.Errorf("expected custom arbitrary field supported, got %v", res["customArbitraryField"])
	}

	// Test non-matched
	res2, matched2, err := ApplyRules("claude-3-7-sonnet", rules, base, Context{})
	if err != nil {
		t.Fatalf("ApplyRules returned error: %v", err)
	}
	if len(matched2) != 0 {
		t.Errorf("expected 0 matches for claude model, got %v", matched2)
	}
	if _, ok := res2["options"]; ok {
		t.Errorf("options should not be injected on non-matching model")
	}
}

func TestLoadRuleFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "rules-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	rulePath := filepath.Join(tempDir, "opencode.json")
	content := []byte(`{
		// Test comments
		"rules": [
			{
				"name": "test-rule",
				"pattern": "^test-.+", // trailing comma test
				"override": {
					"options": { "flag": true },
				},
			},
		],
	}`)

	if err := os.WriteFile(rulePath, content, 0644); err != nil {
		t.Fatal(err)
	}

	rf, err := LoadRuleFile(rulePath)
	if err != nil {
		t.Fatalf("failed to load rule file: %v", err)
	}
	if len(rf.Rules) != 1 || rf.Rules[0].Name != "test-rule" {
		t.Errorf("unexpected rules: %v", rf.Rules)
	}
}

func TestLoadRuleFileRejectsUnknownFields(t *testing.T) {
	tempDir := t.TempDir()
	rulePath := filepath.Join(tempDir, "opencode.json")
	content := []byte(`{
		"agent": "opencode",
		"rules": []
	}`)
	if err := os.WriteFile(rulePath, content, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRuleFile(rulePath); err == nil {
		t.Fatal("expected the legacy agent field to be rejected")
	}
}

func TestApplyRulesModesAndVariables(t *testing.T) {
	now := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	rules := []Rule{
		{
			Name:     "claude-rule",
			Pattern:  `^(claude)-(.+)$`,
			Fill:     map[string]any{"family": "${1}"},
			Merge:    map[string]any{"tags": []any{"overlay"}},
			Override: map[string]any{"name": "${model_id}", "release_date": "${date}"},
			Remove:   []string{"secret"},
		},
	}
	base := map[string]any{
		"family": "existing",
		"tags":   []any{"base"},
		"secret": "x",
		"keep":   true,
	}

	res, matched, err := ApplyRules("claude-3", rules, base, Context{ProviderID: "anthropic", Now: now})
	if err != nil {
		t.Fatalf("ApplyRules error: %v", err)
	}
	if len(matched) != 1 || matched[0] != "claude-rule" {
		t.Fatalf("expected rule match, got %v", matched)
	}
	if res["family"] != "existing" {
		t.Errorf("fill must not overwrite existing value, got %v", res["family"])
	}
	tags, _ := res["tags"].([]any)
	if len(tags) != 2 || tags[0] != "base" || tags[1] != "overlay" {
		t.Errorf("expected merged tag union [base overlay], got %v", res["tags"])
	}
	if res["name"] != "claude-3" {
		t.Errorf("expected ${model_id} rendered, got %v", res["name"])
	}
	if res["release_date"] != "2024-05-06" {
		t.Errorf("expected ${date} rendered from injected clock, got %v", res["release_date"])
	}
	if _, ok := res["secret"]; ok {
		t.Errorf("expected secret key removed")
	}
}

func TestApplyRulesOrderControlsExecution(t *testing.T) {
	base := map[string]any{}

	fillFirst := []Rule{{
		Name:     "r",
		Pattern:  `.*`,
		Order:    []string{"fill", "override"},
		Fill:     map[string]any{"v": "fill"},
		Override: map[string]any{"v": "override"},
	}}
	res, _, err := ApplyRules("m", fillFirst, base, Context{})
	if err != nil {
		t.Fatal(err)
	}
	if res["v"] != "override" {
		t.Fatalf("with fill before override, override should win, got %v", res["v"])
	}

	overrideFirst := []Rule{{
		Name:     "r",
		Pattern:  `.*`,
		Order:    []string{"override", "fill"},
		Fill:     map[string]any{"v": "fill"},
		Override: map[string]any{"v": "override"},
	}}
	res, _, err = ApplyRules("m", overrideFirst, base, Context{})
	if err != nil {
		t.Fatal(err)
	}
	if res["v"] != "override" {
		t.Fatalf("expected override value retained, got %v", res["v"])
	}
}

func TestApplyRulesOrderValidation(t *testing.T) {
	rules := []Rule{{
		Name:     "bad",
		Pattern:  `.*`,
		Order:    []string{"override"},
		Override: map[string]any{"a": 1},
		Remove:   []string{"b"},
	}}
	if _, _, err := ApplyRules("m", rules, map[string]any{}, Context{}); err == nil {
		t.Fatal("expected error when order omits a configured operation")
	}

	dup := []Rule{{
		Name:    "dup",
		Pattern: `.*`,
		Order:   []string{"fill", "fill"},
		Fill:    map[string]any{"a": 1},
	}}
	if _, _, err := ApplyRules("m", dup, map[string]any{}, Context{}); err == nil {
		t.Fatal("expected error on duplicate order entries")
	}
}
