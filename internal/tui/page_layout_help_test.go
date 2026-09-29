package tui_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

// TestAllPageFootersExposeHelp verifies every main page footer advertises the
// per-page help entry point.
func TestAllPageFootersExposeHelp(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	var m tea.Model = tui.NewModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})

	for _, key := range []rune{'1', '2', '3', '4', '5', '6', '7'} {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		if view := m.View(); !strings.Contains(view, "[?] help") {
			t.Fatalf("tab %c footer should expose [?] help, got:\n%s", key, view)
		}
	}
}

// TestChannelsCompactSummary verifies the slimmed-down Channels page: inline
// status filter, one-line summary and no legacy deployment/model section header.
func TestChannelsCompactSummary(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	// "opencode" has a registered adapter (data sync); "standard" does not.
	if err := svc.TargetAdd("opencode", wsRoot); err != nil {
		t.Fatal(err)
	}
	if err := svc.TargetAdd("standard", filepath.Join(wsRoot, "std")); err != nil {
		t.Fatal(err)
	}

	var m tea.Model = tui.NewModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

	view := m.View()
	if !strings.Contains(view, "Agent Channel ([f]") {
		t.Fatalf("expected inline status filter in the channel label, got:\n%s", view)
	}
	if !strings.Contains(view, "SKILLS:") {
		t.Fatalf("expected compact summary with SKILLS count, got:\n%s", view)
	}
	if !strings.Contains(view, "Data sync (") {
		t.Fatalf("supported channel should show the data sync badge, got:\n%s", view)
	}
	if strings.Contains(view, "Deployment") || strings.Contains(view, "not supported for this channel") {
		t.Fatalf("legacy deployment block should be removed, got:\n%s", view)
	}

	// Switch to the unsupported channel: no sync badge and no model table.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	unsupported := m.View()
	if strings.Contains(unsupported, "Data sync (") {
		t.Fatalf("unsupported channel should not show a data sync badge, got:\n%s", unsupported)
	}
	if strings.Contains(unsupported, "MODEL ID") {
		t.Fatalf("unsupported channel should not render the model table, got:\n%s", unsupported)
	}
}

// TestHelpModalPageSpecificAndScrollable verifies the help overlay shows the
// active page's shortcuts and can be scrolled.
func TestHelpModalPageSpecificAndScrollable(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	var m tea.Model = tui.NewModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Open help from the Channels tab.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	view := m.View()
	if !strings.Contains(view, "Keyboard Shortcuts") || !strings.Contains(view, "Channels") {
		t.Fatalf("expected page-specific help title for Channels, got:\n%s", view)
	}

	// The page-specific list lives below the shared navigation block; scroll to
	// the bottom to reveal it.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if !strings.Contains(m.View(), "View the model config file") {
		t.Fatalf("expected channel help content after scrolling, got:\n%s", m.View())
	}

	// Close with Esc.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(m.View(), "Keyboard Shortcuts") {
		t.Fatalf("expected help overlay to close on Esc")
	}
}
