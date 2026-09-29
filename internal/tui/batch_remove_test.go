package tui_test

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIMultiSelectAndBatchRemove(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	// Create test skills
	_ = svc.NewSkill(ctx, "skill-one")
	_ = svc.NewSkill(ctx, "skill-two")
	_ = svc.NewSkill(ctx, "skill-three")

	m := newTestSkillsModel(ctx, svc)

	// 1. Initial view should show empty checkboxes [ ] for each skill
	view := m.View()
	if !strings.Contains(view, "[ ] skill-one") || !strings.Contains(view, "[ ] skill-two") {
		t.Fatalf("expected checkboxes in skills list, got:\n%s", view)
	}

	// 2. Press Space on skill-one to check it
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	view = newM.View()
	if !strings.Contains(view, "[✓] skill-one") {
		t.Fatalf("expected skill-one to be checked, got:\n%s", view)
	}
	if !strings.Contains(view, "1 selected") && !strings.Contains(view, "已选 1 项") {
		t.Fatalf("expected footer or header to reflect 1 selected skill, got:\n%s", view)
	}

	// 3. Move cursor down and press Space on skill-three (alphabetically after skill-one)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyDown})
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	view = newM.View()
	if !strings.Contains(view, "[✓] skill-one") || !strings.Contains(view, "[✓] skill-three") {
		t.Fatalf("expected both skill-one and skill-three to be checked, got:\n%s", view)
	}

	// 4. Press 'x' to trigger batch deletion confirmation modal
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	view = newM.View()
	if !strings.Contains(view, "2 items") && !strings.Contains(view, "2 项") {
		t.Fatalf("expected batch removal modal for 2 items, got:\n%s", view)
	}
	if !strings.Contains(view, "skill-one") || !strings.Contains(view, "skill-three") {
		t.Fatalf("expected modal to list skill-one and skill-three, got:\n%s", view)
	}

	// 5. Confirm deletion by pressing 'y'
	newM, cmd := newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected async command for batch removal")
	}

	// Execute the command message
	msg := cmd()
	if errMsg, isErr := msg.(interface{ Error() string }); isErr {
		t.Fatalf("batch removal command returned error: %s", errMsg.Error())
	}

	// Feed message back to model
	newM, next := newM.Update(msg)
	newM = pump(newM, next)
	view = newM.View()

	// Verify that skill-one and skill-three are removed, skill-two remains
	if strings.Contains(view, "skill-one") || strings.Contains(view, "skill-three") {
		t.Fatalf("expected skill-one and skill-three to be deleted, got:\n%s", view)
	}
	if !strings.Contains(view, "skill-two") {
		t.Fatalf("expected skill-two to remain, got:\n%s", view)
	}
}

func TestTUISelectAllAndEscClear(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "alpha")
	_ = svc.NewSkill(ctx, "beta")

	m := newTestSkillsModel(ctx, svc)

	// Press ctrl+a to select all
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	view := newM.View()
	if !strings.Contains(view, "[✓] alpha") || !strings.Contains(view, "[✓] beta") {
		t.Fatalf("expected all skills checked after ctrl+a, got:\n%s", view)
	}

	// Press Esc to clear selection
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyEsc})
	view = newM.View()
	if strings.Contains(view, "[✓] alpha") || strings.Contains(view, "[✓] beta") {
		t.Fatalf("expected all skills unchecked after Esc, got:\n%s", view)
	}
}

func TestTUIBatchRemoveDeployedWarning(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "prod-skill")
	_ = svc.TargetAdd("agent-prod", tmpDir)
	_ = svc.Deploy(ctx, "prod-skill", "agent-prod", false)

	m := newTestSkillsModel(ctx, svc)

	// Check prod-skill
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	// Press 'x'
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	view := newM.View()

	// Verify modal shows warning about deployment to agent-prod
	if !strings.Contains(view, "agent-prod") {
		t.Fatalf("expected warning modal to mention agent-prod deployment, got:\n%s", view)
	}
}

func TestTUIBatchRemoveModal_RegexFilterAndDelete(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "front-react")
	_ = svc.NewSkill(ctx, "front-vue")
	_ = svc.NewSkill(ctx, "back-go")

	m := newTestSkillsModel(ctx, svc)

	// 1. Press 'X' (Shift+X) to open the Batch Remove Modal
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	view := newM.View()
	if !strings.Contains(view, "Batch Remove") && !strings.Contains(view, "批量筛选删除") {
		t.Fatalf("expected Batch Remove modal, got:\n%s", view)
	}

	// 2. Type regex pattern "front-.*"
	for _, ch := range "front-.*" {
		newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	view = newM.View()
	if !strings.Contains(view, "front-react") || !strings.Contains(view, "front-vue") {
		t.Fatalf("expected matching preview to show front-react and front-vue, got:\n%s", view)
	}
	if strings.Contains(view, "• back-go") {
		t.Fatalf("expected back-go NOT to match regex front-.*, got:\n%s", view)
	}

	// 3. Press Enter to proceed to confirm modal
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view = newM.View()
	if !strings.Contains(view, "2 items") && !strings.Contains(view, "2 项") {
		t.Fatalf("expected confirm modal for 2 items, got:\n%s", view)
	}

	// 4. Press 'y' to confirm deletion
	newM, cmd := newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Fatal("expected async command for batch delete")
	}
	msg := cmd()
	newM, next := newM.Update(msg)
	newM = pump(newM, next)
	view = newM.View()

	// 5. Verify front skills are removed and back-go remains
	if strings.Contains(view, "front-react") || strings.Contains(view, "front-vue") {
		t.Fatalf("expected front skills to be deleted, got:\n%s", view)
	}
	if !strings.Contains(view, "back-go") {
		t.Fatalf("expected back-go to remain, got:\n%s", view)
	}
}

