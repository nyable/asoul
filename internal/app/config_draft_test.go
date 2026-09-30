package app_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gofrs/flock"
)

func TestConfigDraftPreviewCommitAndChangedBaseline(t *testing.T) {
	svc, tmp, cleanup := setupTestService(t)
	defer cleanup()
	path := filepath.Join(tmp, "config draft.jsonc")
	original := []byte("{\r\n // keep comment\r\n \"value\":1\r\n}\r\n")
	candidate := []byte(strings.Replace(string(original), "1", "2", 1))
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.EditAgentConfigDraft(context.Background(), "opencode", path, candidate, "", true)
	if err != nil || !preview.Modified || preview.OriginalHash == "" {
		t.Fatalf("%+v %v", preview, err)
	}
	if data, _ := os.ReadFile(path); string(data) != string(original) {
		t.Fatal("preview wrote original")
	}
	if _, err := svc.EditAgentConfigDraft(context.Background(), "opencode", path, []byte("{"), preview.OriginalHash, false); err == nil {
		t.Fatal("invalid JSON committed")
	}
	result, err := svc.EditAgentConfigDraft(context.Background(), "opencode", path, candidate, preview.OriginalHash, false)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(result.BackupFile); string(data) != string(original) {
		t.Fatal("backup mismatch")
	}
	if data, _ := os.ReadFile(path); string(data) != string(candidate) {
		t.Fatal("draft or CRLF changed")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0600 {
			t.Fatal("permissions widened")
		}
	}
	if _, err := svc.EditAgentConfigDraft(context.Background(), "opencode", path, original, preview.OriginalHash, false); err == nil {
		t.Fatal("stale draft committed")
	}
	if _, err := svc.ApplyAgentConfigValue(context.Background(), "opencode", path, []string{"value"}, 3, false, preview.OriginalHash); err == nil {
		t.Fatal("stale field committed")
	}
	unchanged, err := svc.EditAgentConfigDraft(context.Background(), "opencode", path, candidate, "", false)
	if err != nil || unchanged.Modified || unchanged.BackupFile != "" {
		t.Fatalf("unchanged %+v %v", unchanged, err)
	}
}

func TestConfigDraftHonorsConcurrentFileLock(t *testing.T) {
	svc, tmp, cleanup := setupTestService(t)
	defer cleanup()
	path := filepath.Join(tmp, "locked.jsonc")
	if err := os.WriteFile(path, []byte(`{"value":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(path + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	if _, err := svc.EditAgentConfigDraft(context.Background(), "opencode", path, []byte(`{"value":2}`), "", false); err == nil {
		t.Fatal("ignored concurrent lock")
	}
	if data, _ := os.ReadFile(path); string(data) != `{"value":1}` {
		t.Fatal("locked file changed")
	}
}
