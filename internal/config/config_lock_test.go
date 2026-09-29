package config_test

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"asoul/internal/config"
	"asoul/internal/model"
)

func TestConcurrentTargetWritesDoNotLoseRecords(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := config.NewManager(filepath.Join(tmpDir, "config.jsonc"))
	if err != nil {
		t.Fatal(err)
	}

	const n = 32
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "target-" + string(rune('a'+i%26)) + "-" + strconv.Itoa(i)
			if err := mgr.AddTarget(name, model.TargetConfig{Type: model.TargetTypeAgent}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent AddTarget failed: %v", err)
	}

	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != n {
		t.Fatalf("expected %d targets after concurrent writes, got %d (records lost)", n, len(cfg.Targets))
	}
}

func TestConcurrentWorkspaceWritesDoNotLoseRecords(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := config.NewManager(filepath.Join(tmpDir, "config.jsonc"))
	if err != nil {
		t.Fatal(err)
	}

	// Seed an existing config so Load returns a stable file.
	if err := mgr.AddWorkspace(filepath.Join(tmpDir, "ws-0")); err != nil {
		t.Fatal(err)
	}
	seeded, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	base := len(seeded.Workspaces)

	const n = 24
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := mgr.AddWorkspace(filepath.Join(tmpDir, "ws-"+strconv.Itoa(i+1))); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent AddWorkspace failed: %v", err)
	}

	cfg, err := mgr.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspaces) != base+n {
		t.Fatalf("expected %d workspaces after concurrent writes, got %d (records lost)", base+n, len(cfg.Workspaces))
	}
}

func TestSavePreservesPrivatePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.jsonc")

	// A pre-existing private config must not be widened by a write.
	if err := os.WriteFile(cfgPath, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(cfgPath, 0600); err != nil {
		t.Fatal(err)
	}

	mgr, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddTarget("open", model.TargetConfig{Type: model.TargetTypeAgent}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("expected config permission 0600 to be preserved, got %#o", got)
	}
}

func TestSaveCreatesWithDefaultPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.jsonc")

	mgr, err := config.NewManager(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddTarget("open", model.TargetConfig{Type: model.TargetTypeAgent}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0644 {
		t.Fatalf("expected new config permission 0644, got %#o", got)
	}
}
