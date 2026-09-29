package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/cli"
	"asoul/internal/model"
)

func TestOutputPrintDiff(t *testing.T) {
	out := cli.NewOutput()
	var buf bytes.Buffer
	out.SetWriters(&buf, &buf)

	rawDiff := `diff --git a/SKILL.md b/SKILL.md
--- a/SKILL.md
+++ b/SKILL.md
@@ -1 +1 @@
-old
+new
`
	out.PrintDiff(rawDiff)
	output := buf.String()

	if !strings.Contains(output, "SKILL.md") {
		t.Fatalf("expected output to contain SKILL.md, got:\n%s", output)
	}
	if !strings.Contains(output, "+new") || !strings.Contains(output, "-old") {
		t.Fatalf("expected output to contain diff lines, got:\n%s", output)
	}
	if !strings.Contains(output, "变更概览") && !strings.Contains(output, "Diff Summary") {
		t.Fatalf("expected output to contain diff summary header, got:\n%s", output)
	}
}

func TestCLIDiffCommandJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asoul-diff-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wsRoot := filepath.Join(tmpDir, "ws")
	if err := catalog.InitWorkspace(wsRoot); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(wsRoot, "skills", "test-skill")
	_ = os.MkdirAll(skillDir, 0755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Content\n"), 0644)

	cat, _ := catalog.Load(wsRoot)
	cat.Skills["test-skill"] = model.SkillRecord{
		Source: model.SourceSpec{Type: model.SourceTypeManaged},
	}
	_ = catalog.Save(wsRoot, cat)

	cfgPath := filepath.Join(tmpDir, "config.jsonc")

	// Test diff for a managed skill with no upstream source
	outStr, err := executeCommand("diff", "test-skill", "--root", wsRoot, "--config", cfgPath, "--json")
	if err != nil {
		// Managed skill without upstream source returns error, which is expected
		if !strings.Contains(err.Error(), "upstream") && !strings.Contains(outStr, "upstream") {
			t.Fatalf("expected upstream error or message, got: err=%v, out=%s", err, outStr)
		}
	}
	_ = json.Valid([]byte(outStr))
}
