package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEscCancelsPendingNewSkill(t *testing.T) {
	svc, workspaceRoot, cleanup := setupTestService(t)
	defer cleanup()

	var m tea.Model = newTestSkillsModel(context.Background(), svc)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("cancel-skill")})
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected new-skill command")
	}

	// Cancel before the command is allowed to run. Executing the already queued
	// command must not create a skill or publish a success message.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if msg := cmd(); msg != nil {
		m, _ = m.Update(msg)
	}

	if _, err := os.Stat(filepath.Join(workspaceRoot, "skills", "cancel-skill")); !os.IsNotExist(err) {
		t.Fatalf("canceled command created a skill; stat error = %v", err)
	}
}
