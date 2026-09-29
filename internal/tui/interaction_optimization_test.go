package tui_test

import (
	"context"
	"strings"
	"testing"

	"asoul/internal/i18n"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTopTitleRemoved(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	var m tea.Model = tui.NewModel(ctx, svc)
	view := m.View()

	// Should not have the redundant app.title at the top
	if strings.Contains(view, "asoul - Agent Skills") || strings.Contains(view, "本地管理中心") {
		t.Fatalf("expected redundant top title to be removed, got:\n%s", view)
	}

	// First line or early content should directly be the tabs bar
	if !strings.Contains(view, "1. Overview") && !strings.Contains(view, "1. 概览") {
		t.Fatalf("expected tabs bar at top of view, got:\n%s", view)
	}
}

func TestFooterShortcutWrapping(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	// Test in Chinese
	i18n.Init("zh-CN")
	var mZh tea.Model = newTestSkillsModel(ctx, svc)
	mZh, _ = mZh.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	viewZh := mZh.View()

	// The full skills shortcuts should all appear without being truncated
	expectedShortcutsZh := []string{
		"[Space] 勾选",
		"[a] 添加",
		"[n] 新建",
		"[i] 详情",
		"[v] 差异",
		"[u] 更新",
		"[d] 部署",
		"[r] 刷新",
		"[x] 删除",
		"[X] 条件删除",
		"[/] 搜索",
		"[?] 帮助",
		"[q] 退出",
	}
	for _, sc := range expectedShortcutsZh {
		if !strings.Contains(viewZh, sc) {
			t.Errorf("expected wrapped footer to contain shortcut %q, but got view:\n%s", sc, viewZh)
		}
	}

	// Test in English
	i18n.Init("en")
	var mEn tea.Model = newTestSkillsModel(ctx, svc)
	mEn, _ = mEn.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	viewEn := mEn.View()

	expectedShortcutsEn := []string{
		"[Space] select",
		"[a] add",
		"[n] new",
		"[i] view",
		"[v] diff",
		"[u] update",
		"[d] deploy",
		"[r] refresh",
		"[x] rm",
		"[X] filter rm",
		"[/] search",
		"[?] help",
		"[q] quit",
	}
	for _, sc := range expectedShortcutsEn {
		if !strings.Contains(viewEn, sc) {
			t.Errorf("expected wrapped footer to contain shortcut %q, but got view:\n%s", sc, viewEn)
		}
	}
}

// TestPageFooterHints verifies the per-page footer no longer repeats tab
// navigation (visible in the pinned top tab bar) but still advertises quitting,
// and drops the generic arrow-key navigation hints.
func TestPageFooterHints(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	for _, lang := range []string{"zh-CN", "en"} {
		i18n.Init(lang)
		var m tea.Model = newTestSkillsModel(ctx, svc)
		m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 30})

		view := m.View()
		if strings.Contains(view, "1-7") {
			t.Errorf("page footer should not repeat tab-navigation hint in %s, got:\n%s", lang, view)
		}
		if !strings.Contains(view, "[q]") {
			t.Errorf("page footer should still advertise quit in %s, got:\n%s", lang, view)
		}
		if strings.Contains(view, "↑/↓") {
			t.Errorf("page footer should not describe arrow-key navigation in %s, got:\n%s", lang, view)
		}
	}
	i18n.Init("en")
}

