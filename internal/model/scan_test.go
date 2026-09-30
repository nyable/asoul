package model_test

import (
	"testing"

	"asoul/internal/model"
)

func TestNormalizeScanPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "", false},
		{"   ", "", false},
		{".", ".", true},
		{"skills", "skills", true},
		{"skills/", "skills", true},
		{"  skills/pdf  ", "skills/pdf", true},
		{`skills\pdf`, "skills/pdf", true},
		{"skills//pdf", "skills/pdf", true},
		{"/etc", "", false},
		{"../secret", "", false},
		{"skills/../../etc", "", false},
		{`C:\Users`, "", false},
	}
	for _, tc := range cases {
		got, ok := model.NormalizeScanPath(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("NormalizeScanPath(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestNormalizeScanConfigDefaultsAndValidation(t *testing.T) {
	got, err := model.NormalizeScanConfig(model.ScanConfig{})
	if err != nil {
		t.Fatalf("empty scope should normalize: %v", err)
	}
	if len(got.Roots) != 1 || got.Roots[0] != model.ScanRootDefault || len(got.Exclude) != 0 {
		t.Fatalf("empty scope should default to skills, got %+v", got)
	}

	got, err = model.NormalizeScanConfig(model.ScanConfig{Roots: []string{"skills", "skills", "plugins"}, Exclude: []string{"skills/demo"}})
	if err != nil {
		t.Fatalf("valid scope rejected: %v", err)
	}
	if len(got.Roots) != 2 || got.Roots[0] != "skills" || got.Roots[1] != "plugins" {
		t.Fatalf("roots not deduped/preserved: %+v", got.Roots)
	}

	if _, err := model.NormalizeScanConfig(model.ScanConfig{Roots: []string{".", "skills"}}); err == nil {
		t.Fatal("whole source combined with another root must be rejected")
	}
	if _, err := model.NormalizeScanConfig(model.ScanConfig{Roots: []string{"skills"}, Exclude: []string{"."}}); err == nil {
		t.Fatal("excluding the whole source must be rejected")
	}
	if _, err := model.NormalizeScanConfig(model.ScanConfig{Roots: []string{"/abs"}}); err == nil {
		t.Fatal("absolute scan root must be rejected")
	}
}

func TestScanConfigModeAndStored(t *testing.T) {
	if mode := (model.ScanConfig{}).Mode(); mode != model.ScanModeDefault {
		t.Fatalf("empty scope mode = %q, want default", mode)
	}
	if mode := (model.ScanConfig{Roots: []string{"."}}).Mode(); mode != model.ScanModeWhole {
		t.Fatalf("whole scope mode = %q, want whole", mode)
	}
	if mode := (model.ScanConfig{Roots: []string{"skills", "plugins"}}).Mode(); mode != model.ScanModeCustom {
		t.Fatalf("multi-root mode = %q, want custom", mode)
	}
	stored, err := (model.ScanConfig{}).Stored()
	if err != nil || stored != nil {
		t.Fatalf("default scope should store nil, got %+v err=%v", stored, err)
	}
	stored, err = (model.ScanConfig{Roots: []string{"."}}).Stored()
	if err != nil || stored == nil || len(stored.Roots) != 1 || stored.Roots[0] != "." {
		t.Fatalf("whole scope should be persisted explicitly, got %+v err=%v", stored, err)
	}
	if _, err := (model.ScanConfig{Roots: []string{"../escape"}}).Stored(); err == nil {
		t.Fatal("invalid scope must not be stored silently")
	}
}

func TestInScanScopeBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		rel     string
		roots   []string
		exclude []string
		want    bool
	}{
		{"inside default", "skills/pdf", []string{"skills"}, nil, true},
		{"skill dir at root of default", "skills", []string{"skills"}, nil, true},
		{"template outside default", "template", []string{"skills"}, nil, false},
		{"prefix is not a boundary", "skills-backup/x", []string{"skills"}, nil, false},
		{"source root not in default", "", []string{"skills"}, nil, false},
		{"whole source root", "", []string{"."}, nil, true},
		{"nested inside custom root", "packages/shared/skills/a", []string{"packages/shared/skills"}, nil, true},
		{"exclude wins", "skills/templates/a", []string{"skills"}, []string{"skills/templates"}, false},
		{"exclude boundary", "skills/templates-extra", []string{"skills"}, []string{"skills/templates"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := model.InScanScope(tc.rel, tc.roots, tc.exclude); got != tc.want {
				t.Fatalf("InScanScope(%q, %v, %v) = %v, want %v", tc.rel, tc.roots, tc.exclude, got, tc.want)
			}
		})
	}
}
