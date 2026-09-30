package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/app"
	"asoul/internal/cache"
	"asoul/internal/catalog"
	"asoul/internal/config"
	"asoul/internal/i18n"
	"asoul/internal/model"
	"asoul/internal/source/git"
	"asoul/internal/state"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func regressionModel(t *testing.T) *Model {
	t.Helper()
	i18n.Init("en")
	t.Cleanup(func() { i18n.Init("en") })
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := catalog.InitWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.NewManager(filepath.Join(dir, "config.jsonc"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.NewManager(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	cm, err := cache.NewManager(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return NewModel(context.Background(), app.NewService(ws, cfg, st, cm))
}

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestNewUpstreamAutomaticallyScansWithoutImport(t *testing.T) {
	m := regressionModel(t)
	dir := t.TempDir()
	sk := filepath.Join(dir, "skills", "vue")
	if err := os.MkdirAll(sk, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("---\nname: vue\ndescription: Vue fixture\n---\n# Vue\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m.activeTab = tabUpstreams
	m.Update(runeKey("a"))
	m.addUpstreamModal.URLInput.SetValue(dir)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("registration not scheduled")
	}
	_, scan := m.Update(cmd())
	if scan == nil || !m.loading {
		t.Fatal("no automatic scan after registration")
	}
	msg := scan()
	_, reload := m.Update(msg)
	if m.view != viewModalAdd || len(m.addModal.DiscoveredSkills) != 1 || m.addModal.DiscoveredSkills[0].ID != "vue" {
		t.Fatalf("not discovered: %+v", m.addModal)
	}
	if reload != nil {
		m.Update(reload())
	}
	if len(m.upstreams) != 1 || !m.upstreams[0].Scanned || len(m.upstreams[0].AvailableSkills) != 1 || m.upstreams[0].Type != model.SourceTypeLocal {
		t.Fatalf("source count not updated %+v", m.upstreams)
	}
	statuses, err := m.service.Status(m.ctx, false)
	if err != nil || len(statuses) != 0 {
		t.Fatal("automatically imported", err)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	cfg, err := m.service.Config()
	if err != nil || len(cfg.Upstreams) != 1 {
		t.Fatal("closing removed source", err)
	}
}

func TestSourcesBindingsAndBilingualHints(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN"} {
		m := regressionModel(t)
		i18n.Init(lang)
		m.activeTab = tabUpstreams
		m.upstreams = []model.UpstreamInfo{{URL: "fixture", Type: model.SourceTypeGit}}
		m.skills = []model.SkillStatus{{ID: "vue"}}
		for _, key := range []string{"d", "D"} {
			m.Update(runeKey(key))
			if m.view != viewList {
				t.Fatal("deployment on Sources", key)
			}
		}
		if strings.Contains(m.View(), "[x/d]") {
			t.Fatal("stale remove hint")
		}
		for _, key := range []tea.KeyMsg{runeKey("x"), {Type: tea.KeyDelete}} {
			m.Update(key)
			if m.view != viewModalConfirmUpstreamRemove {
				t.Fatal("missing remove confirmation")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		}
		if !strings.Contains(m.View(), i18n.T("upstream.skills_summary.uncached", 0)) {
			t.Fatal("missing not-scanned state")
		}
	}
}

func TestCancelledScanIgnoresQueuedResultsAndProgress(t *testing.T) {
	m := regressionModel(t)
	m.activeTab = tabUpstreams
	m.upstreams = []model.UpstreamInfo{{URL: "fixture", Type: model.SourceTypeGit}}
	_ = m.startUpstreamScan("fixture", "")
	id := m.upstreamOperation
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(upstreamScanMsg{id: id, url: "fixture", skills: []git.DiscoveredSkill{{ID: "vue"}}})
	m.Update(upstreamProgressMsg{id: id})
	if m.view != viewList || m.loading || m.upstreams[0].Scanned {
		t.Fatal("late scan changed cancelled state")
	}
	if !strings.Contains(m.notice, "retained") {
		t.Fatal(m.notice)
	}
}

func TestEditorKeepsInvalidAndFailedDraft(t *testing.T) {
	m := regressionModel(t)
	m.view = viewModalEditJSON
	m.editJSONModal = newJSONEditModal("draft", "opencode", "unused", []string{"options"}, map[string]any{})
	m.editorOperation = 7
	m.Update(editorFinishedMsg{id: 7, content: "{ invalid"})
	if m.editJSONModal.Input.Value() != "{ invalid" || m.editJSONModal.Err == "" || m.loading {
		t.Fatal("invalid draft lost")
	}
	m.Update(editorFinishedMsg{id: 7, content: `{"draft":true}`, processErr: errors.New("exit 1")})
	if m.editJSONModal.Input.Value() != `{"draft":true}` || m.editJSONModal.Err == "" {
		t.Fatal("failed editor lost draft")
	}
	m.Update(editorFinishedMsg{id: 6, content: "late"})
	if m.editJSONModal.Input.Value() == "late" {
		t.Fatal("late editor changed draft")
	}
}

func TestConfigDiffReeditAndLargeDraftPreserved(t *testing.T) {
	m := regressionModel(t)
	content := "{\n" + strings.Repeat(" // long document\n", 130) + "\"value\":1\n}\n"
	m.pendingConfigEdit = &pendingConfigEdit{channel: "opencode", cfgFile: "fixture.jsonc", candidate: []byte(content), originalHash: "baseline"}
	m.isModelsDiffConfirm = true
	m.openDiff("--- original\n+++ updated\n-old\n+new")
	m.Update(runeKey("e"))
	if m.view != viewModalEditJSON || m.editJSONModal.Input.Value() != content || m.editJSONModal.OriginalHash != "baseline" {
		t.Fatal("candidate lost/truncated")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.view != viewDiff || !m.isModelsDiffConfirm {
		t.Fatal("cannot return to confirmation")
	}
	m.Update(runeKey("e"))
	m.editJSONModal.setDraft("{\"unfinished\":")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(runeKey("e"))
	if m.editJSONModal.draftContent() != "{\"unfinished\":" {
		t.Fatal("back/reopen lost unsubmitted draft")
	}
}

func TestDiffSearchHunksAndFocusIsolation(t *testing.T) {
	m := regressionModel(t)
	m.diffViewport = viewport.New(60, 5)
	m.height = 13
	m.openDiff("--- a/a\n+++ b/a\n@@ -1,20 +1,20 @@\n" + strings.Repeat(" context\n", 20) + "-needle old\n+needle new\n@@ -30 +30 @@\n-next\n+replacement\n")
	m.Update(runeKey("/"))
	m.Update(runeKey("needle"))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	first := m.diffLastJump
	if first < 20 {
		t.Fatal("search did not find displayed line", first)
	}
	m.Update(runeKey("n"))
	if m.diffLastJump <= first {
		t.Fatal("next match failed")
	}
	m.Update(runeKey("N"))
	if m.diffLastJump != first {
		t.Fatal("previous match failed")
	}
	m.Update(runeKey("g"))
	m.Update(runeKey("g"))
	if m.diffViewport.YOffset != 0 {
		t.Fatal("gg failed")
	}
	m.Update(runeKey("]"))
	m.Update(runeKey("c"))
	if m.diffLastJump == first {
		t.Fatal("hunk jump failed")
	}
	tab := m.activeTab
	m.Update(runeKey("h"))
	m.Update(runeKey("l"))
	if m.activeTab != tab {
		t.Fatal("diff navigation changed parent tab")
	}
	m.Update(runeKey("t"))
	m.Update(runeKey("/"))
	m.Update(runeKey("replacement"))
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.diffLastJump < 0 || !strings.Contains(strings.Split(m.diffRendered, "\n")[m.diffLastJump], "replacement") {
		t.Fatal("changes-only search index mismatch")
	}
}

func TestExternalDraftPreservesCRLFAndLargeContent(t *testing.T) {
	s := newJSONEditModal("draft", "opencode", "fixture", nil, nil)
	content := "{\r\n\t\"value\":1\r\n}\r\n"
	s.setDraft(content)
	if s.draftContent() != content {
		t.Fatal("exact external bytes changed")
	}
	large := "{\n" + strings.Repeat(" // line\n", 10010) + "\"value\":1\n}\n"
	s.setDraft(large)
	if !s.ReadOnly || s.draftContent() != large {
		t.Fatal("large draft was truncated")
	}
}

func TestCancelledPreviewCannotReopenDiff(t *testing.T) {
	m := regressionModel(t)
	m.view = viewModalEditJSON
	m.loading = true
	m.editorOperation = 4
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(draftPreviewMsg{id: 4, edit: &app.AgentConfigEdit{Modified: true, DiffText: "late"}})
	m.Update(draftWriteMsg{id: 4, notice: "late commit"})
	if m.view != viewModalEditJSON || m.loading {
		t.Fatal("late preview changed cancelled edit")
	}
}

func TestDiffChromeFitsNarrowTerminalAndShowsErrors(t *testing.T) {
	for _, lang := range []string{"en", "zh-CN"} {
		m := regressionModel(t)
		i18n.Init(lang)
		m.width, m.height = 70, 24
		m.openDiff("--- old\n+++ new\n@@ -1,30 +1,30 @@\n" + strings.Repeat("-old\n+new\n", 30))
		m.isModelsDiffConfirm = true
		m.err = errors.New("configuration changed")
		m.Update(runeKey("/"))
		view := m.View()
		if lipgloss.Height(view) > m.height {
			t.Fatalf("chrome overflow: %d > %d\n%s", lipgloss.Height(view), m.height, view)
		}
		if !strings.Contains(view, "configuration changed") {
			t.Fatal("diff error invisible")
		}
	}
}

func TestEditedConfigCandidateRequiresConfirmation(t *testing.T) {
	m := regressionModel(t)
	path := filepath.Join(t.TempDir(), "config.jsonc")
	original := "{\r\n // comment\r\n \"value\":1\r\n}\r\n"
	updated := strings.Replace(original, "1", "2", 1)
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := m.service.EditAgentConfigDraft(m.ctx, "opencode", path, []byte(updated), "", true)
	if err != nil {
		t.Fatal(err)
	}
	m.pendingConfigEdit = &pendingConfigEdit{channel: "opencode", cfgFile: path, originalHash: preview.OriginalHash, candidate: []byte(updated)}
	m.isModelsDiffConfirm = true
	m.openDiff(preview.DiffText)
	m.Update(runeKey("e"))
	m.editorOperation = 19
	final := strings.Replace(updated, "2", "3", 1)
	_, cmd := m.Update(editorFinishedMsg{id: 19, content: final})
	if cmd == nil {
		t.Fatal("editor did not schedule preview")
	}
	m.Update(cmd())
	if data, _ := os.ReadFile(path); string(data) != original {
		t.Fatal("editor return wrote original without confirmation")
	}
	if m.view != viewDiff || !m.isModelsDiffConfirm {
		t.Fatal("did not return to diff confirmation")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no commit command")
	}
	m.Update(cmd())
	if data, _ := os.ReadFile(path); string(data) != final {
		t.Fatal("confirmed draft not saved exactly")
	}
}
