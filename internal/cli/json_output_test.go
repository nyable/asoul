package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/model"
)

func mustValidJSON(t *testing.T, outStr string) map[string]interface{} {
	t.Helper()
	trimmed := strings.TrimSpace(outStr)
	if trimmed == "" {
		t.Fatalf("expected JSON output, got empty string")
	}
	if trimmed == "null" {
		t.Fatalf("expected non-null JSON output, got: %s", trimmed)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		t.Fatalf("expected valid JSON object, got error %v, output: %s", err, trimmed)
	}
	return doc
}

func mustValidJSONArray(t *testing.T, outStr string) []interface{} {
	t.Helper()
	trimmed := strings.TrimSpace(outStr)
	if !json.Valid([]byte(trimmed)) {
		t.Fatalf("expected valid JSON array, got: %q", trimmed)
	}
	var arr []interface{}
	if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
		t.Fatalf("expected JSON array, got error %v, output: %s", err, trimmed)
	}
	return arr
}

func TestCLIEmptyResultsAreValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "ws")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(tmpDir, "config.jsonc")

	// Array-producing commands must emit [] (never null) when empty.
	for _, args := range [][]string{
		{"--root", wsRoot, "--config", cfgPath, "list", "--json"},
		{"--root", wsRoot, "--config", cfgPath, "status", "--json"},
		{"--root", wsRoot, "--config", cfgPath, "check", "--json"},
	} {
		outStr, _, err := executeCommandWithStdout(args...)
		if err != nil {
			t.Fatalf("command %v failed: %v (stdout=%s)", args, err, outStr)
		}
		trimmed := strings.TrimSpace(outStr)
		if !json.Valid([]byte(trimmed)) {
			t.Fatalf("command %v emitted invalid JSON: %q", args, trimmed)
		}
		if trimmed != "[]" {
			t.Fatalf("command %v expected empty array [], got %q", args, trimmed)
		}
	}

	// Object-producing commands must emit {} when empty.
	outGroups, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "group", "list", "--json")
	if err != nil {
		t.Fatalf("group list failed: %v (stdout=%s)", err, outGroups)
	}
	if trimmed := strings.TrimSpace(outGroups); trimmed != "{}" {
		t.Fatalf("group list expected empty object {}, got %q", trimmed)
	}

	// target list is never empty (it includes built-in presets), but must still
	// be a valid JSON object rather than null.
	outTargets, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "target", "list", "--json")
	if err != nil {
		t.Fatalf("target list failed: %v (stdout=%s)", err, outTargets)
	}
	if doc := mustValidJSON(t, outTargets); len(doc) == 0 {
		t.Fatalf("target list expected non-empty object, got %v", doc)
	}
}

func TestCLIDiffJSONEmptyIsValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "ws")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	content := "---\nname: up\ndescription: Test skill\n---\n# Content\n"
	srcDir := filepath.Join(tmpDir, "src", "up")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	createTestSkill(wsRoot, "up", model.SourceSpec{Type: model.SourceTypeLocal, Path: srcDir})
	cfgPath := filepath.Join(tmpDir, "config.jsonc")

	outStr, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "diff", "up", "--json")
	if err != nil {
		t.Fatalf("diff --json failed: %v (stdout=%s)", err, outStr)
	}
	doc := mustValidJSON(t, outStr)
	if diff, _ := doc["diff"].(string); diff != "" {
		t.Fatalf("expected empty diff, got %q", diff)
	}
	if doc["skillId"] != "up" {
		t.Fatalf("expected skillId up, got %v", doc["skillId"])
	}
}

