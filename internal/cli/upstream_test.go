package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/model"
)

func runGitInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed in %s: %v\nOutput: %s", args, dir, err, out)
	}
	return string(out)
}

func setupUpstreamGitRepoForCLI(t *testing.T, parentDir string) string {
	t.Helper()
	repoDir := filepath.Join(parentDir, "remote-upstream")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	runGitInDir(t, repoDir, "init")
	runGitInDir(t, repoDir, "config", "user.name", "Test")
	runGitInDir(t, repoDir, "config", "user.email", "test@example.com")

	// Create skill-a
	skillADir := filepath.Join(repoDir, "skills", "skill-a")
	if err := os.MkdirAll(skillADir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte("---\nname: skill-a\ndescription: Skill A upstream\n---\n# Skill A\n"), 0644)

	// Create skill-b
	skillBDir := filepath.Join(repoDir, "skills", "skill-b")
	if err := os.MkdirAll(skillBDir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(skillBDir, "SKILL.md"), []byte("---\nname: skill-b\ndescription: Skill B upstream\n---\n# Skill B\n"), 0644)

	runGitInDir(t, repoDir, "add", ".")
	runGitInDir(t, repoDir, "commit", "-m", "Initial commit")

	return repoDir
}

func TestCLIUpstreamList(t *testing.T) {
	wsRoot, cfgPath, tmpDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// 1. Initial list when no upstreams exist
	outText, err := executeCommand("--root", wsRoot, "--config", cfgPath, "upstream", "list")
	if err != nil {
		t.Fatalf("unexpected error running upstream list: %v", err)
	}
	if !strings.Contains(outText, "No tracked upstream sources") {
		t.Fatalf("expected 'No tracked upstream sources', got:\n%s", outText)
	}

	// Initial list JSON output
	outJSON, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "upstream", "list", "--json")
	if err != nil {
		t.Fatalf("unexpected error running upstream list --json: %v", err)
	}
	var emptyList []model.UpstreamInfo
	if err := json.Unmarshal([]byte(outJSON), &emptyList); err != nil {
		t.Fatalf("expected valid JSON array, got: %s", outJSON)
	}
	if len(emptyList) != 0 {
		t.Fatalf("expected 0 upstreams in empty JSON output, got %d", len(emptyList))
	}

	// 2. Track an upstream skill
	repoDir := setupUpstreamGitRepoForCLI(t, tmpDir)
	createTestSkill(wsRoot, "skill-a", model.SourceSpec{
		Type: model.SourceTypeGit,
		URL:  repoDir,
		Path: "skills/skill-a",
	})

	// Run upstream list (text)
	outText, err = executeCommand("--root", wsRoot, "--config", cfgPath, "upstream", "list")
	if err != nil {
		t.Fatalf("upstream list failed: %v", err)
	}
	if !strings.Contains(outText, repoDir) || !strings.Contains(outText, "skill-a") {
		t.Fatalf("expected repoDir and skill-a in output, got:\n%s", outText)
	}

	// Run upstream list (json)
	outJSON, _, err = executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "upstream", "list", "--json")
	if err != nil {
		t.Fatalf("upstream list --json failed: %v", err)
	}
	var upList []model.UpstreamInfo
	if err := json.Unmarshal([]byte(outJSON), &upList); err != nil {
		t.Fatalf("expected valid JSON array, got: %s", outJSON)
	}
	if len(upList) != 1 || upList[0].URL != repoDir {
		t.Fatalf("expected 1 upstream with URL %s, got: %+v", repoDir, upList)
	}
}

func TestCLIUpstreamCheck(t *testing.T) {
	wsRoot, cfgPath, tmpDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoDir := setupUpstreamGitRepoForCLI(t, tmpDir)
	createTestSkill(wsRoot, "skill-a", model.SourceSpec{
		Type: model.SourceTypeGit,
		URL:  repoDir,
		Path: "skills/skill-a",
	})

	// Make a commit in the upstream repo
	_ = os.WriteFile(filepath.Join(repoDir, "skills", "skill-a", "SKILL.md"), []byte("---\nname: skill-a\ndescription: Skill A v2\n---\n# Skill A v2\n"), 0644)
	runGitInDir(t, repoDir, "add", ".")
	runGitInDir(t, repoDir, "commit", "-m", "Bump skill-a")

	// Run check --json
	outJSON, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "upstream", "check", repoDir, "--json")
	if err != nil {
		t.Fatalf("upstream check --json failed: %v", err)
	}

	var checkList []model.UpstreamInfo
	if err := json.Unmarshal([]byte(outJSON), &checkList); err != nil {
		t.Fatalf("expected valid JSON array from check, got: %s", outJSON)
	}
	if len(checkList) == 0 {
		t.Fatal("expected at least 1 upstream checked")
	}
	if checkList[0].Status != model.UpstreamUpdateAvailable {
		t.Fatalf("expected status %s, got %s", model.UpstreamUpdateAvailable, checkList[0].Status)
	}

	// Run check (text)
	outText, err := executeCommand("--root", wsRoot, "--config", cfgPath, "upstream", "check", repoDir)
	if err != nil {
		t.Fatalf("upstream check failed: %v", err)
	}
	if !strings.Contains(outText, repoDir) {
		t.Fatalf("expected repoDir in check output, got:\n%s", outText)
	}
}

