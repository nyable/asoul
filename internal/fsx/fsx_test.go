package fsx_test

import (
	"os"
	"path/filepath"
	"testing"

	"asoul/internal/fsx"
)

func TestPathNormalizationAndComparison(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// 1. SamePath between ~ and home dir
	p1 := "~/.config/asoul"
	p2 := filepath.Join(home, ".config", "asoul")
	if !fsx.SamePath(p1, p2) {
		t.Fatalf("expected %q and %q to be the same path", p1, p2)
	}

	// 2. CompactUser
	compacted := fsx.CompactUser(p2)
	if compacted != "~/.config/asoul" {
		t.Fatalf("expected ~/.config/asoul, got %q", compacted)
	}

	// 3. Relative path comparison with absolute
	cwd, _ := os.Getwd()
	rel := "subfolder"
	abs := filepath.Join(cwd, "subfolder")
	if !fsx.SamePath(rel, abs) {
		t.Fatalf("expected relative %q and absolute %q to be same path", rel, abs)
	}

	// 4. NormalizeWorkspacePath
	norm1 := fsx.NormalizeWorkspacePath(p1)
	norm2 := fsx.NormalizeWorkspacePath(p2)
	if norm1 != norm2 {
		t.Fatalf("expected %q == %q", norm1, norm2)
	}
}
