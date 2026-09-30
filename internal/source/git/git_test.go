package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/cache"
	"asoul/internal/gitx"
	"asoul/internal/model"
	"asoul/internal/skill"
)

func TestDiscoverNestedSkillsAndReportInvalidFiles(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	run("init")
	for _, id := range []string{"vue", "vite", "pinia"} {
		dir := filepath.Join(repo, "skills", id)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), skill.GenerateSkillMD(id, "fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bad := filepath.Join(repo, "skills", "broken")
	if err := os.MkdirAll(bad, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("invalid frontmatter"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "fixture")
	cm, err := cache.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := New(gitx.NewClient(), cm)
	found, err := src.DiscoverSkills(context.Background(), repo, "", model.DefaultScanConfig())
	if len(found) != 3 || err == nil || !strings.Contains(err.Error(), "skills/broken/SKILL.md") {
		t.Fatalf("%+v %v", found, err)
	}
	for _, s := range found {
		if s.Path != "skills/"+s.ID {
			t.Fatalf("nested path lost %+v", s)
		}
	}
	// A cache-only refresh must not resolve a new remote branch by fetching.
	run("branch", "new-remote-only")
	if _, err := src.DiscoverSkillsCached(context.Background(), repo, "new-remote-only", model.DefaultScanConfig()); err == nil {
		t.Fatal("cache-only discovery fetched a missing ref")
	}
}

func TestGitSource_ResolveBatch(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "asoul-test-gitsource-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cacheMgr, err := cache.NewManager(filepath.Join(tmpDir, "cache"))
	if err != nil {
		t.Fatal(err)
	}

	client := gitx.NewClient()
	gitSrc := New(client, cacheMgr)

	remoteRepo := filepath.Join(tmpDir, "remote")
	_ = os.MkdirAll(remoteRepo, 0755)

	runCmd := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %s %v failed: %v, output: %s", name, args, err, string(out))
		}
	}

	runCmd(remoteRepo, "git", "init")
	runCmd(remoteRepo, "git", "config", "user.name", "testuser")
	runCmd(remoteRepo, "git", "config", "user.email", "test@test.local")

	dir1 := filepath.Join(remoteRepo, "skills", "skill-x")
	dir2 := filepath.Join(remoteRepo, "skills", "skill-y")
	_ = os.MkdirAll(dir1, 0755)
	_ = os.MkdirAll(dir2, 0755)

	_ = os.WriteFile(filepath.Join(dir1, "SKILL.md"), skill.GenerateSkillMD("skill-x", "Description X"), 0644)
	_ = os.WriteFile(filepath.Join(dir2, "SKILL.md"), skill.GenerateSkillMD("skill-y", "Description Y"), 0644)

	runCmd(remoteRepo, "git", "add", ".")
	runCmd(remoteRepo, "git", "commit", "-m", "init commit")

	spec := model.SourceSpec{
		Type: model.SourceTypeGit,
		URL:  "file://" + remoteRepo,
		Ref:  "HEAD",
	}

	candidates, failures, err := gitSrc.ResolveBatch(ctx, spec, []string{"skills/skill-x", "skills/skill-y"}, true)
	if err != nil {
		t.Fatalf("ResolveBatch failed: %v", err)
	}
	defer func() {
		for _, c := range candidates {
			c.Cleanup()
		}
	}()

	if len(failures) > 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}

	foundX := false
	foundY := false
	for _, c := range candidates {
		if c.SkillID == "skill-x" {
			foundX = true
			if c.Subpath != "skills/skill-x" {
				t.Errorf("expected Subpath 'skills/skill-x', got %q", c.Subpath)
			}
		}
		if c.SkillID == "skill-y" {
			foundY = true
			if c.Subpath != "skills/skill-y" {
				t.Errorf("expected Subpath 'skills/skill-y', got %q", c.Subpath)
			}
		}
	}

	if !foundX || !foundY {
		t.Fatalf("expected to find both skill-x and skill-y, foundX=%v, foundY=%v", foundX, foundY)
	}
}

// TestDiscoverSkillsRespectsScanScope verifies that only directories inside the configured
// scope are parsed, so placeholder templates outside skills/ no longer fail a scan.
func TestDiscoverSkillsRespectsScanScope(t *testing.T) {
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	run("init")
	write("skills/real/SKILL.md", "---\nname: real\ndescription: fixture\n---\n# real")
	write("skills/demo/SKILL.md", "---\nname: demo\ndescription: fixture\n---\n# demo")
	write("skills/broken/SKILL.md", "invalid frontmatter")
	write("template/SKILL.md", "---\nname: template-skill\ndescription: placeholder\n---\n# template")
	run("add", ".")
	run("commit", "-m", "fixture")

	cm, err := cache.NewManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	src := New(gitx.NewClient(), cm)
	ctx := context.Background()

	// Default scope scans skills/ only: the placeholder template is ignored while the
	// invalid skill inside skills/ still surfaces as a partial failure.
	found, err := src.DiscoverSkills(ctx, repo, "", model.DefaultScanConfig())
	if len(found) != 2 {
		t.Fatalf("default scope should find 2 skills, got %+v", found)
	}
	if err == nil || !strings.Contains(err.Error(), "skills/broken/SKILL.md") {
		t.Fatalf("default scope should report the broken skill, got %v", err)
	}
	if strings.Contains(err.Error(), "template/SKILL.md") {
		t.Fatalf("template outside skills/ must not be reported: %v", err)
	}

	// Whole-source scope parses every SKILL.md, including the placeholder template.
	whole, err := src.DiscoverSkillsCached(ctx, repo, "", model.ScanConfig{Roots: []string{"."}})
	if len(whole) != 2 {
		t.Fatalf("whole scope should still find 2 valid skills, got %+v", whole)
	}
	if err == nil || !strings.Contains(err.Error(), "template/SKILL.md") {
		t.Fatalf("whole scope should report the template, got %v", err)
	}

	// Excluding a directory removes its failures without affecting other scopes.
	excluded, err := src.DiscoverSkillsCached(ctx, repo, "", model.ScanConfig{Roots: []string{"skills"}, Exclude: []string{"skills/broken"}})
	if len(excluded) != 2 || err != nil {
		t.Fatalf("excluded scope should be clean, got %+v err=%v", excluded, err)
	}
}
