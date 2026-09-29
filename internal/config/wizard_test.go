package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/config"
)

func TestDefaultConfigCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-cfg-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgFile := filepath.Join(tmpDir, "asoul", "config.jsonc")
	mgr, err := config.NewManager(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	if mgr.Exists() {
		t.Fatalf("expected config to not exist yet")
	}

	// Create default config
	if err := config.CreateDefaultConfig(mgr); err != nil {
		t.Fatalf("CreateDefaultConfig failed: %v", err)
	}

	if !mgr.Exists() {
		t.Fatalf("expected config to exist after CreateDefaultConfig")
	}

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	expectedWS := config.DefaultWorkspacePathCompact()
	if loaded.DefaultRoot != expectedWS {
		t.Fatalf("expected default_root to be %s, got %s", expectedWS, loaded.DefaultRoot)
	}

	if len(loaded.Targets) == 0 {
		t.Fatalf("expected default targets to be created, got 0")
	}

	if _, ok := loaded.Targets["codex"]; !ok {
		t.Fatalf("expected codex target in default targets")
	}

	opencodeTgt, ok := loaded.Targets["opencode"]
	if !ok {
		t.Fatalf("expected opencode target in default targets")
	}
	if opencodeTgt.ConfigDir != "~/.config/opencode" {
		t.Fatalf("expected opencode config_dir ~/.config/opencode, got %s", opencodeTgt.ConfigDir)
	}
	if opencodeTgt.Paths["skills"] != "${config_dir}/skills" {
		t.Fatalf("expected opencode skills path ${config_dir}/skills, got %s", opencodeTgt.Paths["skills"])
	}
}
