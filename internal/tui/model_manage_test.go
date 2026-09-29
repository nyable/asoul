package tui_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"asoul/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIModelsLayoutAndSummary(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	view := modelModel.View()
	agentLabel := strings.Index(view, "Agent Channel")
	summaryLabel := strings.Index(view, "SKILLS:")
	if agentLabel < 0 || summaryLabel < 0 {
		t.Fatalf("expected channel bar and compact summary, got:\n%s", view)
	}
	if agentLabel > summaryLabel {
		t.Fatalf("expected agent channel bar above the summary line, got:\n%s", view)
	}
	if !strings.Contains(view, "Data sync") {
		t.Fatalf("expected compact summary to show the sync mode, got:\n%s", view)
	}
}

func TestTUIModelsProviderCollapseAndDetail(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 60})
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	// Move up from the first model row onto the provider header, then collapse it.
	mUp, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	modelModel = mUp.(*tui.Model)
	mCollapse, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeySpace})
	modelModel = mCollapse.(*tui.Model)

	view := modelModel.View()
	if strings.Contains(view, "claude-3-5-haiku") || strings.Contains(view, "claude-3-7-sonnet") {
		t.Fatalf("expected anthropic models hidden after collapse, got:\n%s", view)
	}
	if !strings.Contains(view, "gpt-4o") {
		t.Fatalf("expected other providers still visible, got:\n%s", view)
	}

	// "i" on the header opens the provider detail.
	mDetail, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	modelModel = mDetail.(*tui.Model)
	if !modelModel.ProviderDetailOpenForTest() {
		t.Fatal("expected provider detail modal to open after i on header")
	}
	if !strings.Contains(modelModel.View(), "anthropic") {
		t.Fatalf("expected provider detail to show provider id, got:\n%s", modelModel.View())
	}

	// Open the provider field editor.
	mEdit, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	modelModel = mEdit.(*tui.Model)
	if editing, _, count := modelModel.EditFieldModalForTest(); editing || count == 0 {
		t.Fatalf("expected provider field editor with fields, got editing=%v count=%d", editing, count)
	}
	if !strings.Contains(modelModel.View(), "options.apiKey") {
		t.Fatalf("expected provider fields to include options.apiKey, got:\n%s", modelModel.View())
	}

	// Space again expands the provider.
	mEsc, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyEscape})
	modelModel = mEsc.(*tui.Model)
	mExpand, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeySpace})
	modelModel = mExpand.(*tui.Model)
	if !strings.Contains(modelModel.View(), "claude-3-5-haiku") {
		t.Fatalf("expected anthropic models restored after expand, got:\n%s", modelModel.View())
	}
}

func TestTUIModelFieldEditPreviewsAndWrites(t *testing.T) {
	ctx := context.Background()
	svc, opencodePath, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	// Open the first model detail, then the field editor.
	mEnter, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	modelModel = mEnter.(*tui.Model)
	mEdit, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	modelModel = mEdit.(*tui.Model)

	editing, cursor, fieldCount := modelModel.EditFieldModalForTest()
	if editing || cursor != 0 || fieldCount == 0 {
		t.Fatalf("expected field editor on first field, got editing=%v cursor=%d count=%d", editing, cursor, fieldCount)
	}

	// Start editing the name field and commit a new value.
	mStart, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelModel = mStart.(*tui.Model)
	if editing, _, _ := modelModel.EditFieldModalForTest(); !editing {
		t.Fatal("expected editing mode after Enter")
	}
	modelModel.SetEditFieldInputForTest("Claude Haiku Renamed")
	mCommit, cmd := modelModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelModel = mCommit.(*tui.Model)
	if cmd == nil {
		t.Fatal("expected preview command after committing a field value")
	}
	modelModel.PumpForTest(cmd)

	if !modelModel.ModelsDiffConfirmForTest() || modelModel.CurrentViewForTest() != tui.ViewDiffForTest {
		t.Fatalf("expected diff confirm view, got view=%d confirm=%v", modelModel.CurrentViewForTest(), modelModel.ModelsDiffConfirmForTest())
	}

	// Confirm the write.
	mConfirm, cmd := modelModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelModel = mConfirm.(*tui.Model)
	if cmd == nil {
		t.Fatal("expected write command on confirm")
	}
	modelModel.PumpForTest(cmd)

	data, err := os.ReadFile(opencodePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Claude Haiku Renamed") {
		t.Fatalf("expected edited name written to config, got:\n%s", data)
	}
}

func TestTUIModelsDefaultModeAndPersist(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	if !m.ModelOverrideForTest() {
		t.Fatal("expected default enrich mode to be sync upstream (override=true)")
	}

	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)
	if !strings.Contains(modelModel.View(), "Data sync (upstream)") && !strings.Contains(modelModel.View(), "数据同步（同步上游）") {
		t.Fatalf("expected mode label in view, got:\n%s", modelModel.View())
	}

	mO, cmd := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	modelModel = mO.(*tui.Model)
	if modelModel.ModelOverrideForTest() {
		t.Fatal("expected override disabled after toggling mode")
	}
	modelModel.PumpForTest(cmd)

	cfg, err := svc.Config()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetModelEnrichMode(); got != "incremental" {
		t.Fatalf("expected persisted incremental mode, got %q", got)
	}
}

func TestTUIProviderDetailRefreshesAfterEdit(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupModelTestEnvironment(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	mTab9, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	modelModel := mTab9.(*tui.Model)

	// Select the anthropic provider header and open its detail.
	mUp, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	modelModel = mUp.(*tui.Model)
	mSpace, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	modelModel = mSpace.(*tui.Model)

	// Edit the provider name.
	mEdit, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	modelModel = mEdit.(*tui.Model)
	mStart, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelModel = mStart.(*tui.Model)
	modelModel.SetEditFieldInputForTest("Anthropic Renamed")
	mCommit, cmd := modelModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelModel = mCommit.(*tui.Model)
	modelModel.PumpForTest(cmd)

	mConfirm, cmd := modelModel.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelModel = mConfirm.(*tui.Model)
	modelModel.PumpForTest(cmd)

	// Reopen the provider detail; it must reflect the new name.
	mUp2, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyUp})
	modelModel = mUp2.(*tui.Model)
	mSpace2, _ := modelModel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	modelModel = mSpace2.(*tui.Model)
	if !modelModel.ProviderDetailOpenForTest() {
		t.Fatal("expected provider detail to reopen")
	}
	if got := modelModel.ProviderDetailNameForTest(); got != "Anthropic Renamed" {
		t.Fatalf("expected refreshed provider name, got %q", got)
	}
}