func TestTUIBatchRemoveModal_SelectMatchesToMainList(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "api-auth")
	_ = svc.NewSkill(ctx, "api-billing")
	_ = svc.NewSkill(ctx, "web-portal")

	m := newTestSkillsModel(ctx, svc)

	// Press 'X'
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})

	// Type regex pattern "^api-"
	for _, ch := range "^api-" {
		newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// Press ctrl+s to select matching skills to main list
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	view := newM.View()

	// Should be back to main view with skills checked
	if !strings.Contains(view, "[✓] api-auth") || !strings.Contains(view, "[✓] api-billing") {
		t.Fatalf("expected api-auth and api-billing to be checked [✓], got:\n%s", view)
	}
	if strings.Contains(view, "[✓] web-portal") {
		t.Fatalf("expected web-portal NOT to be checked, got:\n%s", view)
	}
}

func TestTUIBatchRemoveModal_SourceAndChannelTabs(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "managed-a")
	_ = svc.NewSkill(ctx, "deployed-b")
	_ = svc.TargetAdd("agent-test", tmpDir)
	_ = svc.Deploy(ctx, "deployed-b", "agent-test", false)

	m := newTestSkillsModel(ctx, svc)

	// Press 'X'
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})

	// Switch to Tab 2 (By Source) with Right
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	view := newM.View()
	if !strings.Contains(view, "managed") {
		t.Fatalf("expected source tab to show 'managed', got:\n%s", view)
	}

	// Switch to Tab 3 (By Channel) with Right
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	view = newM.View()
	if !strings.Contains(view, "agent-test") {
		t.Fatalf("expected channel tab to show 'agent-test', got:\n%s", view)
	}
	if !strings.Contains(view, "deployed-b") {
		t.Fatalf("expected matched preview to show 'deployed-b', got:\n%s", view)
	}

	// Press 's' to select matched into main list
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	view = newM.View()
	if !strings.Contains(view, "[✓] deployed-b") {
		t.Fatalf("expected deployed-b to be checked in main list, got:\n%s", view)
	}
}

func TestTUISearchBar_RegexAndChannelFilter(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "alpha-1")
	_ = svc.NewSkill(ctx, "alpha-2")
	_ = svc.NewSkill(ctx, "gamma-9")
	_ = svc.TargetAdd("opencode-target", tmpDir)
	_ = svc.Deploy(ctx, "gamma-9", "opencode-target", false)

	m := newTestSkillsModel(ctx, svc)

	// 1. Regex search: open search with '/', type "alpha-[0-9]"
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "alpha-[0-9]" {
		newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	view := newM.View()
	if !strings.Contains(view, "alpha-1") || !strings.Contains(view, "alpha-2") {
		t.Fatalf("expected alpha-1 and alpha-2 in regex search, got:\n%s", view)
	}
	if strings.Contains(view, "gamma-9") {
		t.Fatalf("expected gamma-9 excluded from regex search, got:\n%s", view)
	}

	// 2. Press Enter to finish search input, then select all filtered with ctrl+a
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyEnter})
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	view = newM.View()
	if !strings.Contains(view, "[✓] alpha-1") || !strings.Contains(view, "[✓] alpha-2") {
		t.Fatalf("expected both alpha-1 and alpha-2 checked, got:\n%s", view)
	}

	// 3. Clear search and search channel "opencode"
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyEsc}) // clear search and selection
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "opencode" {
		newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	view = newM.View()
	if !strings.Contains(view, "gamma-9") {
		t.Fatalf("expected gamma-9 to match channel search 'opencode', got:\n%s", view)
	}
}

func TestSearchFilterMatchesCachedDeployments(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "deploy-target-skill")
	_ = svc.TargetAdd("agent-prod", tmpDir)
	_ = svc.Deploy(ctx, "deploy-target-skill", "agent-prod", false)

	m := newTestSkillsModel(ctx, svc)

	// Searching by the deployment target name must match the skill. This relies
	// on the model's cached deployment map (no per-keystroke full scan).
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "agent-prod" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	view := m.View()
	if !strings.Contains(view, "deploy-target-skill") {
		t.Fatalf("expected deployed skill to match search on its target name, got:\n%s", view)
	}
}
