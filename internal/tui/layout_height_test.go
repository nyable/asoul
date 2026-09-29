package tui_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"asoul/internal/app"
	"asoul/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

var ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func viewLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// assertTabBarPinned asserts the fixed chrome: the tab bar is the first line
// and the view never exceeds the terminal height.
func assertTabBarPinned(t *testing.T, m *tui.Model, height int) {
	t.Helper()
	lines := viewLines(m.View())
	if len(lines) > height {
		t.Fatalf("view height %d exceeds terminal height %d:\n%s", len(lines), height, m.View())
	}
	if len(lines) == 0 || !strings.Contains(lines[0], "1. Overview") {
		t.Fatalf("expected the first line to be the fixed tab bar, got:\n%s", m.View())
	}
}

func TestBaseLayoutPinsTabBar(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()
	_ = svc.TargetAdd("agent-ch", "/tmp/agent-ch")

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 130, Height: 12})

	assertTabBarPinned(t, step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}}), 12)
	assertTabBarPinned(t, step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}}), 12)
	assertTabBarPinned(t, step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}}), 12)
	assertTabBarPinned(t, step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}}), 12)

	// Skills with the search bar active (extra fixed line).
	sk := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	sk = step(sk, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	assertTabBarPinned(t, sk, 12)
}

func TestChannelsSelectedRowVisibleAtSmallHeight(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 130, Height: 26})
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

	// The row cursor starts on the first model row; one Down selects the next.
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})

	view := m.View()
	assertTabBarPinned(t, m, 26)
	plain := stripANSI(view)
	if !strings.Contains(plain, "> claude-3-7-sonnet") {
		t.Fatalf("expected the selected model row to be visible, got:\n%s", plain)
	}
}

func TestDoctorScrolls(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 130, Height: 14})

	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})
	m = step(m, tea.KeyMsg{Type: tea.KeyTab})
	m = stepPump(t, m, tea.KeyMsg{Type: tea.KeyRight})

	items := make([]app.CheckItem, 40)
	for i := range items {
		items[i] = app.CheckItem{
			Status:  app.CheckOK,
			Name:    fmt.Sprintf("Check-%02d", i),
			Message: "ok",
		}
	}
	m.SetDoctorReportForTest(items)
	assertTabBarPinned(t, m, 14)

	// Jump to the last check; it must be visible with the tab bar pinned.
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if got := m.DoctorCursorForTest(); got != len(items)-1 {
		t.Fatalf("expected doctor cursor at the last item, got %d", got)
	}
	view := m.View()
	if !strings.Contains(view, "Check-39") {
		t.Fatalf("expected the last check visible after G, got:\n%s", view)
	}
	assertTabBarPinned(t, m, 14)

	// Move up one row.
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if got := m.DoctorCursorForTest(); got != len(items)-2 {
		t.Fatalf("expected the cursor to move up, got %d", got)
	}
}
