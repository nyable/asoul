package hash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/hash"
)

func TestDeterministicHash(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-hash-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	skillDir := filepath.Join(tmpDir, "my-skill")
	_ = os.MkdirAll(skillDir, 0755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test content"), 0644)

	h1, err := hash.HashDir(skillDir)
	if err != nil {
		t.Fatalf("HashDir failed: %v", err)
	}

	if !strings.HasPrefix(h1, "h1:") {
		t.Fatalf("expected hash to start with h1:, got %s", h1)
	}

	// Repeated computation must be identical
	h2, err := hash.HashDir(skillDir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("expected deterministic hash, got %s != %s", h1, h2)
	}

	// Modification changes hash
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("modified content"), 0644)
	h3, err := hash.HashDir(skillDir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h3 {
		t.Fatalf("expected hash to change after file edit, but got identical: %s", h1)
	}
}
