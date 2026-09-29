package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"asoul/internal/fsx"
)

var backupTimestampRegex = regexp.MustCompile(`^\d{14}$`)

// CreateConfigFileBackup creates a timestamped backup in .asoul/ directory in the same folder as targetFile.
// Format: <targetFileBaseName>.yyyyMMddHHmmss
// Version retention policy:
// - If maxVersions != nil && *maxVersions == 0: no backup is created, returns ("", nil).
// - If maxVersions == nil: backup is created and unlimited versions are retained.
// - If maxVersions != nil && *maxVersions > 0: backup is created, and older backups exceeding maxVersions are pruned.
func CreateConfigFileBackup(targetFile string, rawBytes []byte, maxVersions *int) (string, error) {
	if maxVersions != nil && *maxVersions == 0 {
		return "", nil
	}

	dir := filepath.Dir(targetFile)
	baseName := filepath.Base(targetFile)
	backupDir := filepath.Join(dir, ".asoul")

	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create backup directory %s: %w", backupDir, err)
	}

	// Generate timestamp yyyyMMddHHmmss, handling same-second collisions cleanly
	timestamp := time.Now().Format("20060102150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s", baseName, timestamp))
	for fsx.FileExists(backupPath) {
		time.Sleep(50 * time.Millisecond)
		timestamp = time.Now().Format("20060102150405")
		backupPath = filepath.Join(backupDir, fmt.Sprintf("%s.%s", baseName, timestamp))
	}

	perm := os.FileMode(0600)
	if info, err := os.Stat(targetFile); err == nil {
		perm = info.Mode().Perm()
	}
	if err := os.WriteFile(backupPath, rawBytes, perm); err != nil {
		return "", fmt.Errorf("failed to write backup file %s: %w", backupPath, err)
	}

	// Prune older backups if a positive limit is configured
	if maxVersions != nil && *maxVersions > 0 {
		_ = PruneOldBackups(backupDir, baseName, *maxVersions)
	}

	return backupPath, nil
}

// PruneOldBackups removes older backups of baseName in backupDir exceeding maxVersions.
func PruneOldBackups(backupDir, baseName string, maxVersions int) error {
	if maxVersions <= 0 {
		return nil
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return err
	}

	prefix := baseName + "."
	var backupFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) == len(prefix)+14 && name[:len(prefix)] == prefix {
			suffix := name[len(prefix):]
			if backupTimestampRegex.MatchString(suffix) {
				backupFiles = append(backupFiles, name)
			}
		}
	}

	if len(backupFiles) <= maxVersions {
		return nil
	}

	// Sort ascending (chronological order because yyyyMMddHHmmss is lexicographical)
	sort.Strings(backupFiles)

	toDelete := len(backupFiles) - maxVersions
	for i := 0; i < toDelete; i++ {
		_ = os.Remove(filepath.Join(backupDir, backupFiles[i]))
	}

	return nil
}
