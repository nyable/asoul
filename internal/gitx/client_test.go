package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClient_ArchiveExtractPaths(t *testing.T) {
	ctx := context.Background()
	client := NewClient()

	tmpDir, err := os.MkdirTemp("", "asoul-test-gitx-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := filepath.Join(tmpDir, "repo")
	_ = os.MkdirAll(repoDir, 0755)

	workDir := filepath.Join(tmpDir, "work")
	_ = os.MkdirAll(workDir, 0755)

	runCmd := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %s %v failed: %v, output: %s", name, args, err, string(out))
		}
	}

	runCmd(workDir, "git", "init")
	runCmd(workDir, "git", "config", "user.name", "testuser")
	runCmd(workDir, "git", "config", "user.email", "test@test.local")

	// Create subdirectories
	dirA := filepath.Join(workDir, "skills", "skill-a")
	dirB := filepath.Join(workDir, "skills", "skill-b")
	_ = os.MkdirAll(dirA, 0755)
	_ = os.MkdirAll(dirB, 0755)

	_ = os.WriteFile(filepath.Join(dirA, "a.txt"), []byte("content A"), 0644)
	_ = os.WriteFile(filepath.Join(dirB, "b.txt"), []byte("content B"), 0644)

	runCmd(workDir, "git", "add", ".")
	runCmd(workDir, "git", "commit", "-m", "init")

	// Clone to bare repo as asoul does
	runCmd(tmpDir, "git", "clone", "--bare", workDir, repoDir)

	destDir := filepath.Join(tmpDir, "extracted")
	err = client.ArchiveExtractPaths(ctx, repoDir, "HEAD", []string{"skills/skill-a", "skills/skill-b"}, destDir)
	if err != nil {
		t.Fatalf("ArchiveExtractPaths failed: %v", err)
	}

	contentA, err := os.ReadFile(filepath.Join(destDir, "skills", "skill-a", "a.txt"))
	if err != nil || string(contentA) != "content A" {
		t.Fatalf("expected content A, got err: %v, content: %q", err, string(contentA))
	}

	contentB, err := os.ReadFile(filepath.Join(destDir, "skills", "skill-b", "b.txt"))
	if err != nil || string(contentB) != "content B" {
		t.Fatalf("expected content B, got err: %v, content: %q", err, string(contentB))
	}
}
