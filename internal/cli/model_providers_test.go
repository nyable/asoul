package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/cli"
)

func TestCLIModelProvidersCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-model-prov-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wsRoot := filepath.Join(tmpDir, "ws")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmpDir, "config.jsonc")

	// 1. List default authoritative providers via --json
	{
		var stdout, stderr bytes.Buffer
		rootCmd := cli.NewRootCmd()
		rootCmd.SetOut(&stdout)
		rootCmd.SetErr(&stderr)

		rootCmd.SetArgs([]string{"--root", wsRoot, "--config", cfgPath, "model", "providers", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("failed to execute model providers --json: %v", err)
		}

		var jsonResult map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &jsonResult); err != nil {
			t.Fatalf("failed to parse json output: %v, raw: %s", err, stdout.String())
		}
		provs, ok := jsonResult["providers"].(map[string]any)
		if !ok || len(provs) == 0 {
			t.Fatalf("expected non-empty providers map, got: %v", jsonResult)
		}
		if _, hasOpenAI := provs["openai"]; !hasOpenAI {
			t.Fatalf("expected openai provider in default mapping, got: %v", provs)
		}
	}

	// 2. Initialize default providers into config.jsonc
	{
		var stdout, stderr bytes.Buffer
		rootCmd := cli.NewRootCmd()
		rootCmd.SetOut(&stdout)
		rootCmd.SetErr(&stderr)

		rootCmd.SetArgs([]string{"--root", wsRoot, "--config", cfgPath, "model", "providers", "--init"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("failed to execute model providers --init: %v", err)
		}
		if !strings.Contains(stdout.String(), "Successfully initialized") {
			t.Fatalf("expected success message on init, got: %s", stdout.String())
		}
	}

	// 3. Verify config file exists and contains official_providers
	content, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("expected config.jsonc to exist on disk: %v", err)
	}
	if !strings.Contains(string(content), "official_providers") {
		t.Fatalf("expected config.jsonc to contain official_providers, got:\n%s", string(content))
	}
}
