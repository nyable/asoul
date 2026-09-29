package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIModelsRulesOnlyRegenerates(t *testing.T) {
	ctx := context.Background()
	svc, opencodePath, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	rulesDir := filepath.Join(filepath.Dir(opencodePath), "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}
	ruleContent := `{
		"rules": [
			{
				"name": "rename-haiku",
				"pattern": "^claude-3-5-haiku$",
				"override": { "name": "Haiku By Rules" }
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(rulesDir, "opencode.json"), []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	m := tui.NewModel(ctx, svc)
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	// R triggers a rules-only dry run and opens the confirm diff.
	mR, cmd := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	modelModel = mR.(*tui.Model)
	if cmd == nil {
		t.Fatal("expected R to return a command")
	}
	modelModel.PumpForTest(cmd)

	if !modelModel.ModelsDiffRulesOnlyForTest() {
		t.Fatal("expected rules-only confirm mode to be active")
	}
	if modelModel.CurrentViewForTest() != tui.ViewDiffForTest {
		t.Fatalf("expected diff view after rules-only dry run, got %d", modelModel.CurrentViewForTest())
	}

	// Confirm the write.
	mEnter, cmd := modelModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelModel = mEnter.(*tui.Model)
	if cmd == nil {
		t.Fatal("expected Enter to return a write command")
	}
	modelModel.PumpForTest(cmd)

	data, err := os.ReadFile(opencodePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Haiku By Rules") {
		t.Fatalf("expected rules-only write to apply the override, got:\n%s", data)
	}
}

func TestTUIStickyProviderLabelStaysVisible(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	label := modelModel.CurrentProviderLabelForTest()
	if !strings.Contains(label, "anthropic") || !strings.Contains(label, "provider 1/2") {
		t.Fatalf("expected initial label at provider 1/2, got %q", label)
	}
	if !strings.Contains(modelModel.View(), "Current provider") {
		t.Fatalf("expected sticky provider line in view, got:\n%s", modelModel.View())
	}

	// Move into the second provider and scroll far enough that the inline
	// group headers leave the viewport.
	for i := 0; i < 3; i++ {
		mDown, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyDown})
		modelModel = mDown.(*tui.Model)
	}

	label = modelModel.CurrentProviderLabelForTest()
	if !strings.Contains(label, "openai") || !strings.Contains(label, "provider 2/2") {
		t.Fatalf("expected label to track the selected provider, got %q", label)
	}
	view := modelModel.View()
	if !strings.Contains(view, "Current provider") || !strings.Contains(view, "openai") {
		t.Fatalf("expected sticky provider line to remain visible after scrolling, got:\n%s", view)
	}
}

func TestTUIDiffChangesOnlyToggle(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	rawDiff := "--- original\n+++ updated\n@@ -1,3 +1,3 @@\n keep-me\n-drop-me\n+add-me\n"

	m := tui.NewModel(ctx, svc)
	mDiff, _ := m.Update(tui.NewDiffLoadedMsgForTest(rawDiff))
	modelModel := mDiff.(*tui.Model)

	if modelModel.CurrentViewForTest() != tui.ViewDiffForTest {
		t.Fatalf("expected diff view, got %d", modelModel.CurrentViewForTest())
	}
	if !strings.Contains(modelModel.View(), "keep-me") {
		t.Fatalf("expected full diff to show context, got:\n%s", modelModel.View())
	}
	if !strings.Contains(modelModel.View(), "│") {
		t.Fatalf("expected diff view to show line numbers, got:\n%s", modelModel.View())
	}

	mT, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	modelModel = mT.(*tui.Model)
	if !modelModel.DiffChangesOnlyForTest() {
		t.Fatal("expected changes-only mode to be enabled after t")
	}
	view := modelModel.View()
	if strings.Contains(view, "keep-me") {
		t.Fatalf("expected changes-only view to drop context lines, got:\n%s", view)
	}
	if !strings.Contains(view, "add-me") || !strings.Contains(view, "drop-me") {
		t.Fatalf("expected changes-only view to keep changed lines, got:\n%s", view)
	}
}
