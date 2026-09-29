package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/cli"
	"asoul/internal/hash"
	"asoul/internal/model"
)

func createTestSkill(wsRoot, id string, source model.SourceSpec) {
	skillDir := filepath.Join(wsRoot, "skills", id)
	_ = os.MkdirAll(skillDir, 0755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: "+id+"\ndescription: Test skill\n---\n# Content\n"), 0644)

	h, _ := hash.HashDir(skillDir)
	cat, _ := catalog.Load(wsRoot)
	if cat.Skills == nil {
		cat.Skills = make(map[string]model.SkillRecord)
	}
	cat.Skills[id] = model.SkillRecord{
		Source: source,
		Resolved: model.Resolved{
			ContentHash: h,
		},
	}
	_ = catalog.Save(wsRoot, cat)
}

func executeCommandWithStdout(args ...string) (string, string, error) {
	rootCmd := cli.NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	rootCmd.SetOut(&outBuf)
	rootCmd.SetErr(&errBuf)
	rootCmd.SetArgs(args)

	err := rootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestCLIRemoveBatch(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	createTestSkill(wsRoot, "azure-vm", model.SourceSpec{Type: model.SourceTypeGit, URL: "https://github.com/microsoft/skills"})
	createTestSkill(wsRoot, "azure-storage", model.SourceSpec{Type: model.SourceTypeGit, URL: "https://github.com/microsoft/skills"})
	createTestSkill(wsRoot, "aws-ec2", model.SourceSpec{Type: model.SourceTypeLocal, Path: "/tmp/aws"})
	createTestSkill(wsRoot, "gcp-gke", model.SourceSpec{Type: model.SourceTypeManaged})

	// 1. Test missing args and flags
	_, err := executeCommand("--root", wsRoot, "--config", cfgPath, "remove")
	if err == nil {
		t.Fatal("expected error when no skill ID or filters provided")
	}

	// 2. Test dry-run
	outDry, err := executeCommand("--root", wsRoot, "--config", cfgPath, "rm", "--pattern", "azure-.*", "--dry-run")
	if err != nil {
		t.Fatalf("unexpected error during dry-run: %v", err)
	}
	if !strings.Contains(outDry, "Dry run: 2 skill(s) matched") {
		t.Fatalf("expected 2 skills matched in dry-run, got:\n%s", outDry)
	}
	// Verify skills still exist
	cat, _ := catalog.Load(wsRoot)
	if _, ok := cat.Skills["azure-vm"]; !ok {
		t.Fatal("azure-vm should not be deleted after dry-run")
	}

	// 3. Test dry-run JSON
	outJSONDry, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "rm", "--pattern", "azure-.*", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("unexpected error during json dry-run: %v", err)
	}
	var dryData map[string]interface{}
	if err := json.Unmarshal([]byte(outJSONDry), &dryData); err != nil {
		t.Fatalf("expected valid JSON for dry-run, got error: %v, output: %s", err, outJSONDry)
	}
	if dryData["dryRun"] != true {
		t.Fatalf("expected dryRun: true in JSON output: %v", dryData)
	}

	// 4. Test batch removal by pattern with -y
	outRm, err := executeCommand("--root", wsRoot, "--config", cfgPath, "rm", "-p", "azure-.*", "-y")
	if err != nil {
		t.Fatalf("failed to remove by pattern: %v", err)
	}
	if !strings.Contains(outRm, "Removed 2 skill(s)") {
		t.Fatalf("expected removal confirmation, got:\n%s", outRm)
	}
	cat, _ = catalog.Load(wsRoot)
	if _, ok := cat.Skills["azure-vm"]; ok {
		t.Fatal("azure-vm should be removed from catalog")
	}
	if _, ok := cat.Skills["azure-storage"]; ok {
		t.Fatal("azure-storage should be removed from catalog")
	}

	// 5. Test batch removal by source type with JSON output
	outJSON, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "rm", "--source-type", "local", "-y", "--json")
	if err != nil {
		t.Fatalf("failed to remove by source-type: %v", err)
	}
	var resData map[string]interface{}
	if err := json.Unmarshal([]byte(outJSON), &resData); err != nil {
		t.Fatalf("expected valid JSON, got: %s", outJSON)
	}
	removedList, _ := resData["removed"].([]interface{})
	if len(removedList) != 1 || removedList[0].(string) != "aws-ec2" {
		t.Fatalf("expected aws-ec2 in removed list, got: %v", resData)
	}

	// 6. Test multi-ID removal with one non-existent skill
	outPartial, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "rm", "gcp-gke", "non-existent-skill", "-y", "--json")
	if err == nil {
		t.Fatal("expected non-zero exit when a requested skill does not exist")
	}
	var partialData map[string]interface{}
	if err := json.Unmarshal([]byte(outPartial), &partialData); err != nil {
		t.Fatalf("expected valid JSON even on partial failure, got: %s", outPartial)
	}
	failedMap, _ := partialData["failed"].(map[string]interface{})
	if failedMap["non-existent-skill"] == nil {
		t.Fatalf("expected non-existent-skill in failed map: %v", partialData)
	}
}

func TestCLIRemoveDeployedSafety(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	createTestSkill(wsRoot, "deployed-skill", model.SourceSpec{Type: model.SourceTypeManaged})

	// Add a target and deploy the skill
	targetDir := filepath.Join(filepath.Dir(cfgPath), "test-agent-dir")
	_ = os.MkdirAll(targetDir, 0755)
	_, err := executeCommand("--root", wsRoot, "--config", cfgPath, "target", "add", "agent-x", targetDir)
	if err != nil {
		t.Fatalf("failed to add target: %v", err)
	}
	_, err = executeCommand("--root", wsRoot, "--config", cfgPath, "deploy", "deployed-skill", "--global", "agent-x")
	if err != nil {
		t.Fatalf("failed to deploy: %v", err)
	}

	// 1. In non-interactive mode without -y or -f, attempting to remove deployed skill should fail with warning
	outNonInteractive, err := executeCommand("--root", wsRoot, "--config", cfgPath, "--non-interactive", "rm", "deployed-skill")
	if err == nil {
		t.Fatal("expected error when deleting deployed skill in non-interactive mode without -y/-f")
	}
	if !strings.Contains(outNonInteractive, "deployed to active targets") {
		t.Fatalf("expected warning about active deployment, got:\n%s", outNonInteractive)
	}

	// Skill should still exist in catalog
	cat, _ := catalog.Load(wsRoot)
	if _, ok := cat.Skills["deployed-skill"]; !ok {
		t.Fatal("deployed-skill should not be deleted without confirmation")
	}

	// 2. With -y confirmation, it should succeed
	outConfirmed, err := executeCommand("--root", wsRoot, "--config", cfgPath, "rm", "deployed-skill", "-y")
	if err != nil {
		t.Fatalf("expected successful deletion with -y, got: %v\nOutput: %s", err, outConfirmed)
	}
	cat, _ = catalog.Load(wsRoot)
	if _, ok := cat.Skills["deployed-skill"]; ok {
		t.Fatal("deployed-skill should now be removed from catalog")
	}
}