func TestCLIAddJSONStructuredFailure(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	badDir := filepath.Join(t.TempDir(), "bad")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Missing required description field.
	if err := os.WriteFile(filepath.Join(badDir, "SKILL.md"), []byte("---\nname: bad\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	outStr, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "add", badDir, "--json")
	if err == nil {
		t.Fatal("expected non-zero exit for invalid add")
	}
	doc := mustValidJSON(t, outStr)
	if msg, _ := doc["error"].(string); msg == "" {
		t.Fatalf("expected structured error field, got: %v", doc)
	}
}

func TestCLIDoctorJSONExitCode(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Register a skill whose directory does not exist to force a diagnostic error.
	cat, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	cat.Skills["ghost"] = model.SkillRecord{Source: model.SourceSpec{Type: model.SourceTypeManaged}}
	if err := catalog.Save(wsRoot, cat); err != nil {
		t.Fatal(err)
	}

	outStr, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "doctor", "--json")
	if err == nil {
		t.Fatalf("expected non-zero exit when doctor reports errors, stdout=%s", outStr)
	}
	doc := mustValidJSON(t, outStr)
	if doc["hasError"] != true {
		t.Fatalf("expected hasError true in JSON, got: %v", doc)
	}
}

func TestCLIRemoveNonInteractiveRequiresConfirmation(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	createTestSkill(wsRoot, "skill-a", model.SourceSpec{Type: model.SourceTypeManaged})
	createTestSkill(wsRoot, "skill-b", model.SourceSpec{Type: model.SourceTypeManaged})

	outStr, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "--non-interactive", "remove", "skill-a", "skill-b", "--json")
	if err == nil {
		t.Fatal("expected non-zero exit for ambiguous non-interactive batch removal")
	}
	doc := mustValidJSON(t, outStr)
	if doc["error"] != "confirmation required" {
		t.Fatalf("expected confirmation required error, got: %v", doc)
	}

	cat, _ := catalog.Load(wsRoot)
	if _, ok := cat.Skills["skill-a"]; !ok {
		t.Fatal("skill-a should not have been removed")
	}
	if _, ok := cat.Skills["skill-b"]; !ok {
		t.Fatal("skill-b should not have been removed")
	}
}

func TestCLICheckJSONFailureExitCode(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	createTestSkill(wsRoot, "broken", model.SourceSpec{
		Type: model.SourceTypeLocal,
		Path: filepath.Join(t.TempDir(), "missing-source"),
	})

	outStr, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "check", "--json")
	if err == nil {
		t.Fatalf("expected non-zero exit when a skill check fails, stdout=%s", outStr)
	}
	arr := mustValidJSONArray(t, outStr)
	foundError := false
	for _, item := range arr {
		obj, _ := item.(map[string]interface{})
		if msg, _ := obj["error"].(string); msg != "" {
			foundError = true
		}
		if up, _ := obj["upstream"].(string); up == "unreachable" || up == "invalid" {
			foundError = true
		}
	}
	if !foundError {
		t.Fatalf("expected structured per-item failure in JSON, got: %s", outStr)
	}
}

func TestCLIUpstreamCheckJSONFailureExitCode(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	createTestSkill(wsRoot, "broken", model.SourceSpec{
		Type: model.SourceTypeLocal,
		Path: filepath.Join(t.TempDir(), "missing-source"),
	})

	outStr, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "upstream", "check", "--json")
	if err == nil {
		t.Fatalf("expected non-zero exit when an upstream check fails, stdout=%s", outStr)
	}
	arr := mustValidJSONArray(t, outStr)
	if len(arr) == 0 {
		t.Fatalf("expected upstream entries in JSON, got: %s", outStr)
	}
}

func TestCLIGroupAddJSONFailureExitCode(t *testing.T) {
	wsRoot, cfgPath, _, cleanup := setupTestEnvironment(t)
	defer cleanup()

	outStr, _, err := executeCommandWithStdout("--root", wsRoot, "--config", cfgPath, "group", "add", "missing-group", "skill-a", "--json")
	if err == nil {
		t.Fatal("expected non-zero exit when adding to a missing group")
	}
	doc := mustValidJSON(t, outStr)
	failed, _ := doc["failed"].(map[string]interface{})
	if len(failed) == 0 {
		t.Fatalf("expected structured failed map in JSON, got: %s", outStr)
	}
}
