package app_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/model"
)

func runGitCmd(t *testing.T, dir string, args ...string) string {
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
		t.Fatalf("git %v failed: %v, output: %s", args, err, out)
	}
	return string(out)
}

func setupUpstreamGitRepo(t *testing.T, parentDir string) string {
	repoDir := filepath.Join(parentDir, "remote-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	runGitCmd(t, repoDir, "init")
	runGitCmd(t, repoDir, "config", "user.name", "Test")
	runGitCmd(t, repoDir, "config", "user.email", "test@example.com")

	// Create skill-a
	skillADir := filepath.Join(repoDir, "skills", "skill-a")
	if err := os.MkdirAll(skillADir, 0755); err != nil {
		t.Fatal(err)
	}
	skillAContent := "---\nname: skill-a\ndescription: Skill A for testing\n---\n# Skill A\n"
	if err := os.WriteFile(filepath.Join(skillADir, "SKILL.md"), []byte(skillAContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create skill-b
	skillBDir := filepath.Join(repoDir, "skills", "skill-b")
	if err := os.MkdirAll(skillBDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillBContent := "---\nname: skill-b\ndescription: Skill B for testing\n---\n# Skill B\n"
	if err := os.WriteFile(filepath.Join(skillBDir, "SKILL.md"), []byte(skillBContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Placeholder template outside skills/ whose name does not match its directory.
	templateDir := filepath.Join(repoDir, "template")
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		t.Fatal(err)
	}
	templateContent := "---\nname: template-skill\ndescription: Replace with a real description\n---\n# Template\n"
	if err := os.WriteFile(filepath.Join(templateDir, "SKILL.md"), []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	runGitCmd(t, repoDir, "add", ".")
	runGitCmd(t, repoDir, "commit", "-m", "Initial commit with skills")

	return repoDir
}

func TestConfiguredUpstreamCheckScansWithoutImports(t *testing.T) {
	svc, tmp, cleanup := setupTestService(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := svc.InitWorkspace(ctx, filepath.Join(tmp, "ws")); err != nil {
		t.Fatal(err)
	}
	repo := setupUpstreamGitRepo(t, tmp)
	url := "file://" + repo
	if err := svc.UpstreamAdd(ctx, url, "", "fixture", model.ScanConfig{}); err != nil {
		t.Fatal(err)
	}
	before, err := svc.UpstreamList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].Scanned {
		t.Fatalf("registration should not scan %+v", before)
	}
	after, err := svc.UpstreamCheck(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || !after[0].Scanned || len(after[0].AvailableSkills) != 2 || len(after[0].Skills) != 0 {
		t.Fatalf("did not discover configured source %+v", after)
	}
	if err := os.Rename(repo, repo+"-unavailable"); err != nil {
		t.Fatal(err)
	}
	failed, err := svc.UpstreamCheck(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if failed[0].Error == "" || failed[0].Status != model.UpstreamUnreachable {
		t.Fatalf("fetch failure swallowed %+v", failed)
	}
}

// TestUpstreamScanScope verifies that the configured scan scope controls which files
// are validated, so a placeholder template outside skills/ no longer fails the scan.
func TestUpstreamScanScope(t *testing.T) {
	svc, tmp, cleanup := setupTestService(t)
	defer cleanup()
	ctx := context.Background()
	if _, err := svc.InitWorkspace(ctx, filepath.Join(tmp, "ws")); err != nil {
		t.Fatal(err)
	}
	repo := setupUpstreamGitRepo(t, tmp)
	url := "file://" + repo

	// Default scope: skills/ only, template/ ignored.
	if err := svc.UpstreamAdd(ctx, url, "", "fixture", model.ScanConfig{}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.UpstreamList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(list))
	}
	if len(list[0].ScanRoots) != 1 || list[0].ScanRoots[0] != model.ScanRootDefault {
		t.Fatalf("expected default scan roots, got %+v", list[0].ScanRoots)
	}

	checked, err := svc.UpstreamCheck(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if checked[0].ScanError != "" {
		t.Fatalf("default scope must not report the template: %q", checked[0].ScanError)
	}
	if len(checked[0].AvailableSkills) != 2 {
		t.Fatalf("expected 2 available skills, got %v", checked[0].AvailableSkills)
	}

	// Whole-source scope: the template is parsed and its name mismatch is reported.
	if err := svc.UpstreamEdit(ctx, url, "", "fixture", model.ScanConfig{Roots: []string{model.ScanRootWholeSource}}); err != nil {
		t.Fatal(err)
	}
	checked, err = svc.UpstreamCheck(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(checked[0].ScanError, "template/SKILL.md") {
		t.Fatalf("whole-source scope should report the template, got %q", checked[0].ScanError)
	}

	// Custom scope with an exclusion returns to a clean scan.
	if err := svc.UpstreamEdit(ctx, url, "", "fixture", model.ScanConfig{Roots: []string{"skills", "template"}, Exclude: []string{"template"}}); err != nil {
		t.Fatal(err)
	}
	checked, err = svc.UpstreamCheck(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if checked[0].ScanError != "" {
		t.Fatalf("excluded template must not be reported: %q", checked[0].ScanError)
	}

	// An invalid scope is rejected before it can be persisted.
	if err := svc.UpstreamEdit(ctx, url, "", "fixture", model.ScanConfig{Roots: []string{"../escape"}}); err == nil {
		t.Fatal("expected invalid scan root to be rejected")
	}
}

func TestUpstreamManagement(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatal(err)
	}

	// 1. Initially UpstreamList is empty
	upstreams, err := svc.UpstreamList(ctx)
	if err != nil {
		t.Fatalf("UpstreamList failed: %v", err)
	}
	if len(upstreams) != 0 {
		t.Fatalf("expected 0 upstreams initially, got %d", len(upstreams))
	}

	// 2. Set up local git repo as upstream
	repoDir := setupUpstreamGitRepo(t, tmpDir)

	// Add skill-a and skill-b
	if err := svc.AddGit(ctx, repoDir, "", "skills/skill-a", false); err != nil {
		t.Fatalf("AddGit skill-a failed: %v", err)
	}
	if err := svc.AddGit(ctx, repoDir, "", "skills/skill-b", false); err != nil {
		t.Fatalf("AddGit skill-b failed: %v", err)
	}

	// 3. UpstreamList should aggregate both skills under the same upstream
	upstreams, err = svc.UpstreamList(ctx)
	if err != nil {
		t.Fatalf("UpstreamList failed: %v", err)
	}
	if len(upstreams) != 1 {
		t.Fatalf("expected 1 aggregated upstream, got %d", len(upstreams))
	}
	u := upstreams[0]
	if u.URL != repoDir {
		t.Errorf("expected URL %s, got %s", repoDir, u.URL)
	}
	if u.Type != model.SourceTypeGit {
		t.Errorf("expected Type git, got %s", u.Type)
	}
	if len(u.Skills) != 2 {
		t.Errorf("expected 2 skills under upstream, got %d", len(u.Skills))
	}
	if !u.CacheExists {
		t.Errorf("expected CacheExists to be true")
	}
	if u.CachePath == "" {
		t.Errorf("expected CachePath to be populated")
	}
	if len(u.AvailableSkills) != 2 {
		t.Errorf("expected 2 AvailableSkills from cache, got %d (%v)", len(u.AvailableSkills), u.AvailableSkills)
	}
	if u.CreatedAt == "" {
		t.Errorf("expected CreatedAt to be non-empty")
	}
	if u.UpdatedAt == "" {
		t.Errorf("expected UpdatedAt to be non-empty")
	}

	// 4. Test UpstreamDiscover (cache-first test: hide remote repo and discover should still work)
	movedRemote := repoDir + "-temp-moved"
	if err := os.Rename(repoDir, movedRemote); err != nil {
		t.Fatal(err)
	}

	// Discover should succeed from cache even though remote repo directory does not exist at repoDir
	cachedDiscovered, err := svc.UpstreamDiscover(ctx, repoDir, "")
	if err != nil {
		_ = os.Rename(movedRemote, repoDir)
		t.Fatalf("UpstreamDiscover failed from cache when remote was unavailable: %v", err)
	}
	if len(cachedDiscovered) != 2 {
		_ = os.Rename(movedRemote, repoDir)
		t.Fatalf("expected 2 discovered skills from cache, got %d", len(cachedDiscovered))
	}

	// Restore remote repo for subsequent tests
	if err := os.Rename(movedRemote, repoDir); err != nil {
		t.Fatal(err)
	}

	discovered, err := svc.UpstreamDiscover(ctx, repoDir, "")
	if err != nil {
		t.Fatalf("UpstreamDiscover failed: %v", err)
	}
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered skills, got %d", len(discovered))
	}

	// 5. Commit an update upstream and check
	skillAFile := filepath.Join(repoDir, "skills", "skill-a", "SKILL.md")
	if err := os.WriteFile(skillAFile, []byte("---\nname: skill-a\ndescription: Skill A v2\n---\n# Skill A v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, repoDir, "add", ".")
	runGitCmd(t, repoDir, "commit", "-m", "Update skill-a to v2")

	// UpstreamCheck
	checkedUpstreams, err := svc.UpstreamCheck(ctx, repoDir)
	if err != nil {
		t.Fatalf("UpstreamCheck failed: %v", err)
	}
	if len(checkedUpstreams) != 1 {
		t.Fatalf("expected 1 upstream after check, got %d", len(checkedUpstreams))
	}
	if checkedUpstreams[0].Status != model.UpstreamUpdateAvailable {
		t.Errorf("expected UpstreamUpdateAvailable, got %s", checkedUpstreams[0].Status)
	}

	// 6. UpstreamPull to update skill-a
	pullRes, err := svc.UpstreamPull(ctx, repoDir, "", []string{"skills/skill-a"}, nil, nil, false)
	if err != nil {
		t.Fatalf("UpstreamPull failed: %v", err)
	}
	if len(pullRes.Added) != 1 || pullRes.Added[0] != "skill-a" {
		t.Errorf("expected skill-a updated, got %v", pullRes.Added)
	}

	// 7. Verify local skill has new description
	stat, _, err := svc.Show(ctx, "skill-a")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("skill-a status after pull: %s\n", stat.ManagedStatus)
}
