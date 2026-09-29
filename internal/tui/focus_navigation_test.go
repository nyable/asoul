package tui_test

import (
	"context"
	"testing"

	"asoul/internal/model"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func focusTestModel(t *testing.T) *tui.Model {
	t.Helper()
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	t.Cleanup(cleanup)
	_ = svc.TargetAdd("agent-ch", "/tmp/agent-ch")
	return tui.NewModel(ctx, svc)
}

// TestFocusArrowsNeverCrossLevels asserts that ←/→ stay inside the focused level.
func TestFocusArrowsNeverCrossLevels(t *testing.T) {
	m := focusTestModel(t)

	// depth 0: arrows cycle top-level tabs.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}))
	if m.ActiveTabForTest() != tui.TabSkillsForTest {
		t.Fatalf("expected Skills tab, got %d", m.ActiveTabForTest())
	}
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRight}))
	if m.ActiveTabForTest() != tui.TabChannelsForTest || m.FocusDepthForTest() != 0 {
		t.Fatalf("expected Right at depth 0 to move to the next top-level tab, got tab=%d depth=%d", m.ActiveTabForTest(), m.FocusDepthForTest())
	}

	// Return to Skills and descend.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	if m.FocusDepthForTest() != 1 {
		t.Fatalf("expected Tab to enter the secondary level")
	}

	// depth 1: arrows cycle the secondary tabs and never touch the top-level tab.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRight}))
	if m.ActiveTabForTest() != tui.TabSkillsForTest || m.SkillSubTabForTest() != 1 {
		t.Fatalf("expected Right at depth 1 to switch to Groups, got tab=%d subtab=%d", m.ActiveTabForTest(), m.SkillSubTabForTest())
	}
	// Wraps within the secondary level.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRight}))
	if m.ActiveTabForTest() != tui.TabSkillsForTest || m.SkillSubTabForTest() != 0 {
		t.Fatalf("expected Right to wrap back to Skills, got tab=%d subtab=%d", m.ActiveTabForTest(), m.SkillSubTabForTest())
	}
}

// TestFocusTabCyclesLevels asserts Tab/Shift+Tab wrap between the page's levels.
func TestFocusTabCyclesLevels(t *testing.T) {
	m := focusTestModel(t)

	// A page without sub-tabs ignores Tab/Shift+Tab (no observable change).
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}}))
	before := m.ActiveTabForTest()
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyShiftTab}))
	if m.FocusDepthForTest() != 0 || m.ActiveTabForTest() != before {
		t.Fatalf("Tab/Shift+Tab on a sub-tab-less page must not change focus or tab, got depth=%d tab=%d", m.FocusDepthForTest(), m.ActiveTabForTest())
	}

	// A page with sub-tabs cycles 0 -> 1 -> 0 with Tab.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	if m.FocusDepthForTest() != 1 {
		t.Fatalf("expected Tab to enter the secondary level, got depth=%d", m.FocusDepthForTest())
	}
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	if m.FocusDepthForTest() != 0 {
		t.Fatalf("expected Tab to wrap back to the primary level, got depth=%d", m.FocusDepthForTest())
	}

	// Shift+Tab wraps the same way (with two levels it equals Tab).
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyShiftTab}))
	if m.FocusDepthForTest() != 1 {
		t.Fatalf("expected Shift+Tab to enter the secondary level, got depth=%d", m.FocusDepthForTest())
	}
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyShiftTab}))
	if m.FocusDepthForTest() != 0 {
		t.Fatalf("expected Shift+Tab to wrap back to the primary level, got depth=%d", m.FocusDepthForTest())
	}
}

// TestAddModalTabCyclesFocusGroups asserts the Add modal's three focus groups
// (source input -> path input -> upstream list) cycle in both directions.
func TestAddModalTabCyclesFocusGroups(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	m = asModel(t, mustUpdate(m, tui.NewUpstreamReloadMsgForTest([]model.UpstreamInfo{
		{URL: "https://github.com/example/skills", Type: model.SourceTypeGit},
	})))
	// Open the Add skill modal from the Skills page. With a registered upstream
	// the modal starts focused on the upstream list.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}))
	if fromUp, _ := m.AddModalFocusForTest(); !fromUp {
		t.Fatalf("expected the Add modal to start in upstream-list mode")
	}

	// Forward cycle: upstream list -> source(0) -> path(1) -> upstream list.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	if fromUp, active := m.AddModalFocusForTest(); fromUp || active != 0 {
		t.Fatalf("expected Tab from the upstream list to focus the source input, got fromUpstream=%v active=%d", fromUp, active)
	}
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	if fromUp, active := m.AddModalFocusForTest(); fromUp || active != 1 {
		t.Fatalf("expected Tab to focus the path input, got fromUpstream=%v active=%d", fromUp, active)
	}
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	if fromUp, _ := m.AddModalFocusForTest(); !fromUp {
		t.Fatalf("expected Tab from the path input to wrap into the upstream list")
	}

	// Reverse cycle: upstream list -> path(1) -> source(0) -> upstream list.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyShiftTab}))
	if fromUp, active := m.AddModalFocusForTest(); fromUp || active != 1 {
		t.Fatalf("expected Shift+Tab from the upstream list to return to the path input, got fromUpstream=%v active=%d", fromUp, active)
	}
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyShiftTab}))
	if fromUp, active := m.AddModalFocusForTest(); fromUp || active != 0 {
		t.Fatalf("expected Shift+Tab from the path input to focus the source input, got fromUpstream=%v active=%d", fromUp, active)
	}
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyShiftTab}))
	if fromUp, _ := m.AddModalFocusForTest(); !fromUp {
		t.Fatalf("expected Shift+Tab from the source input to wrap into the upstream list")
	}
}

// TestFocusBracketKeysTopLevelOnly asserts [/] never act on the secondary level.
func TestFocusBracketKeysTopLevelOnly(t *testing.T) {
	m := focusTestModel(t)
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}}))
	if m.ActiveTabForTest() != tui.TabSkillsForTest || m.FocusDepthForTest() != 1 {
		t.Fatalf("] must be a no-op at depth 1, got tab=%d depth=%d", m.ActiveTabForTest(), m.FocusDepthForTest())
	}
}

// TestFocusResetsOnPageEntry asserts secondary selection rewinds to the first
// item every time a page is entered.
func TestFocusResetsOnPageEntry(t *testing.T) {
	m := focusTestModel(t)
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRight}))
	if m.SkillSubTabForTest() != 1 {
		t.Fatalf("expected Groups secondary tab")
	}
	// Jump to another tab and back: secondary must reset to Skills and focus to 0.
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}))
	m = asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}}))
	if m.FocusDepthForTest() != 0 || m.SkillSubTabForTest() != 0 {
		t.Fatalf("expected page entry to reset focus to depth 0 / Skills, got depth=%d subtab=%d", m.FocusDepthForTest(), m.SkillSubTabForTest())
	}
}