func TestLoadingBlocksOtherKeys(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-a")
	_ = svc.NewSkill(ctx, "skill-b")

	var m tea.Model = newTestSkillsModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Trigger update on skill-a -> enters loading state
	mLoading, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if cmd == nil {
		t.Fatal("expected update command")
	}

	viewLoading := mLoading.View()
	if !strings.Contains(viewLoading, "Updating skill-a") {
		t.Fatalf("expected view to show loading notice for skill-a, got:\n%s", viewLoading)
	}
	if !strings.Contains(viewLoading, "Operation in progress") && !strings.Contains(viewLoading, "操作进行中") {
		t.Fatalf("expected view to display loading progress card, got:\n%s", viewLoading)
	}

	// Attempt navigation or switching tabs during loading -> should be ignored
	mAfterKey, keyCmd := mLoading.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	if keyCmd != nil {
		t.Fatal("expected no command when keys are pressed during loading")
	}
	// View should still be on Skills tab and in loading state, not Targets tab
	if strings.Contains(mAfterKey.View(), "TARGET CHANNEL") || strings.Contains(mAfterKey.View(), "PROJECT PATH") {
		t.Fatalf("tab switch should have been blocked during loading, got:\n%s", mAfterKey.View())
	}

	// Attempt deletion during loading -> should be ignored
	mAfterDelete, delCmd := mLoading.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if delCmd != nil {
		t.Fatal("expected no command on delete key during loading")
	}
	if strings.Contains(mAfterDelete.View(), "Remove Skill") || strings.Contains(mAfterDelete.View(), "删除技能") {
		t.Fatalf("remove modal should have been blocked during loading, got:\n%s", mAfterDelete.View())
	}

	// Esc should cancel loading
	mAfterEsc, _ := mLoading.Update(tea.KeyMsg{Type: tea.KeyEsc})
	viewAfterEsc := mAfterEsc.View()
	if strings.Contains(viewAfterEsc, "Operation in progress") || strings.Contains(viewAfterEsc, "操作进行中") {
		t.Fatalf("loading state should have been cancelled by Esc, got:\n%s", viewAfterEsc)
	}
}

func TestDeployModalRetainsLoadingView(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-deploy")
	targetDir := t.TempDir()
	_ = svc.TargetAdd("agent-target", targetDir)

	var m tea.Model = tui.NewModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Switch to targets tab (4)
	mTargets, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	// Open deploy modal on target
	mDeploy, _ := mTargets.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	// Select skill
	mDeploy, _ = mDeploy.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	// Press Enter to trigger deploy directly (locked target flow)
	mExecuting, cmd := mDeploy.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected async deploy command on enter")
	}

	// BEFORE cmd returns: modal must stay in deploy view and display loading indicator
	viewExecuting := mExecuting.View()
	tuiMExecuting, ok := mExecuting.(*tui.Model)
	if !ok {
		t.Fatal("expected *tui.Model")
	}
	if tuiMExecuting.CurrentViewForTest() != tui.ViewDeployForTest {
		t.Fatalf("modal should NOT have prematurely returned to list view during deploy, got view: %d", tuiMExecuting.CurrentViewForTest())
	}
	if !strings.Contains(viewExecuting, "Deploying") && !strings.Contains(viewExecuting, "正在部署") {
		t.Fatalf("expected modal to show deploy loading indicator, got:\n%s", viewExecuting)
	}

	// Keys should be blocked during deploy
	mBlocked, blockedCmd := mExecuting.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if blockedCmd != nil {
		t.Fatal("expected no command on blocked key during deploy loading")
	}
	tuiMBlocked := mBlocked.(*tui.Model)
	if tuiMBlocked.CurrentViewForTest() != tui.ViewDeployForTest {
		t.Fatalf("tab switch should be blocked during deploy loading, got view: %d", tuiMBlocked.CurrentViewForTest())
	}

	// When async command completes and emits asyncNoticeMsg
	msg := cmd()
	mDone, _ := mExecuting.Update(msg)
	viewDone := mDone.View()

	// NOW it should return to list view and display success notice
	tuiMDone := mDone.(*tui.Model)
	if tuiMDone.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected return to list view after deploy, got view: %d", tuiMDone.CurrentViewForTest())
	}
	if !strings.Contains(viewDone, "Successfully deployed") && !strings.Contains(viewDone, "部署") {
		t.Fatalf("expected deploy notice after completion, got:\n%s", viewDone)
	}
}

func TestRemovalModalLoading(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-rm")

	var m tea.Model = newTestSkillsModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Open delete confirmation modal on selected skill
	mConfirm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	viewConfirm := mConfirm.View()
	if !strings.Contains(viewConfirm, "skill-rm") {
		t.Fatalf("expected confirm modal for skill-rm, got:\n%s", viewConfirm)
	}

	// Press 'y' to confirm deletion -> should trigger async removal and show loading
	mExecuting, cmd := mConfirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected async remove command on y")
	}

	// BEFORE cmd returns: modal must stay in confirm view and display loading indicator
	viewExecuting := mExecuting.View()
	tuiMExecuting := mExecuting.(*tui.Model)
	if tuiMExecuting.CurrentViewForTest() != tui.ViewModalConfirmForTest {
		t.Fatalf("modal should NOT have prematurely returned to list view during removal, got view: %d", tuiMExecuting.CurrentViewForTest())
	}
	if !strings.Contains(viewExecuting, "Executing") && !strings.Contains(viewExecuting, "正在执行") {
		t.Fatalf("expected confirm modal to show loading state, got:\n%s", viewExecuting)
	}

	// When async command completes
	msg := cmd()
	mDone, _ := mExecuting.Update(msg)
	viewDone := mDone.View()

	// NOW it should return to list view and reflect the deletion
	tuiMDone := mDone.(*tui.Model)
	if tuiMDone.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected return to list view after removal, got view: %d", tuiMDone.CurrentViewForTest())
	}
	if !strings.Contains(viewDone, "Removed skill") && !strings.Contains(viewDone, "已删除技能") {
		t.Fatalf("expected removal notice after completion, got:\n%s", viewDone)
	}
}

