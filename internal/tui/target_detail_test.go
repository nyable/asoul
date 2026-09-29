package tui_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"asoul/internal/model"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestTUITargetDetailModal_FixedWidthAndWrapping(t *testing.T) {
	var skills []string
	for i := 1; i <= 25; i++ {
		skills = append(skills, fmt.Sprintf("long-named-deployed-skill-item-%02d", i))
	}

	state := tui.NewTargetDetailModalForTest(
		"test-agent",
		"agent",
		"global",
		"/home/user/.config/test-agent",
		"/home/user/.config/test-agent/resolved",
		"/home/user/.config/test-agent/skills",
		map[string]string{
			"skills":   "${config_dir}/skills",
			"prompts":  "${config_dir}/prompts",
			"contexts": "${config_dir}/contexts",
		},
		true,
		true,
		"",
		skills,
		true,
	)

	// 1. Standard terminal (80 cols, 40 rows): modal outer width should be max 76 cols
	renderedWide := tui.RenderTargetDetailModalForTest(state, 80, 40)
	wideWidth := lipgloss.Width(renderedWide)
	if wideWidth != 76 {
		t.Fatalf("expected fixed modal width 76 in standard terminal (80 cols), got %d", wideWidth)
	}

	for idx, line := range strings.Split(renderedWide, "\n") {
		if line == "" {
			continue
		}
		lineWidth := lipgloss.Width(line)
		if lineWidth != 76 {
			t.Errorf("wide line %d has width %d, expected 76:\n%s", idx, lineWidth, line)
		}
	}

	// 2. Narrow terminal (56 cols, 40 rows): modal outer width should adapt to 52 (56 - 4)
	renderedNarrow := tui.RenderTargetDetailModalForTest(state, 56, 40)
	narrowWidth := lipgloss.Width(renderedNarrow)
	if narrowWidth != 52 {
		t.Fatalf("expected adaptive modal width 52 in narrow terminal (56 cols), got %d", narrowWidth)
	}

	for idx, line := range strings.Split(renderedNarrow, "\n") {
		if line == "" {
			continue
		}
		lineWidth := lipgloss.Width(line)
		if lineWidth != 52 {
			t.Errorf("narrow line %d has width %d, expected 52:\n%s", idx, lineWidth, line)
		}
	}

	// 3. Verify deployed skills list is wrapped with indentation
	if !strings.Contains(renderedWide, "long-named-deployed-skill-item-") {
		t.Fatalf("expected deployed skills in rendered modal")
	}
}

func TestTUITargetDetailModal_HeightAndScrolling(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	// Register a target with many deployed skills
	var deployedSkills []string
	for i := 1; i <= 30; i++ {
		deployedSkills = append(deployedSkills, fmt.Sprintf("deployed-skill-%02d", i))
	}
	_ = svc.TargetAddConfig("claude-desktop", model.TargetConfig{
		Type:      model.TargetTypeAgent,
		ConfigDir: "~/.config/claude-desktop",
		Paths: map[string]string{
			"skills":   "${config_dir}/skills",
			"prompts":  "${config_dir}/prompts",
			"snippets": "${config_dir}/snippets",
		},
	})

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Switch to Tab 4 (Channels)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	if m.ActiveTabForTest() != tui.TabChannelsForTest {
		t.Fatalf("expected active tab Channels, got %d", m.ActiveTabForTest())
	}

	// Press 'I' to open channel target details
	m = stepPump(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	if m.CurrentViewForTest() != tui.ViewModalTargetDetailForTest {
		t.Fatalf("expected view ViewModalTargetDetail, got %d", m.CurrentViewForTest())
	}

	// Inject many deployed skills to state so that content exceeds viewport
	detailState, ok := m.TargetDetailModalForTest()
	if !ok {
		t.Fatal("expected targetDetailModal to be open")
	}
	detailState.DeployedSkills = deployedSkills

	// Verify rendered modal height <= termHeight - 3 (21 rows)
	renderedModal := tui.RenderTargetDetailModalForTest(detailState, 80, 24)
	modalH := lipgloss.Height(renderedModal)
	if modalH > 21 {
		t.Fatalf("expected modal height <= 21 in 24-row terminal, got %d", modalH)
	}

	// Total view output must NOT exceed terminal height (24 rows)
	view := m.View()
	viewLines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(viewLines) > 24 {
		t.Fatalf("expected total view height <= 24 rows, got %d lines", len(viewLines))
	}

	// Window header and modal title must be intact
	if !strings.Contains(view, "Channels") && !strings.Contains(view, "渠道") {
		t.Fatalf("tabs bar overflowed off top of window:\n%s", view)
	}
	if !strings.Contains(view, "🎯") {
		t.Fatalf("modal title missing:\n%s", view)
	}

	// Test scrolling navigation
	if m.TargetDetailModalViewportOffsetForTest() != 0 {
		t.Fatalf("expected initial viewport offset 0, got %d", m.TargetDetailModalViewportOffsetForTest())
	}

	// Down key scrolls by 1 line
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.TargetDetailModalViewportOffsetForTest() != 1 {
		t.Fatalf("expected viewport offset 1 after Down, got %d", m.TargetDetailModalViewportOffsetForTest())
	}

	// 'j' scrolls down another line
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.TargetDetailModalViewportOffsetForTest() != 2 {
		t.Fatalf("expected viewport offset 2 after 'j', got %d", m.TargetDetailModalViewportOffsetForTest())
	}

	// Up key scrolls back up
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.TargetDetailModalViewportOffsetForTest() != 1 {
		t.Fatalf("expected viewport offset 1 after Up, got %d", m.TargetDetailModalViewportOffsetForTest())
	}

	// 'k' scrolls back up to 0
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.TargetDetailModalViewportOffsetForTest() != 0 {
		t.Fatalf("expected viewport offset 0 after 'k', got %d", m.TargetDetailModalViewportOffsetForTest())
	}

	// 'G' (end) jumps to bottom
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	bottomOffset := m.TargetDetailModalViewportOffsetForTest()
	if bottomOffset <= 0 {
		t.Fatalf("expected positive viewport offset after 'G', got %d", bottomOffset)
	}

	// 'g' (home) jumps to top
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.TargetDetailModalViewportOffsetForTest() != 0 {
		t.Fatalf("expected viewport offset 0 after 'g', got %d", m.TargetDetailModalViewportOffsetForTest())
	}

	// PgDown scrolls half view
	m = step(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.TargetDetailModalViewportOffsetForTest() <= 0 {
		t.Fatalf("expected positive viewport offset after PgDown, got %d", m.TargetDetailModalViewportOffsetForTest())
	}

	// Esc closes modal
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to ViewList after Esc, got %d", m.CurrentViewForTest())
	}
}
