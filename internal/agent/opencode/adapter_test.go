package opencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/agent"
	"asoul/internal/jsonc"
	"asoul/internal/modelrules"
	"asoul/internal/modelsdev"
)

func TestOpenCodeAdapterEnrich(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "opencode-adapter-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "opencode.jsonc")
	rawJSONC := []byte(`{
		// Custom gateway
		"provider": {
			"custom-proxy": {
				"options": {
					"baseURL": "https://api.myproxy.com/v1",
				},
				"models": {
					"gpt-5": {}, // empty model definition
					"claude-3-7-sonnet": {
						"limit": {
							"context": 200000,
						},
					},
				},
			},
		},
	}`)

	if err := os.WriteFile(configPath, rawJSONC, 0644); err != nil {
		t.Fatal(err)
	}

	// Mock models.dev catalog
	catalog := modelsdev.Catalog{
		"openai": {
			ID: "openai",
			Models: map[string]modelsdev.ModelData{
				"gpt-5": {
					ID:        "gpt-5",
					Name:      "GPT-5",
					Reasoning: true,
					ToolCall:  true,
					Limit:     &modelsdev.LimitData{Context: 400000, Output: 128000},
					Cost:      &modelsdev.CostData{Input: 5.0, Output: 15.0},
				},
			},
		},
		"anthropic": {
			ID: "anthropic",
			Models: map[string]modelsdev.ModelData{
				"anthropic/claude-3-7-sonnet": {
					ID:        "anthropic/claude-3-7-sonnet",
					Name:      "Claude 3.7 Sonnet",
					Reasoning: true,
					ToolCall:  true,
					Limit:     &modelsdev.LimitData{Context: 200000, Output: 64000},
					Cost:      &modelsdev.CostData{Input: 3.0, Output: 15.0},
				},
			},
		},
	}
	matcher := modelsdev.NewMatcher(catalog)

	// Custom rules for OpenCode
	rules := []modelrules.Rule{
		{
			Name:    "gpt-rule",
			Pattern: `^gpt-.+`,
			Override: map[string]any{
				"options": map[string]any{
					"reasoningEffort": "high",
				},
				"variants": map[string]any{
					"high": map[string]any{"reasoningEffort": "high"},
					"none": map[string]any{"reasoningEffort": "none"},
				},
			},
		},
	}

	adapter := NewAdapter()

	// 1. Dry run test
	summary, err := adapter.Enrich(context.Background(), matcher, rules, agent.EnrichOptions{
		FilePath: configPath,
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("dry run enrich failed: %v", err)
	}
	if !summary.Modified {
		t.Errorf("expected modified to be true")
	}
	if summary.BackupFile != "" {
		t.Errorf("dry run should not create backup file")
	}

	// Verify file was NOT modified in dry run
	curContent, _ := os.ReadFile(configPath)
	if string(curContent) != string(rawJSONC) {
		t.Errorf("dry run modified the file!")
	}

	// 2. Real execution test
	summary, err = adapter.Enrich(context.Background(), matcher, rules, agent.EnrichOptions{
		FilePath: configPath,
		DryRun:   false,
	})
	if err != nil {
		t.Fatalf("real enrich failed: %v", err)
	}
	if summary.BackupFile == "" {
		t.Errorf("expected backup file to be created")
	} else {
		if filepath.Dir(summary.BackupFile) != filepath.Join(tempDir, ".asoul") {
			t.Errorf("expected backup in .asoul dir, got %s", summary.BackupFile)
		}
	}

	for _, r := range summary.Results {
		if r.ModelID == "gpt-5" && r.MatchedProvider != "openai" {
			t.Errorf("expected gpt-5 matched provider openai, got %s", r.MatchedProvider)
		}
		if r.ModelID == "claude-3-7-sonnet" && r.MatchedProvider != "anthropic" {
			t.Errorf("expected claude-3-7-sonnet matched provider anthropic, got %s", r.MatchedProvider)
		}
	}

	// Verify updated file contents
	updatedBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var root map[string]any
	if err := jsonc.Unmarshal(updatedBytes, &root); err != nil {
		t.Fatalf("failed to unmarshal updated config: %v", err)
	}

	models := root["provider"].(map[string]any)["custom-proxy"].(map[string]any)["models"].(map[string]any)

	// Check gpt-5
	gpt5 := models["gpt-5"].(map[string]any)
	if gpt5["name"] != "GPT-5" {
		t.Errorf("expected GPT-5 name, got %v", gpt5["name"])
	}
	limit := gpt5["limit"].(map[string]any)
	if limit["context"] != float64(400000) {
		t.Errorf("expected context 400000, got %v", limit["context"])
	}
	options := gpt5["options"].(map[string]any)
	if options["reasoningEffort"] != "high" {
		t.Errorf("expected reasoningEffort high, got %v", options["reasoningEffort"])
	}
	variants := gpt5["variants"].(map[string]any)
	if variants["high"] == nil || variants["none"] == nil {
		t.Errorf("expected high and none variants, got %v", variants)
	}

	// Check claude-3-7-sonnet
	claude := models["claude-3-7-sonnet"].(map[string]any)
	if claude["name"] != "Claude 3.7 Sonnet" {
		t.Errorf("expected Claude 3.7 Sonnet name, got %v", claude["name"])
	}
	claudeLimit := claude["limit"].(map[string]any)
	if claudeLimit["output"] != float64(64000) {
		t.Errorf("expected output 64000, got %v", claudeLimit["output"])
	}
}

func TestDetectConfigFilesWithCustomConfigDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "opencode-custom-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	customDir := filepath.Join(tempDir, "custom-opencode")
	_ = os.MkdirAll(customDir, 0755)
	cfgFile := filepath.Join(customDir, "opencode.json")
	if err := os.WriteFile(cfgFile, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	adapter := NewAdapter()
	candidates, err := adapter.DetectConfigFiles("", customDir)
	if err != nil {
		t.Fatalf("DetectConfigFiles failed: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatalf("expected at least 1 candidate from customConfigDirs")
	}
	if candidates[0] != cfgFile {
		t.Fatalf("expected candidate %s, got %s", cfgFile, candidates[0])
	}
}

func TestBuildDiffLCS(t *testing.T) {
	oldText := "line1\nline2\nline3\nline4\n"
	newText := "line1\nline2-modified\nline3\nline3.5-inserted\nline4\n"

	diff := buildDiff(oldText, newText)
	if diff == "" {
		t.Fatalf("expected non-empty diff")
	}
	if !strings.Contains(diff, "--- original") || !strings.Contains(diff, "+++ updated") {
		t.Fatalf("expected unified diff header, got: %s", diff)
	}
	if !strings.Contains(diff, "- line2") || !strings.Contains(diff, "+ line2-modified") {
		t.Fatalf("expected replacement of line2, got: %s", diff)
	}
	if !strings.Contains(diff, "+ line3.5-inserted") {
		t.Fatalf("expected inserted line, got: %s", diff)
	}
	if !strings.Contains(diff, "  line1") || !strings.Contains(diff, "  line3") {
		t.Fatalf("expected context lines preserved, got: %s", diff)
	}

	// Identical text should return empty
	if buildDiff(oldText, oldText) != "" {
		t.Fatalf("expected empty diff for identical texts")
	}
}

func TestOpenCodeEnrichPreservesCommentsAndOrder(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "opencode.jsonc")
	raw := `{
  // top comment
  "provider": {
    "custom": {
      "models": {
        "gpt-5": {
          // model comment
          "limit": { "context": 100 }
        }
      }
    }
  }
}`
	if err := os.WriteFile(configPath, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	matcher := modelsdev.NewMatcher(modelsdev.Catalog{
		"openai": {
			ID: "openai",
			Models: map[string]modelsdev.ModelData{
				"gpt-5": {
					ID:    "gpt-5",
					Name:  "GPT-5",
					Limit: &modelsdev.LimitData{Context: 200, Output: 50},
				},
			},
		},
	})

	summary, err := NewAdapter().Enrich(context.Background(), matcher, nil, agent.EnrichOptions{FilePath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Modified {
		t.Fatal("expected enrichment to modify the config")
	}

	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(updated)
	if !strings.Contains(out, "// top comment") || !strings.Contains(out, "// model comment") {
		t.Fatalf("expected comments preserved, got:\n%s", out)
	}
	if !strings.Contains(out, `"output": 50`) {
		t.Fatalf("expected nested output added, got:\n%s", out)
	}
	limitIdx := strings.Index(out, `"limit"`)
	nameIdx := strings.Index(out, `"name"`)
	if limitIdx < 0 || nameIdx < 0 || limitIdx > nameIdx {
		t.Fatalf("expected existing key order preserved (limit before appended name), got:\n%s", out)
	}
}
