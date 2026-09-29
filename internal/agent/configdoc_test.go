package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDocPreservesCommentsAndOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.jsonc")
	raw := `{
  // gateway config
  "provider": {
    "custom": {
      // model docs
      "models": {
        "zeta": {
          "name": "Zeta", // keep me
          "limit": { "context": 100 }
        },
        "alpha": {}
      }
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}

	doc, err := LoadConfigDoc(path)
	if err != nil {
		t.Fatalf("LoadConfigDoc: %v", err)
	}

	merged := map[string]any{
		"name":     "Zeta",
		"limit":    map[string]any{"context": float64(100), "output": float64(50)},
		"coverage": "full",
	}
	if err := doc.Apply(merged, "provider", "custom", "models", "zeta"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	out := string(doc.Bytes())
	for _, comment := range []string{"// gateway config", "// model docs", "// keep me"} {
		if !strings.Contains(out, comment) {
			t.Fatalf("expected comment %q to be preserved, got:\n%s", comment, out)
		}
	}

	if !strings.Contains(out, `"output": 50`) {
		t.Fatalf("expected nested output field added, got:\n%s", out)
	}

	// Original key order must be preserved: zeta stays before alpha, and the
	// new "coverage" key is appended after the existing keys.
	zetaIdx := strings.Index(out, `"zeta"`)
	alphaIdx := strings.Index(out, `"alpha"`)
	if zetaIdx < 0 || alphaIdx < 0 || zetaIdx > alphaIdx {
		t.Fatalf("expected original provider/model order preserved, got:\n%s", out)
	}
	nameIdx := strings.Index(out, `"name"`)
	limitIdx := strings.Index(out, `"limit"`)
	coverageIdx := strings.Index(out, `"coverage"`)
	if !(nameIdx < limitIdx && limitIdx < coverageIdx) {
		t.Fatalf("expected new key appended after existing keys, got:\n%s", out)
	}
}

func TestConfigDocRemovesKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	raw := `{
  "provider": {
    "custom": {
      "models": {
        "m": {
          "name": "M",
          "secret": "remove-me",
          "limit": { "context": 1 }
        }
      }
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadConfigDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	merged := map[string]any{
		"name":  "M",
		"limit": map[string]any{"context": float64(1)},
	}
	if err := doc.Apply(merged, "provider", "custom", "models", "m"); err != nil {
		t.Fatal(err)
	}
	out := string(doc.Bytes())
	if strings.Contains(out, "secret") {
		t.Fatalf("expected removed key to be gone, got:\n%s", out)
	}
	if !strings.Contains(out, `"limit"`) || !strings.Contains(out, `"name"`) {
		t.Fatalf("expected remaining keys, got:\n%s", out)
	}
}

func TestConfigDocAppendsNestedObjectWithIndent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	raw := `{
  "provider": {
    "custom": {
      "models": {
        "m": {
          "name": "M"
        }
      }
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadConfigDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	merged := map[string]any{
		"name":    "M",
		"options": map[string]any{"reasoningEffort": "high"},
	}
	if err := doc.Apply(merged, "provider", "custom", "models", "m"); err != nil {
		t.Fatal(err)
	}
	out := string(doc.Bytes())
	expected := "\"options\": {\n            \"reasoningEffort\": \"high\"\n          }"
	if !strings.Contains(out, expected) {
		t.Fatalf("expected nested object appended with matching indent, got:\n%s", out)
	}
}

func TestConfigDocReplacesNonObjectModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	raw := `{
  "provider": {
    "custom": {
      "models": {
        "m": null
      }
    }
  }
}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadConfigDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Apply(map[string]any{"name": "M"}, "provider", "custom", "models", "m"); err != nil {
		t.Fatal(err)
	}
	out := string(doc.Bytes())
	if !strings.Contains(out, `"name": "M"`) {
		t.Fatalf("expected null model replaced by object, got:\n%s", out)
	}
}
