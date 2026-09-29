package tui_test

import (
	"context"
	"strings"
	"testing"

	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUnifiedTabNavigation_MainScreen(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "sample-skill")
	_ = svc.TargetAdd("agent-ch", "/tmp/agent-ch")
	_ = svc.TargetAdd("agent-two", "/tmp/agent-two")

	m := tui.NewModel(ctx, svc)

	// Initial view: Tab 1 (Overview)
	view := m.View()
	if !strings.Contains(view, "1. Overview") {
		t.Fatalf("expected initial view on Tab 1 (Overview), got:\n%s", view)
	}

	// Overview has no secondary level: Tab must be a strict no-op.
	if fm := asModel(t, mustUpdate(m, tea.KeyMsg{Type: tea.KeyTab})); fm.FocusDepthForTest() != 0 || fm.ActiveTabForTest() != tui.TabDashboardForTest {
		t.Fatalf("Tab on a page without sub-tabs must be a no-op, got depth=%d tab=%d", fm.FocusDepthForTest(), fm.ActiveTabForTest())
	}

	// 1. Right arrow cycles the top level: Overview -> Sources.
	newM := mustUpdate(m, tea.KeyMsg{Type: tea.KeyRight})
	view = newM.View()
	if !strings.Contains(view, "2. Sources") || asModel(t, newM).ActiveTabForTest() != tui.TabUpstreamsForTest {
		t.Fatalf("expected right arrow to go to Sources, got:\n%s", view)
	}

	// 2. Right again: Skills
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRight})
	view = newM.View()
	if !strings.Contains(view, "sample-skill") {
		t.Fatalf("expected right arrow to go to Skills, got:\n%s", view)
	}

	// 3. Right again: Channels (primary level)
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRight})
	view = newM.View()
	if asModel(t, newM).ActiveTabForTest() != tui.TabChannelsForTest || !strings.Contains(view, "agent-ch") {
		t.Fatalf("expected right arrow to go to Channels, got:\n%s", view)
	}
	if fm := asModel(t, newM); fm.FocusDepthForTest() != 0 {
		t.Fatalf("expected focus on the primary level after tab switch")
	}

	// 4. Tab descends into the secondary level; focus is now depth 1.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyTab})
	if fm := asModel(t, newM); fm.FocusDepthForTest() != 1 {
		t.Fatalf("expected Tab to descend into the secondary level, depth=%d", fm.FocusDepthForTest())
	}

	// 5. Right at depth 1 switches the selected channel (does NOT change top-level tab).
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRight})
	if fm := asModel(t, newM); fm.ChannelCursorForTest() != 1 {
		t.Fatalf("expected right arrow to advance the channel cursor, got %d", fm.ChannelCursorForTest())
	}
	if fm := asModel(t, newM); fm.ActiveTabForTest() != tui.TabChannelsForTest {
		t.Fatalf("arrow at depth 1 must not change the top-level tab, got %d", fm.ActiveTabForTest())
	}

	// 6. Right wraps back to the first channel.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRight})
	if fm := asModel(t, newM); fm.ChannelCursorForTest() != 0 {
		t.Fatalf("expected right arrow to wrap back to the first channel, got %d", fm.ChannelCursorForTest())
	}

	// 7. Shift+Tab ascends to the primary level.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyShiftTab})
	if fm := asModel(t, newM); fm.FocusDepthForTest() != 0 {
		t.Fatalf("expected Shift+Tab to return to the primary level, depth=%d", fm.FocusDepthForTest())
	}

	// 8. Right at depth 0 now changes the top-level tab: Channels -> Projects.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRight})
	if fm := asModel(t, newM); fm.ActiveTabForTest() != tui.TabProjectsForTest {
		t.Fatalf("expected right arrow to advance to Projects, got %d", fm.ActiveTabForTest())
	}

	// 9. Left at depth 0 retreats to Channels.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyLeft})
	view = newM.View()
	if asModel(t, newM).ActiveTabForTest() != tui.TabChannelsForTest || !strings.Contains(view, "agent-ch") {
		t.Fatalf("expected left arrow to retreat to Channels, got:\n%s", view)
	}

	// 10. Esc from the secondary level returns to the primary level.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyTab})
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyEsc})
	if fm := asModel(t, newM); fm.FocusDepthForTest() != 0 {
		t.Fatalf("expected Esc to leave the secondary level, depth=%d", fm.FocusDepthForTest())
	}

	// 11. Bracket keys only cycle the top level.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if fm := asModel(t, newM); fm.ActiveTabForTest() != tui.TabProjectsForTest {
		t.Fatalf("expected ] to advance to Projects, got %d", fm.ActiveTabForTest())
	}
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if fm := asModel(t, newM); fm.ActiveTabForTest() != tui.TabChannelsForTest {
		t.Fatalf("expected [ to retreat to Channels, got %d", fm.ActiveTabForTest())
	}

	// 12. Number key direct jump: press '3' to jump to Skills, focus back to depth 0.
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	view = newM.View()
	if !strings.Contains(view, "sample-skill") {
		t.Fatalf("expected '3' to jump to Skills, got:\n%s", view)
	}
	if fm := asModel(t, newM); fm.FocusDepthForTest() != 0 {
		t.Fatalf("expected direct jump to reset focus to the primary level")
	}

	// Press '1' to jump back to Overview
	newM = mustUpdate(newM, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	view = newM.View()
	if !strings.Contains(view, "1. Overview") {
		t.Fatalf("expected '1' to jump back to Overview, got:\n%s", view)
	}
}

