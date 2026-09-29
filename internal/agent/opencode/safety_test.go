package opencode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/agent"
	"asoul/internal/jsonc"
	"asoul/internal/modelsdev"
)

func TestEnrichPreservesModeAndPersistsNestedFields(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "custom": {
      "models": {
        "gpt-5": {"limit": {"context": 100}}
      }
    }
  }
}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		t.Fatal(err)
	}

	matcher := modelsdev.NewMatcher(modelsdev.Catalog{
		"openai": {
			ID: "openai",
			Models: map[string]modelsdev.ModelData{
				"gpt-5": {
					ID:   "gpt-5",
					Name: "GPT-5",
					Limit: &modelsdev.LimitData{
						Context: 200,
						Output:  50,
					},
				},
			},
		},
	})

	summary, err := NewAdapter().Enrich(context.Background(), matcher, nil, agent.EnrichOptions{FilePath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Modified {
		t.Fatal("expected nested output field to mark configuration modified")
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("config mode widened to %o", got)
	}

	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := jsonc.Unmarshal(updated, &root); err != nil {
		t.Fatal(err)
	}
	modelMap := root["provider"].(map[string]any)["custom"].(map[string]any)["models"].(map[string]any)["gpt-5"].(map[string]any)
	limit := modelMap["limit"].(map[string]any)
	if got := limit["output"]; got != float64(50) {
		t.Fatalf("expected nested output field to be saved, got %v", got)
	}
	if got := limit["context"]; got != float64(100) {
		t.Fatalf("existing nested context was overwritten, got %v", got)
	}

	backupInfo, err := os.Stat(summary.BackupFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := backupInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("backup mode widened to %o", got)
	}
	if got := backupInfo.IsDir(); got {
		t.Fatal("backup path is a directory")
	}
}
