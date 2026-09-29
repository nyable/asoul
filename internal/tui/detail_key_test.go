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

// TestDetailKeyProjects verifies Projects uses the unified detail key:
// "i" opens the project detail, while Enter/v no longer do.
func TestDetailKeyProjects(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	projDir := filepath.Join(filepath.Dir(wsRoot), "detail-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectAdd(projDir); err != nil {
		t.Fatal(err)
	}

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	mProj := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})

	if got := step(mProj, tea.KeyMsg{Type: tea.KeyEnter}); got.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("Enter must not open project detail on the main list, got %d", got.CurrentViewForTest())
	}
	if got := step(mProj, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}); got.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("v must not open project detail on the main list, got %d", got.CurrentViewForTest())
	}

	mDetail := stepPump(t, mProj, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if mDetail.CurrentViewForTest() != tui.ViewModalTargetDetailForTest {
		t.Fatalf("i must open project detail, got %d", mDetail.CurrentViewForTest())
	}
	if state, ok := mDetail.TargetDetailModalForTest(); !ok || state.Type != "project" {
		t.Fatalf("expected project detail modal, got ok=%v type=%q", ok, state.Type)
	}
}

// TestDetailKeySkillsEnterNoOp verifies the Skills page only opens details via
// "i"; Enter is a strict no-op on the main list.
func TestDetailKeySkillsEnterNoOp(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "detail-skill")

	m := newTestSkillsModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if got := step(m, tea.KeyMsg{Type: tea.KeyEnter}); strings.Contains(got.View(), "SKILL.md") {
		t.Fatalf("Enter must not open skill detail on the main list")
	}

	mDetail := stepPump(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if !strings.Contains(mDetail.View(), "SKILL.md") {
		t.Fatalf("i must open skill detail with SKILL.md preview")
	}
}

// TestDetailKeySettings verifies Settings reserves "i" for nothing (no detail
// object) and uses "I" to initialize the workspace.
func TestDetailKeySettings(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	mSet := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})

	before := mSet.NoticeForTest()
	afterI := step(mSet, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if afterI.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("i on Settings must be a no-op, got view %d", afterI.CurrentViewForTest())
	}
	if afterI.NoticeForTest() != before {
		t.Fatalf("i on Settings must not trigger init, notice changed from %q to %q", before, afterI.NoticeForTest())
	}

	afterInit := step(mSet, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	if view := afterInit.View(); !strings.Contains(view, "already") && !strings.Contains(view, "无需") {
		t.Fatalf("I on Settings must initialize the workspace, got:\n%s", view)
	}
}
