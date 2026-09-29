package tui_test

import (
	"context"
	"strings"
	"testing"

	"asoul/internal/model"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func step(m *tui.Model, msg tea.Msg) *tui.Model {
	newM, _ := m.Update(msg)
	return newM.(*tui.Model)
}

// stepPump sends a message and drains the returned command, so async writes and
// reloads complete before the next assertion.
func stepPump(t *testing.T, m *tui.Model, msg tea.Msg) *tui.Model {
	t.Helper()
	newM, cmd := m.Update(msg)
	return pump(newM, cmd).(*tui.Model)
}

func TestSearchFilterAndSelection_SkillsTab(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "docker-helper")
	_ = svc.NewSkill(ctx, "git-helper")
	_ = svc.NewSkill(ctx, "python-tools")

	m := newTestSkillsModel(ctx, svc)

	// Verify all 3 are in view initially
	view := m.View()
	if !strings.Contains(view, "docker-helper") || !strings.Contains(view, "git-helper") || !strings.Contains(view, "python-tools") {
		t.Fatalf("expected all 3 skills in view, got:\n%s", view)
	}

	// Press '/' to search for "helper"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "helper" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// Now only docker-helper and git-helper should be visible
	view = m.View()
	if !strings.Contains(view, "docker-helper") || !strings.Contains(view, "git-helper") {
		t.Fatalf("expected docker-helper and git-helper in view, got:\n%s", view)
	}
	if strings.Contains(view, "python-tools") {
		t.Fatalf("expected python-tools to be filtered out, got:\n%s", view)
	}

	// Press down arrow from search input to transition into list
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})

	// Press Space to select the current item (git-helper or docker-helper)
	m = step(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.SelectedSkillsCountForTest() != 1 {
		t.Fatalf("expected 1 selected skill, got %d", m.SelectedSkillsCountForTest())
	}

	// Press ctrl+a: since 1 out of 2 visible is selected, ctrl+a selects all visible items (now 2)
	m = step(m, tea.KeyMsg{Type: tea.KeyCtrlA})
	if m.SelectedSkillsCountForTest() != 2 {
		t.Fatalf("expected 2 selected skills, got %d", m.SelectedSkillsCountForTest())
	}
	if !m.IsSkillSelectedForTest("docker-helper") || !m.IsSkillSelectedForTest("git-helper") {
		t.Fatal("expected both visible skills to be selected")
	}
	if m.IsSkillSelectedForTest("python-tools") {
		t.Fatal("filtered out python-tools should not be selected by visible ctrl+a")
	}

	// Press Esc to clear search
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	view = m.View()
	// Python-tools is back in view
	if !strings.Contains(view, "python-tools") {
		t.Fatalf("expected python-tools back in view after clearing search, got:\n%s", view)
	}
	// Selections still preserved
	if m.SelectedSkillsCountForTest() != 2 {
		t.Fatalf("expected selections to remain after clearing search, got %d", m.SelectedSkillsCountForTest())
	}
}

