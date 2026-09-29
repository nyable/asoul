package catalog_test

import (
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/model"
)

func TestInitAndLoadWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-cat-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wsRoot := filepath.Join(tmpDir, "workspace")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}

	cat, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cat.Version != 1 {
		t.Fatalf("expected catalog version 1, got %d", cat.Version)
	}

	// Double init should fail
	if err := catalog.InitWorkspace(wsRoot); err == nil {
		t.Fatalf("expected double init to fail, got nil")
	}

	// Add skill and save
	cat.Skills["test-skill"] = model.SkillRecord{
		Source: model.SourceSpec{Type: model.SourceTypeManaged},
		Resolved: model.Resolved{
			ContentHash: "h1:dummy",
		},
	}
	if err := catalog.Save(wsRoot, cat); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload
	reloaded, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if _, ok := reloaded.Skills["test-skill"]; !ok {
		t.Fatalf("expected test-skill to exist after save and load")
	}
}

func TestFindWorkspaceRoot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-find-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wsRoot := filepath.Join(tmpDir, "my-skills")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	// 1. Explicit root flag always works
	found, err := catalog.FindWorkspaceRoot(wsRoot, nil)
	if err != nil || found != wsRoot {
		t.Fatalf("expected found root %s, got %s (err: %v)", wsRoot, found, err)
	}

	// 2. Upward search from child dir still works when cfg is nil (standalone mode)
	childDir := filepath.Join(wsRoot, "skills", "child")
	_ = os.MkdirAll(childDir, 0755)

	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)
	_ = os.Chdir(childDir)

	foundUp, err := catalog.FindWorkspaceRoot("", nil)
	if err != nil || foundUp != wsRoot {
		t.Fatalf("expected upward search to find %s, got %s (err: %v)", wsRoot, foundUp, err)
	}

	// 3. Config-based discovery: uses default_root from config, NOT cwd
	cfgWS := filepath.Join(tmpDir, "config-ws")
	if err := catalog.InitWorkspace(cfgWS); err != nil {
		t.Fatal(err)
	}
	cfg := &model.Config{DefaultRoot: cfgWS}
	foundCfg, err := catalog.FindWorkspaceRoot("", cfg)
	if err != nil || foundCfg != cfgWS {
		t.Fatalf("expected config-based discovery to find %s, got %s (err: %v)", cfgWS, foundCfg, err)
	}

	// 4. When config is provided, cwd is NOT used for discovery even if cwd contains catalog.json
	// (we are still in childDir under wsRoot, but cfg points to cfgWS)
	foundCfg2, err := catalog.FindWorkspaceRoot("", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if foundCfg2 != cfgWS {
		t.Fatalf("expected config workspace %s to be used instead of cwd, got %s", cfgWS, foundCfg2)
	}
}
