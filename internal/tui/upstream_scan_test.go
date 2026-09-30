package tui

import (
	"strings"
	"testing"

	"asoul/internal/i18n"
	"asoul/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

func ctrlSKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlS} }

func TestUpstreamScanModalTabAndModeNavigation(t *testing.T) {
	m := regressionModel(t)
	m.activeTab = tabUpstreams
	m.Update(runeKey("a"))

	if m.view != viewModalAddUpstream {
		t.Fatalf("expected add-upstream modal, got %v", m.view)
	}
	if m.addUpstreamModal.Tab != 0 || m.addUpstreamModal.Active != 1 {
		t.Fatalf("unexpected initial focus tab=%d active=%d", m.addUpstreamModal.Tab, m.addUpstreamModal.Active)
	}

	// Tab cycles through the field groups of the basic tab and back to the tab bar.
	for _, want := range []int{2, 3, 0} {
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if m.addUpstreamModal.Active != want {
			t.Fatalf("tab focus = %d, want %d", m.addUpstreamModal.Active, want)
		}
	}

	// Focus is on the tab bar; switch to the scan tab with the right arrow.
	if m.addUpstreamModal.Active != 0 {
		t.Fatalf("expected tab bar focus, got %d", m.addUpstreamModal.Active)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.addUpstreamModal.Tab != 1 || m.addUpstreamModal.Active != 1 {
		t.Fatalf("expected scan tab mode focus, got tab=%d active=%d", m.addUpstreamModal.Tab, m.addUpstreamModal.Active)
	}

	// The scan mode cycles with the horizontal keys.
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.addUpstreamModal.Scan.Mode != upstreamScanModeCustom {
		t.Fatalf("mode = %d, want custom", m.addUpstreamModal.Scan.Mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.addUpstreamModal.Scan.Mode != upstreamScanModeWhole {
		t.Fatalf("mode = %d, want whole", m.addUpstreamModal.Scan.Mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.addUpstreamModal.Scan.Mode != upstreamScanModeCustom {
		t.Fatalf("mode = %d, want custom after left", m.addUpstreamModal.Scan.Mode)
	}

	// With a custom scope the roots textarea becomes focusable.
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.addUpstreamModal.Active != 2 {
		t.Fatalf("expected roots focus, got %d", m.addUpstreamModal.Active)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.addUpstreamModal.Active != 3 {
		t.Fatalf("expected exclude focus, got %d", m.addUpstreamModal.Active)
	}

	// Whole-source mode skips the roots field.
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // back to tab bar
	m.Update(tea.KeyMsg{Type: tea.KeyTab}) // mode
	if m.addUpstreamModal.Active != 1 {
		t.Fatalf("expected mode focus, got %d", m.addUpstreamModal.Active)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft}) // custom -> default
	m.Update(tea.KeyMsg{Type: tea.KeyLeft}) // default -> whole
	if m.addUpstreamModal.Scan.Mode != upstreamScanModeWhole {
		t.Fatalf("mode = %d, want whole", m.addUpstreamModal.Scan.Mode)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.addUpstreamModal.Active != 3 {
		t.Fatalf("whole-source mode should skip roots, got active=%d", m.addUpstreamModal.Active)
	}
}

func TestUpstreamScanModalSavesCustomScope(t *testing.T) {
	m := regressionModel(t)
	m.activeTab = tabUpstreams
	m.Update(runeKey("a"))

	m.addUpstreamModal.URLInput.SetValue("https://example.com/repo.git")
	m.addUpstreamModal.Scan.Mode = upstreamScanModeCustom
	m.addUpstreamModal.Scan.RootsInput.SetValue("skills\npackages/shared-skills")
	m.addUpstreamModal.Scan.ExcludeInput.SetValue("skills/templates")

	_, cmd := m.Update(ctrlSKey())
	if cmd == nil {
		t.Fatal("registration not scheduled")
	}
	m.Update(cmd())

	cfg, err := m.service.Config()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Upstreams) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(cfg.Upstreams))
	}
	scan := cfg.Upstreams[0].Scan
	if scan == nil {
		t.Fatal("custom scope not persisted")
	}
	if len(scan.Roots) != 2 || scan.Roots[0] != "skills" || scan.Roots[1] != "packages/shared-skills" {
		t.Fatalf("unexpected roots: %+v", scan.Roots)
	}
	if len(scan.Exclude) != 1 || scan.Exclude[0] != "skills/templates" {
		t.Fatalf("unexpected excludes: %+v", scan.Exclude)
	}
}

func TestUpstreamScanModalRejectsInvalidScope(t *testing.T) {
	m := regressionModel(t)
	m.activeTab = tabUpstreams
	m.Update(runeKey("a"))

	m.addUpstreamModal.URLInput.SetValue("https://example.com/repo.git")
	m.addUpstreamModal.Scan.Mode = upstreamScanModeCustom
	m.addUpstreamModal.Scan.RootsInput.SetValue("../escape")

	_, cmd := m.Update(ctrlSKey())
	if cmd != nil {
		t.Fatal("invalid scope must not schedule registration")
	}
	if m.err == nil {
		t.Fatal("invalid scope should surface an error")
	}
	cfg, err := m.service.Config()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Upstreams) != 0 {
		t.Fatalf("invalid upstream persisted: %+v", cfg.Upstreams)
	}

	// Enter inside a multi-line directory box must not submit.
	m.addUpstreamModal.Scan.RootsInput.SetValue("skills")
	m.addUpstreamModal.Tab = 1
	m.addUpstreamModal.Active = 2
	m.err = nil
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("Enter in the roots textarea must not submit")
	}
}

func TestUpstreamScanModalRendersBilingual(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN"} {
		t.Run(lang, func(t *testing.T) {
			i18n.Init(lang)
			st := newAddUpstreamModal()
			out := RenderAddUpstreamModal(&st, boxStyle, nil)
			if !strings.Contains(out, i18n.T("modal.upstream.tab.scan")) {
				t.Fatalf("scan tab label missing:\n%s", out)
			}

			st.Tab = 1
			st.Active = 1
			st.Scan.Mode = upstreamScanModeWhole
			out = RenderAddUpstreamModal(&st, boxStyle, nil)
			if !strings.Contains(out, i18n.T("modal.upstream.scan.mode_whole")) {
				t.Fatalf("whole-source label missing:\n%s", out)
			}
			if !strings.Contains(out, i18n.T("modal.upstream.scan.exclude_label")) {
				t.Fatalf("exclude label missing:\n%s", out)
			}
		})
	}
	i18n.Init("en")
}

func TestUpstreamDetailModalShowsScanScope(t *testing.T) {
	i18n.Init("en")
	up := model.UpstreamInfo{
		URL:         "https://example.com/repo",
		Type:        model.SourceTypeGit,
		ScanRoots:   []string{"skills", "plugins"},
		ScanExclude: []string{"skills/templates"},
	}
	rendered := RenderUpstreamDetailModalForTest(up, 100, 40)
	if !strings.Contains(rendered, "skills, plugins") {
		t.Fatalf("detail modal missing scan roots:\n%s", rendered)
	}
	if !strings.Contains(rendered, "skills/templates") {
		t.Fatalf("detail modal missing scan exclude:\n%s", rendered)
	}
}
