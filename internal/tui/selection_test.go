package tui_test

import (
	"context"
	"strings"
	"testing"

	"asoul/internal/model"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUpdatesActionUsesRenderedSkill(t *testing.T) {
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	var m tea.Model = tui.NewModel(context.Background(), svc)
	m, _ = m.Update(tui.NewReloadStatusMsgForTest([]model.SkillStatus{
		{ID: "alpha", Upstream: model.UpstreamUpToDate},
		{ID: "beta", Upstream: model.UpstreamUpdateAvailable},
	}))
	// Tab 1 (Dashboard) shows pending updates
	view := m.View()
	if !strings.Contains(view, "beta") {
		t.Fatalf("dashboard pending updates did not contain beta: %s", view)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if cmd == nil {
		t.Fatal("expected update command")
	}
	if !strings.Contains(updated.View(), "批量更新") && !strings.Contains(updated.View(), "Updating") {
		t.Fatalf("expected update notice on dashboard: %s", updated.View())
	}
}
