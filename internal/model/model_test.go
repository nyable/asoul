package model_test

import (
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/model"
)

func TestTargetConfigResolve(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	tgt := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "opencode",
		ConfigDir: "~/.config/opencode",
		Paths: map[string]string{
			"skills": "${config_dir}/skills",
			"rules":  "${config_dir}/rules",
			"custom": "/opt/custom/path",
		},
	}

	expectedConfigDir := filepath.Join(home, ".config", "opencode")
	resolvedConfigDir := tgt.ResolveConfigDir()
	if resolvedConfigDir != expectedConfigDir {
		t.Fatalf("expected config dir %q, got %q", expectedConfigDir, resolvedConfigDir)
	}

	expectedSkillsDir := filepath.Join(home, ".config", "opencode", "skills")
	resolvedSkills := tgt.ResolvePath("skills")
	if resolvedSkills != expectedSkillsDir {
		t.Fatalf("expected skills dir %q, got %q", expectedSkillsDir, resolvedSkills)
	}

	if tgt.SkillsDir() != expectedSkillsDir {
		t.Fatalf("expected SkillsDir() to return %q, got %q", expectedSkillsDir, tgt.SkillsDir())
	}

	expectedRulesDir := filepath.Join(home, ".config", "opencode", "rules")
	resolvedRules := tgt.ResolvePath("rules")
	if resolvedRules != expectedRulesDir {
		t.Fatalf("expected rules dir %q, got %q", expectedRulesDir, resolvedRules)
	}

	resolvedCustom := tgt.ResolvePath("custom")
	if resolvedCustom != "/opt/custom/path" {
		t.Fatalf("expected custom path %q, got %q", "/opt/custom/path", resolvedCustom)
	}

	// Non-existent key
	if tgt.ResolvePath("nonexistent") != "" {
		t.Fatalf("expected empty string for nonexistent key")
	}

	// Features
	featKeys := tgt.FeatureKeys()
	if len(featKeys) != 3 || featKeys[0] != "custom" || featKeys[1] != "rules" || featKeys[2] != "skills" {
		t.Fatalf("expected sorted feature keys [custom, rules, skills], got %v", featKeys)
	}
	if tgt.FeaturesString() != "custom, rules, skills" {
		t.Fatalf("expected features string 'custom, rules, skills', got %q", tgt.FeaturesString())
	}

	// IsEnabled defaults to true
	if !tgt.IsEnabled() {
		t.Fatalf("expected default IsEnabled to be true")
	}

	// Explicit disable
	disabled := false
	tgt.Enabled = &disabled
	if tgt.IsEnabled() {
		t.Fatalf("expected IsEnabled to be false when set to false")
	}
}

func TestConfigProjectEnabled(t *testing.T) {
	cfg := &model.Config{}
	p1 := "/home/user/project1"
	p2 := "/home/user/project2"

	if !cfg.IsProjectEnabled(p1) {
		t.Fatalf("expected project1 to be enabled by default")
	}

	cfg.SetProjectEnabled(p1, false)
	if cfg.IsProjectEnabled(p1) {
		t.Fatalf("expected project1 to be disabled")
	}
	if !cfg.IsProjectEnabled(p2) {
		t.Fatalf("expected project2 to remain enabled")
	}

	cfg.SetProjectEnabled(p1, true)
	if !cfg.IsProjectEnabled(p1) {
		t.Fatalf("expected project1 to be re-enabled")
	}
}
