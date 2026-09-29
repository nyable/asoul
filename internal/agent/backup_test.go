package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateConfigFileBackup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "asoul-backup-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	cfgFile := filepath.Join(tempDir, "opencode.jsonc")
	content := []byte(`{"test": true}`)
	if err := os.WriteFile(cfgFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Test maxVersions == 0 (no backup created)
	zero := 0
	bPath0, err := CreateConfigFileBackup(cfgFile, content, &zero)
	if err != nil {
		t.Fatalf("unexpected error with maxVersions=0: %v", err)
	}
	if bPath0 != "" {
		t.Fatalf("expected empty backup path for maxVersions=0, got %s", bPath0)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".asoul")); !os.IsNotExist(err) {
		t.Errorf(".asoul directory should not be created if maxVersions=0")
	}

	// 2. Test normal backup creation (maxVersions == nil, unlimited)
	bPath1, err := CreateConfigFileBackup(cfgFile, content, nil)
	if err != nil {
		t.Fatalf("failed to create backup: %v", err)
	}
	if bPath1 == "" {
		t.Fatalf("expected non-empty backup path")
	}

	// Verify directory is .asoul in same dir
	expectedDir := filepath.Join(tempDir, ".asoul")
	if filepath.Dir(bPath1) != expectedDir {
		t.Errorf("expected backup in %s, got %s", expectedDir, filepath.Dir(bPath1))
	}

	// Verify filename format: opencode.jsonc.yyyyMMddHHmmss (14 digits)
	base := filepath.Base(bPath1)
	prefix := "opencode.jsonc."
	if len(base) != len(prefix)+14 || base[:len(prefix)] != prefix {
		t.Errorf("backup filename format incorrect: %s", base)
	}
	suffix := base[len(prefix):]
	if !backupTimestampRegex.MatchString(suffix) {
		t.Errorf("timestamp suffix not 14 digits: %s", suffix)
	}

	// 3. Test version retention pruning
	// Manually create some older dummy backups in .asoul
	oldBackups := []string{
		"opencode.jsonc.20250101000000",
		"opencode.jsonc.20250101000001",
		"opencode.jsonc.20250101000002",
		"other_file.json.20250101000000", // should NOT be pruned when pruning opencode.jsonc
	}
	for _, ob := range oldBackups {
		_ = os.WriteFile(filepath.Join(expectedDir, ob), []byte("old"), 0644)
	}

	// Prune to max 2 versions of opencode.jsonc
	two := 2
	err = PruneOldBackups(expectedDir, "opencode.jsonc", two)
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	entries, _ := os.ReadDir(expectedDir)
	var remainingOpencode []string
	hasOther := false
	for _, e := range entries {
		if e.Name() == "other_file.json.20250101000000" {
			hasOther = true
		}
		if len(e.Name()) > len(prefix) && e.Name()[:len(prefix)] == prefix {
			remainingOpencode = append(remainingOpencode, e.Name())
		}
	}

	if len(remainingOpencode) != 2 {
		t.Errorf("expected exactly 2 remaining opencode backups, got %d: %v", len(remainingOpencode), remainingOpencode)
	}
	if !hasOther {
		t.Errorf("other file was wrongly pruned!")
	}

	// The two remaining should be the newest ones: bPath1 (current timestamp) and 20250101000002
	if _, err := os.Stat(filepath.Join(expectedDir, "opencode.jsonc.20250101000000")); !os.IsNotExist(err) {
		t.Errorf("oldest backup 20250101000000 should have been removed")
	}
	if _, err := os.Stat(filepath.Join(expectedDir, "opencode.jsonc.20250101000001")); !os.IsNotExist(err) {
		t.Errorf("old backup 20250101000001 should have been removed")
	}
}