func TestCLIUpstreamSkills(t *testing.T) {
	wsRoot, cfgPath, tmpDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoDir := setupUpstreamGitRepoForCLI(t, tmpDir)

	// Run upstream skills <repoDir> (text)
	outText, err := executeCommand("--root", wsRoot, "--config", cfgPath, "upstream", "skills", repoDir)
	if err != nil {
		t.Fatalf("upstream skills failed: %v", err)
	}
	if !strings.Contains(outText, "skill-a") || !strings.Contains(outText, "skill-b") {
		t.Fatalf("expected skill-a and skill-b in skills output, got:\n%s", outText)
	}

	// Run upstream skills <repoDir> --json
	outJSON, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "upstream", "skills", repoDir, "--json")
	if err != nil {
		t.Fatalf("upstream skills --json failed: %v", err)
	}

	var skills []struct {
		ID          string `json:"id"`
		Path        string `json:"path"`
		Description string `json:"description"`
		Installed   bool   `json:"installed"`
	}
	if err := json.Unmarshal([]byte(outJSON), &skills); err != nil {
		t.Fatalf("expected valid JSON array, got: %s", outJSON)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 discovered skills, got %d", len(skills))
	}
}

func TestCLIUpstreamSkillsEmptyAndPartialFailureJSON(t *testing.T) {
	ws, cfg, _, cleanup := setupTestEnvironment(t)
	defer cleanup()
	dir := t.TempDir()
	stdout, _, err := executeCommandWithStdout("--root", ws, "--config", cfg, "upstream", "skills", dir, "--json")
	if err != nil || strings.TrimSpace(stdout) != "[]" {
		t.Fatalf("empty must be []: %q %v", stdout, err)
	}
	for _, id := range []string{"valid", "broken"} {
		d := filepath.Join(dir, "skills", id)
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
		content := "broken"
		if id == "valid" {
			content = "---\nname: valid\ndescription: fixture\n---\n# Valid"
		}
		if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	stdout, _, err = executeCommandWithStdout("--root", ws, "--config", cfg, "upstream", "skills", dir, "--json")
	if err == nil {
		t.Fatal("partial failure should be nonzero")
	}
	var result struct {
		Skills []struct {
			ID string `json:"id"`
		} `json:"skills"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON %q %v", stdout, err)
	}
	if len(result.Skills) != 1 || result.Skills[0].ID != "valid" || !strings.Contains(result.Error, "broken") {
		t.Fatalf("missing successful result or failure %+v", result)
	}
}

func TestCLIUpstreamPull(t *testing.T) {
	wsRoot, cfgPath, tmpDir, cleanup := setupTestEnvironment(t)
	defer cleanup()

	repoDir := setupUpstreamGitRepoForCLI(t, tmpDir)

	// Pull specific skill-a with --json
	outJSON, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "upstream", "pull", repoDir, "skill-a", "--json")
	if err != nil {
		t.Fatalf("upstream pull skill-a --json failed: %v", err)
	}

	var res struct {
		Added []string `json:"added"`
	}
	if err := json.Unmarshal([]byte(outJSON), &res); err != nil {
		t.Fatalf("expected valid JSON result, got: %s", outJSON)
	}
	if len(res.Added) != 1 || res.Added[0] != "skill-a" {
		t.Fatalf("expected skill-a added, got %+v", res)
	}

	// Pull all remaining skills with --all
	outText, err := executeCommand("--root", wsRoot, "--config", cfgPath, "upstream", "pull", repoDir, "--all")
	if err != nil {
		t.Fatalf("upstream pull --all failed: %v", err)
	}
	if !strings.Contains(outText, "skill-b") && !strings.Contains(outText, "Successfully pulled") {
		t.Fatalf("expected successful pull message, got:\n%s", outText)
	}
}
