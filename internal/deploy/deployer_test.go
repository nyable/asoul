package deploy_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/deploy"
	"asoul/internal/model"
	"asoul/internal/state"
)

func createTestWorkspaceWithSkill(t *testing.T, skillID string) (string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "asoul-deploy-test-*")
	if err != nil {
		t.Fatal(err)
	}

	wsRoot := filepath.Join(tmpDir, "ws")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(wsRoot, "skills", skillID)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skillMD := "---\nname: " + skillID + "\ndescription: Test skill for deploy\n---\n# " + skillID + "\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}

	cat, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	cat.Skills[skillID] = model.SkillRecord{
		Source: model.SourceSpec{
			Type: model.SourceTypeManaged,
		},
	}
	if err := catalog.Save(wsRoot, cat); err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return tmpDir, cleanup
}

func TestResolveProjectFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-format-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. Explicit standard format
	resolved, fallback, err := deploy.ResolveProjectFormats(tmpDir, []string{"standard"})
	if err != nil || len(resolved) != 1 || resolved[0] != "standard" || fallback {
		t.Fatalf("expected [standard], got %v (fallback: %v, err: %v)", resolved, fallback, err)
	}

	// 2. Explicit multi-format
	resolved, fallback, err = deploy.ResolveProjectFormats(tmpDir, []string{"standard,claude", "cursor"})
	if err != nil || len(resolved) != 3 {
		t.Fatalf("expected 3 formats, got %v (err: %v)", resolved, err)
	}

	// 3. Unsupported format
	_, _, err = deploy.ResolveProjectFormats(tmpDir, []string{"nonexistent-format"})
	if err == nil {
		t.Fatalf("expected error for unsupported format, got nil")
	}

	// 4. Auto mode without existing agent dirs -> fallback to standard
	resolved, fallback, err = deploy.ResolveProjectFormats(tmpDir, []string{"auto"})
	if err != nil || !fallback || len(resolved) != 1 || resolved[0] != "standard" {
		t.Fatalf("expected auto fallback to [standard], got %v (fallback: %v, err: %v)", resolved, fallback, err)
	}

	// 5. Auto mode with existing .claude and .cursor dirs
	_ = os.MkdirAll(filepath.Join(tmpDir, ".claude"), 0755)
	_ = os.MkdirAll(filepath.Join(tmpDir, ".cursor"), 0755)

	resolved, fallback, err = deploy.ResolveProjectFormats(tmpDir, []string{"auto"})
	if err != nil || fallback {
		t.Fatalf("expected detected formats without fallback, err: %v", err)
	}

	hasClaude, hasCursor := false, false
	for _, f := range resolved {
		if f == "claude" {
			hasClaude = true
		}
		if f == "cursor" {
			hasCursor = true
		}
	}
	if !hasClaude || !hasCursor {
		t.Fatalf("expected claude and cursor in auto detected formats, got %v", resolved)
	}
}