func TestModalCenteredOverlay(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-center-a")
	_ = svc.NewSkill(ctx, "skill-center-b")

	var m tea.Model = newTestSkillsModel(ctx, svc)
	width, height := 120, 30
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})

	// Open remove confirmation modal (which opens viewModalConfirm)
	mModal, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	view := mModal.View()

	// 1. Background tabs bar and content must still be visible in the rendered output
	if !strings.Contains(view, "1. Overview") && !strings.Contains(view, "1. 概览") {
		t.Fatalf("expected background tabs to be visible behind modal, got:\n%s", view)
	}

	// 2. Modal content must be present
	if !strings.Contains(view, "skill-center-a") {
		t.Fatalf("expected modal to contain selected skill name, got:\n%s", view)
	}
	if !strings.Contains(view, "[y]") {
		t.Fatalf("expected modal hotkey [y] to be present, got:\n%s", view)
	}

	// 3. Modal border must be centered horizontally (not left-aligned at column 0)
	lines := strings.Split(view, "\n")
	foundTopBorder := false
	for _, line := range lines {
		if strings.Contains(line, "╭─") {
			foundTopBorder = true
			idx := strings.Index(line, "╭")
			if idx < 10 {
				t.Fatalf("modal border should be centered horizontally (found at column %d in line: %s)", idx, line)
			}
			break
		}
	}
	if !foundTopBorder {
		t.Fatalf("expected to find rounded modal top border '╭─' in view:\n%s", view)
	}
}

func TestLoadingModalCenteredOverlayOnRefreshAndUpdate(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-load-1")

	var m tea.Model = newTestSkillsModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// 1. Test Refresh ('r') -> shows centered loading overlay modal
	mRefreshed, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Fatal("expected refresh command on r")
	}
	viewRefresh := mRefreshed.View()

	if !strings.Contains(viewRefresh, "1. Overview") && !strings.Contains(viewRefresh, "1. 概览") {
		t.Fatalf("expected background tabs behind refresh loading modal, got:\n%s", viewRefresh)
	}
	if !strings.Contains(viewRefresh, "Operation in progress") && !strings.Contains(viewRefresh, "操作进行中") {
		t.Fatalf("expected loading modal title in refresh view, got:\n%s", viewRefresh)
	}

	// Verify horizontal centering of loading modal
	lines := strings.Split(viewRefresh, "\n")
	foundTopBorder := false
	for _, line := range lines {
		if strings.Contains(line, "╭─") {
			foundTopBorder = true
			idx := strings.Index(line, "╭")
			if idx < 15 {
				t.Fatalf("loading modal should be centered horizontally, got column %d in line: %s", idx, line)
			}
			break
		}
	}
	if !foundTopBorder {
		t.Fatalf("expected loading modal border '╭─' in:\n%s", viewRefresh)
	}

	// Cancel loading
	mCancelled, _ := mRefreshed.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// 2. Test Update ('u') -> shows centered loading overlay modal
	mUpdate, uCmd := mCancelled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if uCmd == nil {
		t.Fatal("expected update command on u")
	}
	viewUpdate := mUpdate.View()

	if !strings.Contains(viewUpdate, "Updating skill-load-1") {
		t.Fatalf("expected update notice in loading modal, got:\n%s", viewUpdate)
	}
	if !strings.Contains(viewUpdate, "1. Overview") && !strings.Contains(viewUpdate, "1. 概览") {
		t.Fatalf("expected background tabs behind update loading modal, got:\n%s", viewUpdate)
	}

	// Verify horizontal centering of update loading modal
	foundTopBorder = false
	for _, line := range strings.Split(viewUpdate, "\n") {
		if strings.Contains(line, "╭─") {
			foundTopBorder = true
			idx := strings.Index(line, "╭")
			if idx < 15 {
				t.Fatalf("update loading modal should be centered horizontally, got column %d in line: %s", idx, line)
			}
			break
		}
	}
	if !foundTopBorder {
		t.Fatalf("expected update loading modal border in:\n%s", viewUpdate)
	}
}

