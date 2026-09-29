package tui_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/i18n"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

// TestChannelsFooterReflectsCapability verifies that model-config shortcuts are
// only advertised for channels whose adapter is registered (currently opencode)
// and are hidden for unsupported channels such as "standard".
func TestChannelsFooterReflectsCapability(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		lang      string
		modelHint string
	}{
		{"en", "[e] enrich"},
		{"zh-CN", "[e] 补全"},
	}

	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			svc, wsRoot, cleanup := setupTestService(t)
			defer cleanup()
			i18n.Init(tc.lang)
			defer i18n.Init("en")

			// "opencode" has a registered adapter; "standard" does not.
			if err := svc.TargetAdd("opencode", wsRoot); err != nil {
				t.Fatal(err)
			}
			if err := svc.TargetAdd("standard", filepath.Join(wsRoot, "standard")); err != nil {
				t.Fatal(err)
			}

			var m tea.Model = tui.NewModel(ctx, svc)
			m, _ = m.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

			// Channels sort by name, so opencode (supported) is selected first.
			if view := m.View(); !strings.Contains(view, tc.modelHint) {
				t.Fatalf("supported channel footer should show %q, got:\n%s", tc.modelHint, view)
			}

			// Enter the channel layer and move to the unsupported channel.
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
			m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

			if view := m.View(); strings.Contains(view, tc.modelHint) {
				t.Fatalf("unsupported channel footer should not show %q, got:\n%s", tc.modelHint, view)
			}
		})
	}
}