func TestSearchFilterAndSelection_UpdatesTab(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := newTestSkillsModel(ctx, svc)
	// Inject status updates
	m = step(m, tui.NewReloadStatusMsgForTest([]model.SkillStatus{
		{ID: "update-alpha", Upstream: model.UpstreamUpdateAvailable, NewCommit: "c0ffee1122"},
		{ID: "update-beta", Upstream: model.UpstreamUpdateAvailable, NewCommit: "deadbeef33"},
		{ID: "uptodate-gamma", Upstream: model.UpstreamUpToDate},
	}))

	// Verify items are present in skills tab
	view := m.View()
	if !strings.Contains(view, "update-alpha") || !strings.Contains(view, "update-beta") {
		t.Fatalf("expected update-alpha and update-beta in skills tab, got:\n%s", view)
	}

	// Search filter on updates tab for "beta"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "beta" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	view = m.View()
	if !strings.Contains(view, "update-beta") {
		t.Fatalf("expected update-beta in filtered updates tab, got:\n%s", view)
	}
	if strings.Contains(view, "update-alpha") {
		t.Fatalf("expected update-alpha to be filtered out, got:\n%s", view)
	}

	// Press Enter to exit search typing to list
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Press Space to toggle checkbox on the filtered item (update-beta)
	m = step(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.IsSkillSelectedForTest("update-beta") {
		t.Fatal("expected update-beta to be selected via space toggle")
	}
	if m.IsSkillSelectedForTest("update-alpha") {
		t.Fatal("update-alpha should not be selected")
	}

	// View should now display checked checkbox [✓] and footer with updates_selected
	view = m.View()
	if !strings.Contains(view, "[✓]") {
		t.Fatalf("expected view to contain [✓], got:\n%s", view)
	}

	// Clear search with Esc
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	view = m.View()
	if !strings.Contains(view, "update-alpha") || !strings.Contains(view, "update-beta") {
		t.Fatalf("expected all updates back in view, got:\n%s", view)
	}
}

func TestSearchFilterAndToggling_TargetsTab(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.TargetAdd("agent-claude", "/tmp/claude")
	_ = svc.TargetAdd("agent-opencode", "/tmp/opencode")

	m := tui.NewModel(ctx, svc)

	// Switch to Tab 4 (Channels)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view := m.View()
	if !strings.Contains(view, "agent-claude") || !strings.Contains(view, "agent-opencode") {
		t.Fatalf("expected channels in view, got:\n%s", view)
	}

	// Search for "opencode"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "opencode" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	view = m.View()
	if !strings.Contains(view, "agent-opencode") {
		t.Fatalf("expected agent-opencode in view, got:\n%s", view)
	}
	if strings.Contains(view, "agent-claude") {
		t.Fatalf("expected agent-claude to be filtered out, got:\n%s", view)
	}

	// Leave search, then press 't' to toggle the selected channel enabled status
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	m = stepPump(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	// Check that agent-opencode is now disabled
	if svc.IsTargetEnabled("agent-opencode") {
		t.Fatal("expected agent-opencode to be disabled after space toggle")
	}

	// Press '/' to search for non-existent target
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "xyz999" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	view = m.View()
	if !strings.Contains(view, "No agent channels matching filter") && !strings.Contains(view, "没有符合当前条件的") {
		t.Fatalf("expected empty filtered target message, got:\n%s", view)
	}
}

func TestSearchFilter_GroupsTab(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.GroupCreate("frontend")
	_ = svc.GroupCreate("backend")

	m := tui.NewModel(ctx, svc)

	// Switch to Skills > Groups
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	m = step(m, tea.KeyMsg{Type: tea.KeyRight})
	view := m.View()
	if !strings.Contains(view, "frontend") || !strings.Contains(view, "backend") {
		t.Fatalf("expected frontend and backend in groups tab, got:\n%s", view)
	}

	// Search for "front"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "front" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	view = m.View()
	if !strings.Contains(view, "frontend") {
		t.Fatalf("expected frontend in filtered view, got:\n%s", view)
	}
	if strings.Contains(view, "backend") {
		t.Fatalf("expected backend to be filtered out, got:\n%s", view)
	}

	// Search for something non-existent
	for _, ch := range "xyz" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	view = m.View()
	if !strings.Contains(view, "未找到匹配项") && !strings.Contains(view, "No matching items found") {
		t.Fatalf("expected empty search result message, got:\n%s", view)
	}
}

func TestModalGroupSkills_SearchAndToggle(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "web-react")
	_ = svc.NewSkill(ctx, "web-vue")
	_ = svc.NewSkill(ctx, "db-postgres")
	_ = svc.GroupCreate("dev")

	m := tui.NewModel(ctx, svc)

	// Switch to Skills > Groups and press 'e' on 'dev'
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	m = step(m, tea.KeyMsg{Type: tea.KeyRight})
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	if m.GroupSkillsModalVisibleCountForTest() != 3 {
		t.Fatalf("expected 3 skills in GroupSkillsModal, got %d", m.GroupSkillsModalVisibleCountForTest())
	}

	// Press '/' to search in modal for "vue"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "vue" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	if m.GroupSkillsModalVisibleCountForTest() != 1 {
		t.Fatalf("expected 1 skill in filtered GroupSkillsModal, got %d", m.GroupSkillsModalVisibleCountForTest())
	}

	// Exit search typing
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Toggle space on the filtered item (web-vue)
	m = step(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.GroupSkillsModalIsSkillSelectedForTest("web-vue") {
		t.Fatal("expected web-vue to be selected")
	}
	if m.GroupSkillsModalIsSkillSelectedForTest("web-react") {
		t.Fatal("web-react should not be selected")
	}

	// Now search for "react"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	// Clear previous search and type react
	m = step(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = step(m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = step(m, tea.KeyMsg{Type: tea.KeyBackspace})
	for _, ch := range "react" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = step(m, tea.KeyMsg{Type: tea.KeySpace})

	// Both web-vue and web-react should be selected!
	if !m.GroupSkillsModalIsSkillSelectedForTest("web-vue") {
		t.Fatal("web-vue selection should be preserved across searches")
	}
	if !m.GroupSkillsModalIsSkillSelectedForTest("web-react") {
		t.Fatal("web-react should now be selected")
	}
	if m.GroupSkillsModalIsSkillSelectedForTest("db-postgres") {
		t.Fatal("db-postgres should not be selected")
	}
}

func TestModalAddStep1_SearchFilter(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Simulate discovery of multiple skills
	m = step(m, tui.NewDiscoveredSkillsMsgForTest("https://github.com/org/repo.git", "main", []string{
		"auth-service",
		"payment-service",
		"auth-token",
	}))

	if m.AddModalStepForTest() != 1 {
		t.Fatalf("expected step 1, got %d", m.AddModalStepForTest())
	}
	if m.AddModalVisibleCountForTest() != 3 {
		t.Fatalf("expected 3 visible skills, got %d", m.AddModalVisibleCountForTest())
	}

	// Press '/' to search for "token"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "token" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	if m.AddModalVisibleCountForTest() != 1 {
		t.Fatalf("expected 1 visible skill for 'token', got %d", m.AddModalVisibleCountForTest())
	}

	// Press Enter to exit search focus
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Initially auth-token is selected as a new skill
	if !m.AddModalIsSelectedForTest(2) {
		t.Fatal("expected auth-token (index 2) to be initially selected")
	}

	// Press Space to toggle selection off
	m = step(m, tea.KeyMsg{Type: tea.KeySpace})
	if m.AddModalIsSelectedForTest(2) {
		t.Fatal("expected auth-token (index 2) to be unselected after toggle")
	}

	// Press Space again to toggle back on
	m = step(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.AddModalIsSelectedForTest(2) {
		t.Fatal("expected auth-token (index 2) to be selected after second toggle")
	}
	if !m.AddModalIsSelectedForTest(0) {
		t.Fatal("expected auth-service (index 0) to remain selected")
	}
}

func TestDeployModal_SearchFilter(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-network")
	_ = svc.NewSkill(ctx, "skill-storage")
	_ = svc.TargetAdd("agent-remote", "/tmp/remote")
	_ = svc.TargetAdd("agent-local", "/tmp/local")

	m := newTestSkillsModel(ctx, svc)

	// Open Deploy Modal by pressing 'd'
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if m.DeployModalVisibleSkillsCountForTest() != 2 {
		t.Fatalf("expected 2 skills in DeployModal, got %d", m.DeployModalVisibleSkillsCountForTest())
	}

	// Search for "storage" in Step 0
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "storage" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	if m.DeployModalVisibleSkillsCountForTest() != 1 {
		t.Fatalf("expected 1 skill in filtered DeployModal, got %d", m.DeployModalVisibleSkillsCountForTest())
	}

	// Exit search focus
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Toggle selection on visible skill (skill-storage)
	m = step(m, tea.KeyMsg{Type: tea.KeySpace})
	if !m.DeployModalIsSkillSelectedForTest("skill-storage") {
		t.Fatal("expected skill-storage to be selected")
	}

	// Press Enter to proceed to Step 1 (SelectTarget)
	m = step(m, tea.KeyMsg{Type: tea.KeyEnter})

	// In Step 1: 2 agent targets
	if m.DeployModalVisibleAgentTargetsCountForTest() != 2 {
		t.Fatalf("expected 2 agent targets in Step 1, got %d", m.DeployModalVisibleAgentTargetsCountForTest())
	}

	// Search for "remote" in Step 1
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "remote" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	if m.DeployModalVisibleAgentTargetsCountForTest() != 1 {
		t.Fatalf("expected 1 agent target in filtered Step 1, got %d", m.DeployModalVisibleAgentTargetsCountForTest())
	}
}
