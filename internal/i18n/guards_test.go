package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	return filepath.Clean(filepath.Join("..", ".."))
}

func eachGoFile(t *testing.T, root string, fn func(path string, fset *token.FileSet, file *ast.File)) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Errorf("parse %s: %v", path, perr)
			return nil
		}
		fn(path, fset, file)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func hasCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
			return true
		case r >= 0x3040 && r <= 0x30FF: // Hiragana / Katakana
			return true
		case r >= 0xAC00 && r <= 0xD7AF: // Hangul syllables
			return true
		}
	}
	return false
}

// TestNoHardcodedCJKOutsideI18n enforces that user-visible CJK text lives only
// in the dictionaries (internal/i18n) and the intentionally bilingual wizard.
// Test files are exempt because they assert localized output literally.
func TestNoHardcodedCJKOutsideI18n(t *testing.T) {
	root := moduleRoot(t)
	eachGoFile(t, root, func(path string, fset *token.FileSet, file *ast.File) {
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, "_test.go") {
			return
		}
		if strings.HasPrefix(rel, "internal/i18n/") {
			return
		}
		if rel == "internal/config/wizard.go" {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if hasCJK(lit.Value) {
				t.Errorf("hardcoded CJK string literal at %s: %s", fset.Position(lit.Pos()), lit.Value)
			}
			return true
		})
	})
}

// TestAllI18nKeysExist ensures every literal key passed to i18n.T is defined in
// both dictionaries. i18n.T returns the key verbatim when it is missing, which
// leaks raw keys into the UI.
func TestAllI18nKeysExist(t *testing.T) {
	root := moduleRoot(t)
	checked := 0
	eachGoFile(t, root, func(path string, fset *token.FileSet, file *ast.File) {
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/i18n/") {
			return
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "i18n" || sel.Sel.Name != "T" {
				return true
			}
			if len(call.Args) == 0 {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				// Dynamic keys follow the documented enum pattern
				// (e.g. "source.type."+string(t)) and are checked by hand.
				return true
			}
			key := strings.Trim(lit.Value, `"`)
			checked++
			if _, ok := enDict[key]; !ok {
				t.Errorf("i18n key %q used at %s is missing from en dictionary", key, fset.Position(call.Pos()))
			}
			if _, ok := zhCNDict[key]; !ok {
				t.Errorf("i18n key %q used at %s is missing from zh-CN dictionary", key, fset.Position(call.Pos()))
			}
			return true
		})
	})
	if checked == 0 {
		t.Fatal("no i18n.T literal keys were checked; guard is ineffective")
	}
}
