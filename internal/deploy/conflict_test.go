package deploy_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/deploy"
	"asoul/internal/state"
)

func TestTargetConflictErrorGlobal(t *testing.T) {
	tmp, cleanup := createTestWorkspaceWithSkill(t, "pdf")
	defer cleanup()

	targetDir := filepath.Join(tmp, "global-target")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	stateMgr, _ := state.NewManager(filepath.Join(tmp, "state.json"))
	deployer := deploy.NewDeployer(stateMgr)
	wsRoot := filepath.Join(tmp, "ws")

	// 1. Untracked collision
	destSkillDir := filepath.Join(targetDir, "pdf")
	if err := os.MkdirAll(destSkillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destSkillDir, "SKILL.md"), []byte("---\nname: pdf\ndescription: untracked\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := deployer.DeployGlobal(context.Background(), wsRoot, "claude", targetDir, "pdf", false)
	if err == nil {
		t.Fatal("expected collision error")
	}
	var confErr *deploy.TargetConflictError
	if !errors.As(err, &confErr) {
		t.Fatalf("expected *deploy.TargetConflictError, got %T: %v", err, err)
	}
	if !confErr.IsUntracked || confErr.IsProject || confErr.SkillID != "pdf" || confErr.Target != "claude" {
		t.Fatalf("unexpected conflict details: %+v", confErr)
	}

	// 2. Force deploy to establish initial baseline
	if err := deployer.DeployGlobal(context.Background(), wsRoot, "claude", targetDir, "pdf", true); err != nil {
		t.Fatalf("force deploy failed: %v", err)
	}

	// 3. Manually modify target skill
	if err := os.WriteFile(filepath.Join(destSkillDir, "custom.txt"), []byte("manual change"), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Redeploy without force -> should return TargetConflictError (modified)
	err = deployer.DeployGlobal(context.Background(), wsRoot, "claude", targetDir, "pdf", false)
	if err == nil {
		t.Fatal("expected modification conflict error")
	}
	confErr = nil
	if !errors.As(err, &confErr) {
		t.Fatalf("expected *deploy.TargetConflictError, got %T: %v", err, err)
	}
	if confErr.IsUntracked || confErr.IsProject || confErr.SkillID != "pdf" || confErr.Target != "claude" {
		t.Fatalf("unexpected conflict details: %+v", confErr)
	}
}

func TestTargetConflictErrorProject(t *testing.T) {
	tmp, cleanup := createTestWorkspaceWithSkill(t, "pdf")
	defer cleanup()

	projectRoot := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectRoot, 0755); err != nil {
		t.Fatal(err)
	}
	stateMgr, _ := state.NewManager(filepath.Join(tmp, "state.json"))
	deployer := deploy.NewDeployer(stateMgr)
	wsRoot := filepath.Join(tmp, "ws")

	// 1. Initial deploy
	if _, _, err := deployer.DeployProject(context.Background(), wsRoot, projectRoot, []string{"standard"}, "pdf", false); err != nil {
		t.Fatalf("initial project deploy failed: %v", err)
	}

	// 2. Manually modify deployed target skill
	targetSkillDir := filepath.Join(projectRoot, ".agents", "skills", "pdf")
	if err := os.WriteFile(filepath.Join(targetSkillDir, "SKILL.md"), []byte("---\nname: pdf\ndescription: user modified in project\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Redeploy without force -> must return TargetConflictError
	_, _, err := deployer.DeployProject(context.Background(), wsRoot, projectRoot, []string{"standard"}, "pdf", false)
	if err == nil {
		t.Fatal("expected modification conflict error in project")
	}
	var confErr *deploy.TargetConflictError
	if !errors.As(err, &confErr) {
		t.Fatalf("expected *deploy.TargetConflictError, got %T: %v", err, err)
	}
	if confErr.IsUntracked || !confErr.IsProject || confErr.SkillID != "pdf" || confErr.Format != "standard" {
		t.Fatalf("unexpected project conflict details: %+v", confErr)
	}
}
