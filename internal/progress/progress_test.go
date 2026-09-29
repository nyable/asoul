package progress

import "testing"

func TestParseGitLineCounts(t *testing.T) {
	cases := []struct {
		line    string
		phase   Phase
		current int
		total   int
		ok      bool
	}{
		{"Receiving objects:  45% (100/200), 1.2 MiB | 1.0 MiB/s", PhaseReceiving, 100, 200, true},
		{"remote: Counting objects: 100% (200/200)", PhaseCounting, 200, 200, true},
		{"remote: Compressing objects:  75% (150/200)", PhaseCompressing, 150, 200, true},
		{"Resolving deltas:  30% (60/200)", PhaseResolving, 60, 200, true},
		{"Cloning into bare repository '/tmp/repo.git'...", PhaseClone, 0, 0, true},
		{"From https://github.com/example/repo", PhaseFetch, 0, 0, true},
		{"fatal: repository not found", PhaseUnknown, 0, 0, false},
	}
	for _, tc := range cases {
		got, ok := ParseGitLine(tc.line)
		if ok != tc.ok {
			t.Fatalf("ParseGitLine(%q) ok=%v want %v", tc.line, ok, tc.ok)
		}
		if !ok {
			continue
		}
		if got.Phase != tc.phase || got.Current != tc.current || got.Total != tc.total {
			t.Fatalf("ParseGitLine(%q) = %+v, want phase=%v %d/%d", tc.line, got, tc.phase, tc.current, tc.total)
		}
	}
}

func TestUpdateDeterminate(t *testing.T) {
	if (Update{Total: 0}).Determinate() {
		t.Fatal("zero total must be indeterminate")
	}
	if !(Update{Current: 1, Total: 2}).Determinate() {
		t.Fatal("counted update must be determinate")
	}
}

func TestPhaseI18nKeysAreDistinct(t *testing.T) {
	seen := map[string]Phase{}
	for _, p := range []Phase{
		PhaseClone, PhaseFetch, PhaseCounting, PhaseCompressing, PhaseReceiving,
		PhaseResolving, PhaseExtract, PhaseValidate, PhaseInstall, PhaseCheck,
		PhaseRemove, PhaseDeploy, PhaseUpdate, PhaseScan,
	} {
		key := p.I18nKey()
		if key == "" {
			t.Fatalf("phase %d has no i18n key", p)
		}
		if other, dup := seen[key]; dup {
			t.Fatalf("phase %d and %d share key %q", p, other, key)
		}
		seen[key] = p
	}
	if PhaseUnknown.I18nKey() != "" {
		t.Fatal("unknown phase must not map to a label key")
	}
}