// asModel casts a Bubble Tea model back to the TUI model for assertions.
func asModel(t *testing.T, m tea.Model) *tui.Model {
	t.Helper()
	mm, ok := m.(*tui.Model)
	if !ok {
		t.Fatalf("expected *tui.Model, got %T", m)
	}
	return mm
}

// mustUpdate applies a key message and returns the resulting model.
func mustUpdate(m tea.Model, msg tea.Msg) tea.Model {
	next, _ := m.Update(msg)
	return next
}

func TestUnifiedTabNavigation_AddModalFilters(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "alpha")
	m := tui.NewModel(ctx, svc)

	// Press 'a' to open add modal
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// Send discovered skills to enter step 1
	discMsg := tui.NewDiscoveredSkillsMsgForTest("https://github.com/microsoft/skills", "main", []string{
		"alpha",
		"beta",
	})
	newM, _ = newM.Update(discMsg)

	// Explicitly select filter All (1) first
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	modelObj := newM.(*tui.Model)
	if modelObj.AddModalFilterForTest() != tui.AddFilterAllForTest {
		t.Fatalf("expected filter All, got %d", modelObj.AddModalFilterForTest())
	}

	// 1. Initial filter is All (0). Press Right arrow (→) to go to Conflict (1)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	modelObj = newM.(*tui.Model)
	if modelObj.AddModalFilterForTest() != tui.AddFilterConflictForTest {
		t.Fatalf("expected right arrow to switch to Conflict filter, got %d", modelObj.AddModalFilterForTest())
	}

	// 2. Press Right arrow (→) again: New (2)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	modelObj = newM.(*tui.Model)
	if modelObj.AddModalFilterForTest() != tui.AddFilterNewForTest {
		t.Fatalf("expected right arrow to switch to New filter, got %d", modelObj.AddModalFilterForTest())
	}

	// 3. Press Left arrow (←): back to Conflict (1)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyLeft})
	modelObj = newM.(*tui.Model)
	if modelObj.AddModalFilterForTest() != tui.AddFilterConflictForTest {
		t.Fatalf("expected left arrow to switch back to Conflict filter, got %d", modelObj.AddModalFilterForTest())
	}

	// 4. Press Left arrow: back to All (0)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyLeft})
	modelObj = newM.(*tui.Model)
	if modelObj.AddModalFilterForTest() != tui.AddFilterAllForTest {
		t.Fatalf("expected left arrow to switch to All filter, got %d", modelObj.AddModalFilterForTest())
	}

	// 5. Press '4': direct jump to Installed (3)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelObj = newM.(*tui.Model)
	if modelObj.AddModalFilterForTest() != tui.AddFilterInstalledForTest {
		t.Fatalf("expected '4' to switch to Installed filter, got %d", modelObj.AddModalFilterForTest())
	}
}

func TestUnifiedTabNavigation_BatchRemoveModal(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "test-skill")
	m := newTestSkillsModel(ctx, svc)

	// Press 'X' to open Batch Remove modal
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	view := newM.View()
	if !strings.Contains(view, "Batch Remove") && !strings.Contains(view, "批量筛选删除") {
		t.Fatalf("expected Batch Remove modal, got:\n%s", view)
	}

	// 1. Initial mode is Regex. Empty input + Right arrow (or Tab) advances to Source
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	view = newM.View()
	if !strings.Contains(view, "managed") {
		t.Fatalf("expected right arrow to advance to Source mode, got:\n%s", view)
	}

	// 2. In Source mode, press Right arrow (→) again: advances to Channel
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	view = newM.View()
	if !strings.Contains(view, "target channel") && !strings.Contains(view, "目标渠道") {
		t.Fatalf("expected right arrow to advance to Channel mode, got:\n%s", view)
	}

	// 3. In Channel mode, press Left arrow (←): retreats to Source
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyLeft})
	view = newM.View()
	if !strings.Contains(view, "managed") {
		t.Fatalf("expected left arrow to retreat to Source mode, got:\n%s", view)
	}

	// 4. Press '1' to jump directly to Regex mode
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	view = newM.View()
	if !strings.Contains(view, "Regular Expression") && !strings.Contains(view, "正则表达式") {
		t.Fatalf("expected '1' to jump to Regex mode, got:\n%s", view)
	}
}

func TestUnifiedTabNavigation_DeployModal(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-x")
	_ = svc.TargetAdd("agent-tgt", tmpDir)

	m := newTestSkillsModel(ctx, svc)

	// Open deploy modal with 'd'
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Step 0: Select Skills (Manual vs Group)
	// Press Right arrow (→) to switch to Group mode
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	view := newM.View()
	if !strings.Contains(view, "Batch by Group") && !strings.Contains(view, "Workspace Managed") && !strings.Contains(view, "当前没有任何技能分组") {
		t.Fatalf("expected group mode after right arrow, got:\n%s", view)
	}

	// Press Left arrow (←) to switch back to Manual mode
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyLeft})
	view = newM.View()
	if !strings.Contains(view, "skill-x") {
		t.Fatalf("expected manual skills mode after left arrow, got:\n%s", view)
	}

	// Press Enter to proceed to Step 1 (Select Target)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Step 1: Select Target (Agent vs Project)
	// Press Right arrow (→) to switch to Project channel
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	view = newM.View()
	if !strings.Contains(view, "Projects (0)") && !strings.Contains(view, "No project paths cached") && !strings.Contains(view, "未配置任何项目工程") {
		t.Fatalf("expected project channel after right arrow, got:\n%s", view)
	}

	// Press Left arrow (←) to switch back to Agent channel
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyLeft})
	view = newM.View()
	if !strings.Contains(view, "agent-tgt") {
		t.Fatalf("expected agent channel after left arrow, got:\n%s", view)
	}
}
