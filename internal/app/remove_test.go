package app_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/app"
	"asoul/internal/catalog"
)

func TestRemoveBatch(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	_, err := svc.InitWorkspace(ctx, wsRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Create test skills
	skillIDs := []string{"skill-alpha", "skill-beta", "skill-gamma", "skill-delta"}
	for _, id := range skillIDs {
		if err := svc.NewSkill(ctx, id); err != nil {
			t.Fatalf("failed to create skill %s: %v", id, err)
		}
	}

	// Add skills to a profile/group
	if err := svc.GroupCreate("dev-tools"); err != nil {
		t.Fatal(err)
	}
	if err := svc.GroupSetSkills("dev-tools", []string{"skill-alpha", "skill-beta"}); err != nil {
		t.Fatal(err)
	}

	// Modify skill-gamma locally
	gammaDir := catalog.SkillDir(wsRoot, "skill-gamma")
	if err := os.WriteFile(filepath.Join(gammaDir, "modified.txt"), []byte("local edit"), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Test RemoveBatch without force:
	// skill-alpha (clean -> should remove)
	// skill-gamma (modified -> should fail)
	// non-existent (not found -> should fail)
	// invalid/id (invalid -> should fail)
	res, err := svc.RemoveBatch(ctx, []string{"skill-alpha", "skill-gamma", "non-existent", "invalid/id"}, false)
	if err != nil {
		t.Fatalf("unexpected error from RemoveBatch: %v", err)
	}

	if len(res.Removed) != 1 || res.Removed[0] != "skill-alpha" {
		t.Fatalf("expected only skill-alpha removed, got: %v", res.Removed)
	}
	if _, ok := res.Failed["skill-gamma"]; !ok {
		t.Fatalf("expected skill-gamma to fail due to local modification, got: %v", res.Failed)
	}
	if _, ok := res.Failed["non-existent"]; !ok {
		t.Fatalf("expected non-existent to fail, got: %v", res.Failed)
	}
	if _, ok := res.Failed["invalid/id"]; !ok {
		t.Fatalf("expected invalid/id to fail, got: %v", res.Failed)
	}

	// Verify dev-tools group pruned skill-alpha
	grp, err := svc.GroupGet("dev-tools")
	if err != nil {
		t.Fatal(err)
	}
	if len(grp.Skills) != 1 || grp.Skills[0] != "skill-beta" {
		t.Fatalf("expected dev-tools to only retain skill-beta, got: %v", grp.Skills)
	}

	// 2. Test RemoveBatch with force=true on skill-gamma
	resForce, err := svc.RemoveBatch(ctx, []string{"skill-gamma"}, true)
	if err != nil {
		t.Fatalf("unexpected error with force: %v", err)
	}
	if len(resForce.Removed) != 1 || resForce.Removed[0] != "skill-gamma" {
		t.Fatalf("expected skill-gamma removed with force, got: %v", resForce.Removed)
	}
}

func TestFilterSkills(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	_, err := svc.InitWorkspace(ctx, wsRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Create test skills
	_ = svc.NewSkill(ctx, "azure-compute")
	_ = svc.NewSkill(ctx, "azure-storage")
	_ = svc.NewSkill(ctx, "aws-s3")
	_ = svc.NewSkill(ctx, "gcp-cloudrun")

	// 1. Filter by pattern (regex)
	azureSkills, err := svc.FilterSkills(ctx, app.SkillFilterOptions{
		Pattern: "^azure-.*",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(azureSkills) != 2 {
		t.Fatalf("expected 2 azure skills, got %d", len(azureSkills))
	}

	// 2. Filter by ID subset + pattern
	matched, err := svc.FilterSkills(ctx, app.SkillFilterOptions{
		IDs:     []string{"azure-compute", "aws-s3"},
		Pattern: "compute",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != 1 || matched[0].ID != "azure-compute" {
		t.Fatalf("expected only azure-compute, got: %v", matched)
	}

	// 3. Filter by invalid regex pattern
	_, err = svc.FilterSkills(ctx, app.SkillFilterOptions{
		Pattern: "[invalid-regex(",
	})
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

func TestSkillDeployments(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	_, err := svc.InitWorkspace(ctx, wsRoot)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.NewSkill(ctx, "deploy-target-skill"); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(tmpDir, "agent-target")
	_ = os.MkdirAll(targetDir, 0755)
	if err := svc.TargetAdd("test-agent", targetDir); err != nil {
		t.Fatal(err)
	}

	// Deploy skill
	if err := svc.Deploy(ctx, "deploy-target-skill", "test-agent", false); err != nil {
		t.Fatalf("failed to deploy: %v", err)
	}

	// Verify deployments
	deps := svc.SkillDeployments("deploy-target-skill")
	if len(deps) == 0 {
		t.Fatalf("expected deployments for deploy-target-skill, got none")
	}
	found := false
	for _, d := range deps {
		if d == "test-agent (agent)" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected test-agent (agent) in deployments, got: %v", deps)
	}

	allDeps := svc.AllSkillDeployments()
	if len(allDeps["deploy-target-skill"]) == 0 {
		t.Fatalf("expected deploy-target-skill in all deployments map")
	}
}
