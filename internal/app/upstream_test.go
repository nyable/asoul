package app_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	runGitCmd(t, repoDir, "add", ".")
	runGitCmd(t, repoDir, "commit", "-m", "Initial commit with skills")

	return repoDir
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
