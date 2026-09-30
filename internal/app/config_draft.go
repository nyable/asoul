package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"

	"asoul/internal/agent"
	"asoul/internal/fsx"
	"asoul/internal/jsonc"

	"github.com/gofrs/flock"
)

// EditAgentConfigDraft previews or commits an exact JSONC candidate. It checks
// the baseline under the same file lock used by field edits before backup/write.
func (s *Service) EditAgentConfigDraft(ctx context.Context, channel, cfgFile string, candidate []byte, expectedHash string, dryRun bool) (*AgentConfigEdit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var root map[string]any
	if err := jsonc.Unmarshal(candidate, &root); err != nil || root == nil {
		return nil, fmt.Errorf("config draft must be a JSON/JSONC object: %v", err)
	}
	path, err := s.resolveAgentConfigPath(channel, cfgFile)
	if err != nil {
		return nil, err
	}
	// Bind the lock and all reads/writes to the resolved target, not a mutable
	// symlink name. A later symlink change cannot redirect this transaction.
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	fileLock := flock.New(path + ".lock")
	ok, err := fileLock.TryLock()
	if err != nil || !ok {
		return nil, fmt.Errorf("cannot lock config draft target %s: %v", path, err)
	}
	defer fileLock.Unlock()
	doc, err := agent.LoadConfigDoc(path)
	if err != nil {
		return nil, err
	}
	originalHash := fmt.Sprintf("%x", sha256.Sum256(doc.Raw()))
	if expectedHash != "" && expectedHash != originalHash {
		return nil, fmt.Errorf("configuration changed since preview; review a new diff before saving")
	}
	result := &AgentConfigEdit{ConfigFile: path, OriginalHash: originalHash, Modified: !bytes.Equal(doc.Raw(), candidate)}
	if !result.Modified {
		return result, nil
	}
	result.DiffText = agent.UnifiedDiff(string(doc.Raw()), string(candidate))
	if dryRun {
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var maxVersions *int
	if s.configMgr != nil {
		cfg, err := s.configMgr.Load()
		if err != nil {
			return nil, err
		}
		maxVersions = cfg.GetBackupMaxVersions()
	}
	result.BackupFile, err = agent.CreateConfigFileBackup(path, doc.Raw(), maxVersions)
	if err != nil {
		return nil, err
	}
	if err := fsx.AtomicWriteFile(path, candidate, doc.Perm()); err != nil {
		return nil, err
	}
	return result, nil
}
