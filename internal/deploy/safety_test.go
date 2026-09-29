package deploy_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/deploy"
	"asoul/internal/state"
)

func TestUndeployRejectsInvalidAndUntrackedSkills(t *testing.T) {
	for _, id := range []string{"manual-skill", "../../victim"} {
		t.Run(id, func(t *testing.T) {
			tmp := t.TempDir()
			target := filepath.Join(tmp, "target")
			victim := filepath.Clean(filepath.Join(target, id))
			if err := os.MkdirAll(victim, 0755); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(victim, "user.txt")
			if err := os.WriteFile(marker, []byte("user-owned"), 0644); err != nil {
				t.Fatal(err)
			}
			stateMgr, _ := state.NewManager(filepath.Join(tmp, "state.json"))

			err := deploy.NewDeployer(stateMgr).UndeployGlobal(context.Background(), "test", target, id, false)
			if err == nil {
				t.Fatal("expected protected undeploy to fail")
			}
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("protected file was changed: %v", err)
			}
		})
	}
}

func TestDeployRollsBackWhenStateCannotBeSaved(t *testing.T) {
	tmp, cleanup := createTestWorkspaceWithSkill(t, "sample")
	defer cleanup()

	stateBlock := filepath.Join(tmp, "state-block")
	if err := os.Mkdir(stateBlock, 0755); err != nil {
		t.Fatal(err)
	}
	stateMgr, _ := state.NewManager(stateBlock)
	target := filepath.Join(tmp, "target")
	err := deploy.NewDeployer(stateMgr).DeployGlobal(
		context.Background(), filepath.Join(tmp, "ws"), "test", target, "sample", false,
	)
	if err == nil {
		t.Fatal("expected state save failure")
	}
	if _, err := os.Stat(filepath.Join(target, "sample")); !os.IsNotExist(err) {
		t.Fatal("failed deployment left a published directory")
	}
}

func TestProjectUndeployAutoUsesRecordedFormats(t *testing.T) {
	tmp, cleanup := createTestWorkspaceWithSkill(t, "sample")
	defer cleanup()

	project := filepath.Join(tmp, "project")
	if err := os.Mkdir(project, 0755); err != nil {
		t.Fatal(err)
	}
	stateMgr, _ := state.NewManager(filepath.Join(tmp, "state.json"))
	deployer := deploy.NewDeployer(stateMgr)
	if _, _, err := deployer.DeployProject(
		context.Background(), filepath.Join(tmp, "ws"), project, []string{"standard"}, "sample", false,
	); err != nil {
		t.Fatal(err)
	}
	formats, err := deployer.UndeployProject(context.Background(), project, []string{"auto"}, "sample", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(formats) != 1 || formats[0] != "standard" {
		t.Fatalf("unexpected removed formats: %v", formats)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents", "skills", "sample")); !os.IsNotExist(err) {
		t.Fatal("recorded standard deployment was not removed")
	}
}

func TestDeployGlobalRecordsTargetAndWorkspace(t *testing.T) {
	tmp, cleanup := createTestWorkspaceWithSkill(t, "sample")
	defer cleanup()

	target := filepath.Join(tmp, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	stateMgr, _ := state.NewManager(filepath.Join(tmp, "state.json"))
	deployer := deploy.NewDeployer(stateMgr)
	wsRoot := filepath.Join(tmp, "ws")

	err := deployer.DeployGlobal(context.Background(), wsRoot, "test-target", target, "sample", false)
	if err != nil {
		t.Fatalf("DeployGlobal failed: %v", err)
	}

	rec, ok, err := stateMgr.GetRecord("test-target", "sample")
	if err != nil || !ok || rec == nil {
		t.Fatalf("expected record in state, got ok=%v err=%v", ok, err)
	}
	if rec.TargetPath == "" || !strings.Contains(rec.TargetPath, "target") {
		t.Fatalf("expected TargetPath recorded, got %q", rec.TargetPath)
	}
	if rec.Workspace == "" || !strings.Contains(rec.Workspace, "ws") {
		t.Fatalf("expected Workspace recorded, got %q", rec.Workspace)
	}
}
