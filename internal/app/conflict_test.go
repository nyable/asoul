package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/app"
	"asoul/internal/cache"
	"asoul/internal/catalog"
	"asoul/internal/config"
	"asoul/internal/hash"
	"asoul/internal/state"
)

func TestDiffAndAdoptFromTargetProject(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "workspace")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	cfgMgr, _ := config.NewManager(filepath.Join(tmpDir, "config.jsonc"))
	stateMgr, _ := state.NewManager(filepath.Join(tmpDir, "state.json"))
	cacheMgr, _ := cache.NewManager(filepath.Join(tmpDir, "cache"))
	svc := app.NewService(wsRoot, cfgMgr, stateMgr, cacheMgr)

	ctx := context.Background()

	// 1. Create a skill in workspace
	skillID := "pdf"
	if err := svc.NewSkill(ctx, skillID); err != nil {
		t.Fatal(err)
	}

	// 2. Deploy to project
	projectRoot := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.DeployProject(ctx, skillID, projectRoot, []string{"standard"}, false)
	if err != nil {
		t.Fatalf("DeployProject failed: %v", err)
	}

	// 3. Manually modify target skill in project
	targetSkillDir := filepath.Join(projectRoot, ".agents", "skills", skillID)
	modifiedContent := "---\nname: pdf\ndescription: user modified in target project\n---\n# PDF Skill Modified\n"
	if err := os.WriteFile(filepath.Join(targetSkillDir, "SKILL.md"), []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Test DiffTarget
	diff, err := svc.DiffTarget(ctx, skillID, "", projectRoot, "standard")
	if err != nil {
		t.Fatalf("DiffTarget failed: %v", err)
	}
	if !strings.Contains(diff, "user modified in target project") {
		t.Fatalf("expected diff to contain modified text, got:\n%s", diff)
	}

	// 5. Test AdoptFromTarget
	err = svc.AdoptFromTarget(ctx, skillID, "", projectRoot, "standard")
	if err != nil {
		t.Fatalf("AdoptFromTarget failed: %v", err)
	}

	// Verify workspace files were updated
	skillDir := catalog.SkillDir(wsRoot, skillID)
	wsSkillContent, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(wsSkillContent) != modifiedContent {
		t.Fatalf("workspace content was not updated, got:\n%s", string(wsSkillContent))
	}

	// Verify catalog record hash was updated
	catUpdated, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := catUpdated.Skills[skillID]
	if !ok {
		t.Fatal("skill not in catalog")
	}
	newTargetHash, err := hash.HashDir(targetSkillDir)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Resolved.ContentHash != newTargetHash {
		t.Fatalf("catalog contentHash mismatch: expected %s, got %s", newTargetHash, rec.Resolved.ContentHash)
	}

	// Verify subsequent deploy without force succeeds cleanly without conflict!
	_, _, err = svc.DeployProject(ctx, skillID, projectRoot, []string{"standard"}, false)
	if err != nil {
		t.Fatalf("expected re-deploy after adopt to succeed without conflict, got: %v", err)
	}
}

func TestDiffAndAdoptFromTargetGlobal(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "workspace")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(tmpDir, "config.jsonc")
	cfgMgr, _ := config.NewManager(cfgPath)
	stateMgr, _ := state.NewManager(filepath.Join(tmpDir, "state.json"))
	cacheMgr, _ := cache.NewManager(filepath.Join(tmpDir, "cache"))

	// Configure global target
	globalTargetDir := filepath.Join(tmpDir, "claude-skills")
	if err := os.MkdirAll(globalTargetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := cfgMgr.AddTargetSimple("claude", globalTargetDir); err != nil {
		t.Fatal(err)
	}

	svc := app.NewService(wsRoot, cfgMgr, stateMgr, cacheMgr)
	ctx := context.Background()

	// 1. Create skill in workspace
	skillID := "helper"
	if err := svc.NewSkill(ctx, skillID); err != nil {
		t.Fatal(err)
	}

	// 2. Deploy to global
	if err := svc.Deploy(ctx, skillID, "claude", false); err != nil {
		t.Fatalf("DeployGlobal failed: %v", err)
	}

	// 3. Manually edit target
	targetSkillDir := filepath.Join(globalTargetDir, "skills", skillID)
	modifiedContent := "---\nname: helper\ndescription: modified in global claude\n---\n"
	if err := os.WriteFile(filepath.Join(targetSkillDir, "SKILL.md"), []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Test DiffTarget
	diff, err := svc.DiffTarget(ctx, skillID, "claude", "", "")
	if err != nil {
		t.Fatalf("DiffTarget failed: %v", err)
	}
	if !strings.Contains(diff, "modified in global claude") {
		t.Fatalf("expected diff to contain modified text, got: %s", diff)
	}

	// 5. Test AdoptFromTarget
	if err := svc.AdoptFromTarget(ctx, skillID, "claude", "", ""); err != nil {
		t.Fatalf("AdoptFromTarget failed: %v", err)
	}

	// Verify workspace updated
	skillDir := catalog.SkillDir(wsRoot, skillID)
	wsContent, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if string(wsContent) != modifiedContent {
		t.Fatalf("workspace content mismatch: %s", string(wsContent))
	}

	// Verify redeploy succeeds without force
	if err := svc.Deploy(ctx, skillID, "claude", false); err != nil {
		t.Fatalf("redeploy failed after adopt: %v", err)
	}
}
