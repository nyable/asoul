package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/app"
	"asoul/internal/cache"
	"asoul/internal/catalog"
	"asoul/internal/config"
	"asoul/internal/model"
	"asoul/internal/state"
)

func TestTargetReconciliationOnManualDiskDeletion(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "workspace")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(tmpDir, "config.jsonc")
	cfgMgr, _ := config.NewManager(cfgPath)
	stateMgr, _ := state.NewManager(filepath.Join(tmpDir, "state.json"))
	cacheMgr, _ := cache.NewManager(filepath.Join(tmpDir, "cache"))
	svc := app.NewService(wsRoot, cfgMgr, stateMgr, cacheMgr)

	ctx := context.Background()

	// 1. Configure an agent target
	claudeSkillsDir := filepath.Join(tmpDir, "claude-config", "skills")
	_ = os.MkdirAll(claudeSkillsDir, 0755)
	err := svc.TargetAddConfig("claude", model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "claude",
		ConfigDir: filepath.Join(tmpDir, "claude-config"),
	})
	if err != nil {
		t.Fatalf("TargetAddConfig failed: %v", err)
	}

	// 2. Create and deploy two skills
	for _, id := range []string{"skill1", "skill2"} {
		if err := svc.NewSkill(ctx, id); err != nil {
			t.Fatal(err)
		}
		if err := svc.Deploy(ctx, id, "claude", false); err != nil {
			t.Fatalf("Deploy %s failed: %v", id, err)
		}
	}

	// Check deployed skills count
	deployed := svc.TargetDeployedSkills("claude")
	if len(deployed) != 2 {
		t.Fatalf("expected 2 deployed skills, got %d: %v", len(deployed), deployed)
	}

	// 3. Manually delete skill1 on disk
	_ = os.RemoveAll(filepath.Join(claudeSkillsDir, "skill1"))

	// Re-check deployed skills: should now be 1
	deployedAfterDel1 := svc.TargetDeployedSkills("claude")
	if len(deployedAfterDel1) != 1 || deployedAfterDel1[0] != "skill2" {
		t.Fatalf("expected 1 skill (skill2) after deleting skill1, got %v", deployedAfterDel1)
	}

	// 4. Manually delete all remaining skills on disk
	_ = os.RemoveAll(claudeSkillsDir)
	_ = os.MkdirAll(claudeSkillsDir, 0755)

	deployedAfterDelAll := svc.TargetDeployedSkills("claude")
	if len(deployedAfterDelAll) != 0 {
		t.Fatalf("expected 0 skills after deleting all skills on disk, got %v", deployedAfterDelAll)
	}

	// 5. Deploy a new skill3
	if err := svc.NewSkill(ctx, "skill3"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Deploy(ctx, "skill3", "claude", false); err != nil {
		t.Fatalf("Deploy skill3 failed: %v", err)
	}

	deployedAfterNew := svc.TargetDeployedSkills("claude")
	if len(deployedAfterNew) != 1 || deployedAfterNew[0] != "skill3" {
		t.Fatalf("expected 1 skill (skill3), got %v", deployedAfterNew)
	}
}

func TestProjectReconciliationOnManualDiskDeletion(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "workspace")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(tmpDir, "config.jsonc")
	cfgMgr, _ := config.NewManager(cfgPath)
	stateMgr, _ := state.NewManager(filepath.Join(tmpDir, "state.json"))
	cacheMgr, _ := cache.NewManager(filepath.Join(tmpDir, "cache"))
	svc := app.NewService(wsRoot, cfgMgr, stateMgr, cacheMgr)

	ctx := context.Background()

	projectRoot := filepath.Join(tmpDir, "sample-project")
	_ = os.MkdirAll(projectRoot, 0755)

	// 1. Create and deploy two skills to project
	for _, id := range []string{"proj-skill1", "proj-skill2"} {
		if err := svc.NewSkill(ctx, id); err != nil {
			t.Fatal(err)
		}
		if _, _, err := svc.DeployProject(ctx, id, projectRoot, []string{"standard"}, false); err != nil {
			t.Fatalf("DeployProject %s failed: %v", id, err)
		}
	}

	deployed := svc.ProjectDeployedSkills(projectRoot)
	if len(deployed) != 2 {
		t.Fatalf("expected 2 deployed skills in project, got %d: %v", len(deployed), deployed)
	}

	// 2. Manually delete all skills on disk in project
	_ = os.RemoveAll(filepath.Join(projectRoot, ".agents", "skills"))

	deployedAfterDel := svc.ProjectDeployedSkills(projectRoot)
	if len(deployedAfterDel) != 0 {
		t.Fatalf("expected 0 deployed skills after deleting on disk, got %v", deployedAfterDel)
	}

	st, err := svc.ProjectStatus(projectRoot)
	if err != nil {
		t.Fatalf("ProjectStatus failed: %v", err)
	}
	if len(st.Skills) != 0 {
		t.Fatalf("expected ProjectStatus to report 0 skills, got %d", len(st.Skills))
	}
}