func TestDeployGlobalAndUndeploy(t *testing.T) {
	ctx := context.Background()
	tmpDir, cleanup := createTestWorkspaceWithSkill(t, "oracle-skill")
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	stateFile := filepath.Join(tmpDir, "deployments.json")
	stateMgr, err := state.NewManager(stateFile)
	if err != nil {
		t.Fatal(err)
	}

	deployer := deploy.NewDeployer(stateMgr)
	globalDir := filepath.Join(tmpDir, "global-agents", "claude", "skills")

	// Deploy global
	if err := deployer.DeployGlobal(ctx, wsRoot, "claude", globalDir, "oracle-skill", false); err != nil {
		t.Fatalf("DeployGlobal failed: %v", err)
	}

	destFile := filepath.Join(globalDir, "oracle-skill", "SKILL.md")
	if _, err := os.Stat(destFile); err != nil {
		t.Fatalf("expected deployed file at %s", destFile)
	}

	// Tamper target file
	_ = os.WriteFile(destFile, []byte("tampered content"), 0644)

	// Deploy without force must fail
	if err := deployer.DeployGlobal(ctx, wsRoot, "claude", globalDir, "oracle-skill", false); err == nil {
		t.Fatalf("expected DeployGlobal to fail when tampered without force")
	}

	// Deploy with force must succeed
	if err := deployer.DeployGlobal(ctx, wsRoot, "claude", globalDir, "oracle-skill", true); err != nil {
		t.Fatalf("DeployGlobal with force failed: %v", err)
	}

	// Undeploy without force on tampered file
	_ = os.WriteFile(destFile, []byte("tampered again"), 0644)
	if err := deployer.UndeployGlobal(ctx, "claude", globalDir, "oracle-skill", false); err == nil {
		t.Fatalf("expected UndeployGlobal to fail when modified without force")
	}

	// Undeploy with force
	if err := deployer.UndeployGlobal(ctx, "claude", globalDir, "oracle-skill", true); err != nil {
		t.Fatalf("UndeployGlobal with force failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(globalDir, "oracle-skill")); !os.IsNotExist(err) {
		t.Fatalf("expected skill dir to be removed after undeploy")
	}
}

func TestDeployProjectMultiFormatAndAuto(t *testing.T) {
	ctx := context.Background()
	tmpDir, cleanup := createTestWorkspaceWithSkill(t, "db-helper")
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	stateFile := filepath.Join(tmpDir, "deployments.json")
	stateMgr, _ := state.NewManager(stateFile)
	deployer := deploy.NewDeployer(stateMgr)

	projectDir := filepath.Join(tmpDir, "my-repo")
	_ = os.MkdirAll(projectDir, 0755)

	// 1. Deploy with auto (no existing agent dirs -> fallback to standard)
	deployed, fallback, err := deployer.DeployProject(ctx, wsRoot, projectDir, []string{"auto"}, "db-helper", false)
	if err != nil {
		t.Fatalf("DeployProject auto failed: %v", err)
	}
	if !fallback || len(deployed) != 1 || deployed[0] != "standard" {
		t.Fatalf("expected fallback to standard, got deployed=%v, fallback=%v", deployed, fallback)
	}

	stdDest := filepath.Join(projectDir, ".agents", "skills", "db-helper", "SKILL.md")
	if _, err := os.Stat(stdDest); err != nil {
		t.Fatalf("expected standard deployment file at %s", stdDest)
	}

	// Verify project state in .agents/.asoul-state.json
	projState, err := state.LoadProjectState(projectDir)
	if err != nil {
		t.Fatalf("LoadProjectState failed: %v", err)
	}
	skillRec, ok := projState.Skills["db-helper"]
	if !ok || skillRec.Formats["standard"].Format != "standard" {
		t.Fatalf("expected db-helper standard format in project state")
	}

	// 2. Deploy with multi-format: claude and cline
	deployed2, _, err := deployer.DeployProject(ctx, wsRoot, projectDir, []string{"claude", "cline"}, "db-helper", false)
	if err != nil {
		t.Fatalf("DeployProject claude and cline failed: %v", err)
	}
	if len(deployed2) != 2 {
		t.Fatalf("expected 2 deployed formats, got %v", deployed2)
	}

	claudeDest := filepath.Join(projectDir, ".claude", "skills", "db-helper", "SKILL.md")
	clineDest := filepath.Join(projectDir, ".cline", "skills", "db-helper", "SKILL.md")
	if _, err := os.Stat(claudeDest); err != nil {
		t.Fatalf("expected claude deployment at %s", claudeDest)
	}
	if _, err := os.Stat(clineDest); err != nil {
		t.Fatalf("expected cline deployment at %s", clineDest)
	}

	// Verify updated state has all 3 formats
	projState, _ = state.LoadProjectState(projectDir)
	if len(projState.Skills["db-helper"].Formats) != 3 {
		t.Fatalf("expected 3 formats in state, got %d", len(projState.Skills["db-helper"].Formats))
	}

	// 3. Undeploy single format: claude
	undeployed, err := deployer.UndeployProject(ctx, projectDir, []string{"claude"}, "db-helper", false)
	if err != nil {
		t.Fatalf("UndeployProject claude failed: %v", err)
	}
	if len(undeployed) != 1 || undeployed[0] != "claude" {
		t.Fatalf("expected [claude] undeployed, got %v", undeployed)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".claude", "skills", "db-helper")); !os.IsNotExist(err) {
		t.Fatalf("expected .claude/skills/db-helper to be removed")
	}
	// standard must still exist
	if _, err := os.Stat(stdDest); err != nil {
		t.Fatalf("expected standard skill to still exist")
	}

	// 4. Undeploy all remaining formats (empty formats parameter)
	undeployedAll, err := deployer.UndeployProject(ctx, projectDir, nil, "db-helper", false)
	if err != nil {
		t.Fatalf("UndeployProject all failed: %v", err)
	}
	if len(undeployedAll) != 2 { // standard and cline
		t.Fatalf("expected 2 remaining formats undeployed, got %v", undeployedAll)
	}
	if _, err := os.Stat(stdDest); !os.IsNotExist(err) {
		t.Fatalf("expected standard skill to be removed after undeploy all")
	}

	projState, _ = state.LoadProjectState(projectDir)
	if _, ok := projState.Skills["db-helper"]; ok {
		t.Fatalf("expected db-helper to be removed from project state")
	}
}
