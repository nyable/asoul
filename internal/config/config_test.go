package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/config"
	"asoul/internal/fsx"
	"asoul/internal/model"
)

func TestEditorConfigSurvivesLockedMutations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.jsonc")
	mgr, err := config.NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Update(func(cfg *model.Config) error {
		cfg.Editor = &model.EditorConfig{Command: `C:\Program Files\Neovim\bin\nvim.exe`, Args: []string{"--clean"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddUpstream(model.UpstreamConfig{URL: "file://fixture", Type: model.SourceTypeGit}); err != nil {
		t.Fatal(err)
	}
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Editor == nil || cfg.Editor.Command != `C:\Program Files\Neovim\bin\nvim.exe` || len(cfg.Editor.Args) != 1 || cfg.Editor.Args[0] != "--clean" {
		t.Fatalf("editor config lost: %+v", cfg.Editor)
	}
}

func TestWorkspaceDeduplicationAndNormalization(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.jsonc")
	mgr, err := config.NewManager(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial default workspaces
	expectedDef := config.DefaultWorkspacePathCompact()
	wsList := mgr.GetWorkspaces()
	if len(wsList) != 1 || wsList[0] != expectedDef {
		t.Fatalf("expected [%s], got %v", expectedDef, wsList)
	}

	// 2. Add absolute path of default workspace - should NOT add duplicate!
	absDefault, _ := fsx.ExpandUser(expectedDef)
	if err := mgr.AddWorkspace(absDefault); err != nil {
		t.Fatal(err)
	}
	wsList = mgr.GetWorkspaces()
	if len(wsList) != 1 {
		t.Fatalf("expected 1 workspace after adding equivalent absolute path, got %v", wsList)
	}

	// 3. Add relative path "asd"
	if err := mgr.AddWorkspace("asd"); err != nil {
		t.Fatal(err)
	}
	wsList = mgr.GetWorkspaces()
	if len(wsList) != 2 {
		t.Fatalf("expected 2 workspaces, got %v", wsList)
	}

	// 4. Add equivalent absolute path of "asd" - should NOT add duplicate!
	cwd, _ := os.Getwd()
	absASD := filepath.Join(cwd, "asd")
	if err := mgr.AddWorkspace(absASD); err != nil {
		t.Fatal(err)
	}
	wsList = mgr.GetWorkspaces()
	if len(wsList) != 2 {
		t.Fatalf("expected still 2 workspaces, got %v", wsList)
	}

	// 5. Test loading file directly when file manually contains duplicate variants, JSONC comments and trailing commas
	manualJSONC := `// User asoul configuration
{
  "version": 1,
  "default_root": "asd",
  "workspaces": [
    "` + expectedDef + `",
    "` + strings.ReplaceAll(absDefault, `\`, `\\`) + `",
    "asd",
    "` + strings.ReplaceAll(absASD, `\`, `\\`) + `", // trailing comma test
  ],
  "targets": {
    "opencode": {
      "type": "agent",
      "channel": "opencode",
      "config_dir": "~/.config/opencode",
      "paths": {
        "skills": "${config_dir}/skills",
      },
    },
  },
}
`
	if err := os.WriteFile(cfgFile, []byte(manualJSONC), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != 2 {
		t.Fatalf("expected Load() to deduplicate to 2 workspaces, got %v", cfg.Workspaces)
	}
	if !fsx.SamePath(cfg.DefaultRoot, absASD) {
		t.Fatalf("expected DefaultRoot to be normalized to asd path, got %v", cfg.DefaultRoot)
	}

	tgt, skillsDir, err := mgr.GetTarget("opencode")
	if err != nil {
		t.Fatalf("expected opencode target to exist: %v", err)
	}
	if tgt.ConfigDir != "~/.config/opencode" {
		t.Fatalf("expected config_dir ~/.config/opencode, got %s", tgt.ConfigDir)
	}
	if !strings.HasSuffix(skillsDir, filepath.Join(".config", "opencode", "skills")) {
		t.Fatalf("expected resolved skills path ending in .config/opencode/skills, got %s", skillsDir)
	}

	// 6. Test RemoveWorkspace with variant representation
	if err := mgr.RemoveWorkspace(absASD); err != nil {
		t.Fatal(err)
	}
	wsList = mgr.GetWorkspaces()
	if len(wsList) != 1 || !fsx.SamePath(wsList[0], expectedDef) {
		t.Fatalf("expected only %s remaining, got %v", expectedDef, wsList)
	}

	// 7. Test SetTargetEnabled & SetProjectEnabled
	if err := mgr.SetTargetEnabled("opencode", false); err != nil {
		t.Fatal(err)
	}
	tgt2, _, _ := mgr.GetTarget("opencode")
	if tgt2.IsEnabled() {
		t.Fatalf("expected opencode to be disabled")
	}

	projPath := filepath.Join(tmpDir, "my-project")
	if err := mgr.SetProjectEnabled(projPath, false); err != nil {
		t.Fatal(err)
	}
	en, err := mgr.IsProjectEnabled(projPath)
	if err != nil || en {
		t.Fatalf("expected my-project to be disabled, got %v (err: %v)", en, err)
	}
	if err := mgr.SetProjectEnabled(projPath, true); err != nil {
		t.Fatal(err)
	}
	en, _ = mgr.IsProjectEnabled(projPath)
	if !en {
		t.Fatalf("expected my-project to be re-enabled")
	}
}

func TestOfficialProvidersConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.jsonc")
	mgr, err := config.NewManager(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initially without file on disk, returns defaults
	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	defaults := cfg.GetOfficialProviders()
	if len(defaults["openai"]) == 0 || defaults["openai"][0] != "(?i)^gpt-" {
		t.Fatalf("expected default openai regex patterns, got %v", defaults["openai"])
	}

	// 2. Custom override
	custom := map[string][]string{
		"my-lab": {"(?i)^custom-.*"},
	}
	if err := mgr.SetOfficialProviders(custom); err != nil {
		t.Fatal(err)
	}

	// 3. Reload from disk
	cfg2, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	loaded := cfg2.GetOfficialProviders()
	if len(loaded["my-lab"]) != 1 || loaded["my-lab"][0] != "(?i)^custom-.*" {
		t.Fatalf("expected custom providers to be loaded, got %v", loaded)
	}
	if len(loaded["openai"]) != 0 {
		t.Fatalf("expected custom providers to completely override, got %v", loaded)
	}
}
