package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"asoul/internal/cache"
	"asoul/internal/gitx"
	"asoul/internal/model"
	"asoul/internal/skill"
)

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
