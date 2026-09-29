package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/skill"
)

func TestValidateID(t *testing.T) {
	validIDs := []string{"oracle-helper", "code-review-123", "skill", "a-b-c"}
	for _, id := range validIDs {
		if err := skill.ValidateID(id); err != nil {
			t.Errorf("expected valid ID for %s, got %v", id, err)
		}
	}

	invalidIDs := []string{"", "Oracle-Helper", "skill_name", "-leading-dash", "trailing-dash-", "double--dash"}
	for _, id := range invalidIDs {
		if err := skill.ValidateID(id); err == nil {
			t.Errorf("expected invalid ID for %s, got nil error", id)
		}
	}
}

func TestValidateSkillDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-skill-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	skillDir := filepath.Join(tmpDir, "valid-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Missing SKILL.md
	if _, err := skill.ValidateSkillDir(skillDir, "valid-skill"); err == nil {
		t.Errorf("expected error for missing SKILL.md")
	}

	// 2. Valid SKILL.md
	mdContent := skill.GenerateSkillMD("valid-skill", "A test skill description")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), mdContent, 0644); err != nil {
		t.Fatal(err)
	}

	meta, err := skill.ValidateSkillDir(skillDir, "valid-skill")
	if err != nil {
		t.Fatalf("expected valid skill, got error: %v", err)
	}
	if meta.Name != "valid-skill" || meta.Description != "A test skill description" {
		t.Fatalf("unexpected parsed metadata: %+v", meta)
	}

	// 3. Name mismatch with expected ID
	if _, err := skill.ValidateSkillDir(skillDir, "different-id"); err == nil {
		t.Errorf("expected error when name does not match expected ID")
	}

	// 4. Symlinks forbidden
	symlinkPath := filepath.Join(skillDir, "bad-link")
	_ = os.Symlink(filepath.Join(skillDir, "SKILL.md"), symlinkPath)
	if _, err := skill.ValidateSkillDir(skillDir, "valid-skill"); err == nil {
		t.Errorf("expected error when directory contains symlink")
	}
}