func TestDeployModalPreservesTopTabsBar(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-deploy-preserve")
	targetDir := t.TempDir()
	_ = svc.TargetAdd("agent-target", targetDir)

	var m tea.Model = newTestSkillsModel(ctx, svc)
	// Standard terminal dimensions 120 x 24
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	viewBefore := m.View()
	linesBefore := strings.Split(viewBefore, "\n")
	if len(linesBefore) == 0 {
		t.Fatal("expected view lines")
	}
	tabLineBefore := linesBefore[0]
	if !strings.Contains(tabLineBefore, "1. Overview") && !strings.Contains(tabLineBefore, "1. 概览") {
		t.Fatalf("expected tabs bar on line 0 before deploy, got line: %s", tabLineBefore)
	}

	// Press 'd' to open deploy modal
	mDeploy, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	viewDeploy := mDeploy.View()
	linesDeploy := strings.Split(viewDeploy, "\n")

	// Line 0 MUST still be the tabs bar and MUST NOT be overwritten by the modal border!
	if len(linesDeploy) == 0 {
		t.Fatal("expected view lines in deploy view")
	}
	tabLineDeploy := linesDeploy[0]
	if strings.Contains(tabLineDeploy, "╭─") {
		t.Fatalf("deploy modal top border must NOT overwrite line 0 (tabs bar), got: %s", tabLineDeploy)
	}
	if !strings.Contains(tabLineDeploy, "1. Overview") && !strings.Contains(tabLineDeploy, "1. 概览") {
		t.Fatalf("tabs bar on line 0 must be preserved when deploy modal is open, got: %s", tabLineDeploy)
	}

	// Modal must be present and start at y >= 2
	modalStartLine := -1
	for idx, line := range linesDeploy {
		if strings.Contains(line, "╭─") {
			modalStartLine = idx
			break
		}
	}
	if modalStartLine < 2 {
		t.Fatalf("deploy modal top border must start at line >= 2 to leave tabs untouched, started at line %d", modalStartLine)
	}

	// Press Esc to close deploy modal
	mAfterEsc, _ := mDeploy.Update(tea.KeyMsg{Type: tea.KeyEsc})
	viewAfterEsc := mAfterEsc.View()
	linesAfterEsc := strings.Split(viewAfterEsc, "\n")
	if !strings.Contains(linesAfterEsc[0], "1. Overview") && !strings.Contains(linesAfterEsc[0], "1. 概览") {
		t.Fatalf("tabs bar on line 0 must remain after closing deploy modal, got: %s", linesAfterEsc[0])
	}
}

func TestDashboardKeepsTabsVisibleAndScrolls(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	var m tea.Model = newTestSkillsModel(ctx, svc)
	// Narrow, short terminal: the overview content would overflow without clamping.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	// Switch to the Overview tab.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatal("expected view lines")
	}
	if !strings.Contains(lines[0], "1. Overview") && !strings.Contains(lines[0], "1. 概览") {
		t.Fatalf("expected tabs bar on line 0, got: %s", lines[0])
	}
	if len(lines) > 20 {
		t.Fatalf("dashboard rendered %d lines, exceeds terminal height 20; tabs would scroll away", len(lines))
	}

	// Scrolling the dashboard must not panic and must keep the tabs pinned.
	for _, key := range []tea.Msg{
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyPgDown},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}},
		tea.KeyMsg{Type: tea.KeyUp},
	} {
		m, _ = m.Update(key)
	}
	scrolled := m.View()
	scrolledLines := strings.Split(scrolled, "\n")
	if !strings.Contains(scrolledLines[0], "1. Overview") && !strings.Contains(scrolledLines[0], "1. 概览") {
		t.Fatalf("expected tabs bar to stay on line 0 after scrolling, got: %s", scrolledLines[0])
	}
	if len(scrolledLines) > 20 {
		t.Fatalf("dashboard view exceeds terminal height after scrolling: %d lines", len(scrolledLines))
	}
}
