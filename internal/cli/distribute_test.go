package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/cli"
	"asoul/internal/model"
)

func setupTestEnvironment(t *testing.T) (string, string, string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "asoul-cli-test-*")
	if err != nil {
		t.Fatal(err)
	}

	wsRoot := filepath.Join(tmpDir, "ws")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	// Create a managed skill
	skillDir := filepath.Join(wsRoot, "skills", "test-skill")
	_ = os.MkdirAll(skillDir, 0755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test-skill\ndescription: Test skill\n---\n# Content\n"), 0644)

	cat, _ := catalog.Load(wsRoot)
	cat.Skills["test-skill"] = model.SkillRecord{
		Source: model.SourceSpec{Type: model.SourceTypeManaged},
	}
	_ = catalog.Save(wsRoot, cat)

	cfgPath := filepath.Join(tmpDir, "config.jsonc")
	projectDir := filepath.Join(tmpDir, "sample-project")
	_ = os.MkdirAll(projectDir, 0755)

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}

	return wsRoot, cfgPath, projectDir, cleanup
}

func executeCommand(args ...string) (string, error) {
	rootCmd := cli.NewRootCmd()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(args)

	err := rootCmd.Execute()
	return buf.String(), err
}

func TestCLIDeployParameterValidation(t *testing.T) {
	wsRoot, cfgPath, projectDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	baseArgs := []string{"--root", wsRoot, "--config", cfgPath}

	// 1. Missing both --global and --project
	args := append(baseArgs, "deploy", "test-skill")
	_, err := executeCommand(args...)
	if err == nil || !strings.Contains(err.Error(), "must specify either --global <program> or --project <path>") {
		t.Fatalf("expected missing target error, got: %v", err)
	}

	// 2. Both --global and --project
	args = append(baseArgs, "deploy", "test-skill", "--global", "claude", "--project", projectDir)
	_, err = executeCommand(args...)
	if err == nil || !strings.Contains(err.Error(), "--global and --project are mutually exclusive") {
		t.Fatalf("expected mutually exclusive error, got: %v", err)
	}

	// 3. --global with --format
	args = append(baseArgs, "deploy", "test-skill", "--global", "claude", "--format", "standard")
	_, err = executeCommand(args...)
	if err == nil || !strings.Contains(err.Error(), "--format is only valid when using --project") {
		t.Fatalf("expected format with global error, got: %v", err)
	}

	// 4. Unsupported / unconfigured global program
	args = append(baseArgs, "deploy", "test-skill", "--global", "invalid-program")
	_, err = executeCommand(args...)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected not configured error, got: %v", err)
	}

	// 5. Unsupported format
	args = append(baseArgs, "deploy", "test-skill", "--project", projectDir, "--format", "bad-format")
	_, err = executeCommand(args...)
	if err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got: %v", err)
	}

	// 6. Non-existent project path
	args = append(baseArgs, "deploy", "test-skill", "--project", filepath.Join(projectDir, "nonexistent"))
	_, err = executeCommand(args...)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected project not exist error, got: %v", err)
	}
}

func TestCLIProjectDeployAndManagement(t *testing.T) {
	wsRoot, cfgPath, projectDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	baseArgs := []string{"--root", wsRoot, "--config", cfgPath}

	// 1. Deploy to project with multi-format standard,claude
	args := append(baseArgs, "deploy", "test-skill", "--project", projectDir, "--format", "standard,claude")
	out, err := executeCommand(args...)
	if err != nil {
		t.Fatalf("deploy command failed: %v (output: %s)", err, out)
	}
	if !strings.Contains(out, "Deployed skill \"test-skill\"") {
		t.Fatalf("expected success output, got: %s", out)
	}

	// Verify target files
	if _, err := os.Stat(filepath.Join(projectDir, ".agents", "skills", "test-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected .agents/skills/test-skill/SKILL.md")
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".claude", "skills", "test-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected .claude/skills/test-skill/SKILL.md")
	}

	// 2. Project list should now contain projectDir
	args = append(baseArgs, "project", "list")
	out, err = executeCommand(args...)
	if err != nil {
		t.Fatalf("project list failed: %v", err)
	}
	if !strings.Contains(out, projectDir) {
		t.Fatalf("expected project list to contain %s, got: %s", projectDir, out)
	}

	// 3. Undeploy format claude
	args = append(baseArgs, "undeploy", "test-skill", "--project", projectDir, "--format", "claude")
	out, err = executeCommand(args...)
	if err != nil {
		t.Fatalf("undeploy claude failed: %v (out: %s)", err, out)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".claude", "skills", "test-skill")); !os.IsNotExist(err) {
		t.Fatalf("expected .claude/skills/test-skill to be removed")
	}

	// 4. Undeploy remaining formats
	args = append(baseArgs, "undeploy", "test-skill", "--project", projectDir)
	out, err = executeCommand(args...)
	if err != nil {
		t.Fatalf("undeploy all failed: %v (out: %s)", err, out)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".agents", "skills", "test-skill")); !os.IsNotExist(err) {
		t.Fatalf("expected .agents/skills/test-skill to be removed")
	}

	// 5. Remove project from cache
	args = append(baseArgs, "project", "remove", projectDir)
	out, err = executeCommand(args...)
	if err != nil {
		t.Fatalf("project remove failed: %v (out: %s)", err, out)
	}

	args = append(baseArgs, "project", "list")
	out, err = executeCommand(args...)
	if err != nil {
		t.Fatalf("project list after remove failed: %v", err)
	}
	if strings.Contains(out, projectDir) {
		t.Fatalf("expected project to be removed from cache, got: %s", out)
	}
}

func TestCLIGlobalDeployAndUndeploy(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	baseArgs := []string{"--root", wsRoot, "--config", cfgPath}

	// 1. Deploy to global standard
	args := append(baseArgs, "deploy", "test-skill", "--global", "standard")
	out, err := executeCommand(args...)
	if err != nil {
		t.Fatalf("global deploy failed: %v (out: %s)", err, out)
	}
	if !strings.Contains(out, "Deployed skill \"test-skill\" to global program \"standard\"") {
		t.Fatalf("expected success message, got: %s", out)
	}

	// 2. Undeploy from global standard
	args = append(baseArgs, "undeploy", "test-skill", "--global", "standard")
	out, err = executeCommand(args...)
	if err != nil {
		t.Fatalf("global undeploy failed: %v (out: %s)", err, out)
	}
	if !strings.Contains(out, "Undeployed skill \"test-skill\" from global program \"standard\"") {
		t.Fatalf("expected success message, got: %s", out)
	}
}
