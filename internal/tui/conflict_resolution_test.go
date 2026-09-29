package tui_test

import (
	"context"
	"strings"
	"testing"

	"asoul/internal/model"
	"asoul/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUI_DiscoveredSkillsConflictDetectionAndResolution(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	// 1. Setup existing skills in workspace:
	// - "skill-same-repo" from https://github.com/microsoft/skills
	// - "skill-other-repo" from https://github.com/other/skills
	_ = svc.NewSkill(ctx, "skill-other-repo")
	_ = svc.NewSkill(ctx, "skill-same-repo")

	m := tui.NewModel(ctx, svc)

	// Refresh model statuses with source info
	statusMsg := tui.NewReloadStatusMsgForTest([]model.SkillStatus{
		{
			ID: "skill-same-repo",
			Source: model.SourceSpec{
				Type: model.SourceTypeGit,
				URL:  "https://github.com/microsoft/skills",
				Path: "skills/skill-same-repo",
			},
			ManagedStatus: model.StatusClean,
		},
		{
			ID: "skill-other-repo",
			Source: model.SourceSpec{
				Type: model.SourceTypeGit,
				URL:  "https://github.com/other/skills",
				Path: "skills/skill-other-repo",
			},
			ManagedStatus: model.StatusClean,
		},
	})
	newM, _ := m.Update(statusMsg)
	m = newM.(*tui.Model)

	// Send discovered skills from https://github.com/microsoft/skills:
	// [0] "skill-new" (brand new)
	// [1] "skill-same-repo" (already installed from this repo)
	// [2] "skill-other-repo" (conflict from other repo)
	// [3] "skill-new" (intra-batch duplicate)
	discMsg := tui.NewDiscoveredSkillsMsgForTest("https://github.com/microsoft/skills", "main", []string{
		"skill-new",
		"skill-same-repo",
		"skill-other-repo",
		"skill-new",
	})
	newM, _ = m.Update(discMsg)
	m = newM.(*tui.Model)

	// Verify Step is 1
	if m.AddModalStepForTest() != 1 {
		t.Fatalf("expected AddModal step 1, got %d", m.AddModalStepForTest())
	}

	// Because conflicts exist, filter should automatically default to AddFilterConflict
	if m.AddModalFilterForTest() != tui.AddFilterConflictForTest {
		t.Fatalf("expected initial filter to be AddFilterConflict, got %d", m.AddModalFilterForTest())
	}
	if m.AddModalVisibleCountForTest() != 2 {
		t.Fatalf("expected 2 visible items in conflict view, got %d", m.AddModalVisibleCountForTest())
	}

	// 2. Test switching filters:
	// Switch to 1 (All)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = newM.(*tui.Model)
	if m.AddModalFilterForTest() != tui.AddFilterAllForTest {
		t.Fatalf("expected filter All, got %d", m.AddModalFilterForTest())
	}
	if m.AddModalVisibleCountForTest() != 4 {
		t.Fatalf("expected 4 visible items in All view, got %d", m.AddModalVisibleCountForTest())
	}

	// Switch to 3 (New Only)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = newM.(*tui.Model)
	if m.AddModalFilterForTest() != tui.AddFilterNewForTest {
		t.Fatalf("expected filter New, got %d", m.AddModalFilterForTest())
	}
	if m.AddModalVisibleCountForTest() != 1 {
		t.Fatalf("expected 1 visible item in New view, got %d", m.AddModalVisibleCountForTest())
	}

	// Switch to 4 (Installed / Identical)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m = newM.(*tui.Model)
	if m.AddModalFilterForTest() != tui.AddFilterInstalledForTest {
		t.Fatalf("expected filter Installed, got %d", m.AddModalFilterForTest())
	}
	if m.AddModalVisibleCountForTest() != 1 {
		t.Fatalf("expected 1 visible item in Installed view, got %d", m.AddModalVisibleCountForTest())
	}

	// Right to cycle back to All
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = newM.(*tui.Model)
	if m.AddModalFilterForTest() != tui.AddFilterAllForTest {
		t.Fatalf("expected cycle to All, got %d", m.AddModalFilterForTest())
	}

	// 3. Test Select All in current view [a]
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = newM.(*tui.Model)
	// Now all 4 should be selected
	if !m.AddModalIsSelectedForTest(1) || !m.AddModalIsSelectedForTest(2) {
		t.Errorf("expected all selected after 'a'")
	}

	// 4. Test Pre-flight collision prevention
	// Now item 2 (conflict) is selected, but not aliased and not marked for overwrite
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(*tui.Model)
	if cmd != nil {
		t.Errorf("expected no background cmd when pre-flight fails")
	}
	if !strings.Contains(m.AddModalErrorForTest(), "skill-other-repo") {
		t.Fatalf("expected conflict error, got %q", m.AddModalErrorForTest())
	}

	// 5. Switch to Conflict view (2)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = newM.(*tui.Model)
	// Cursor is at 0 in conflict view, which corresponds to item 2 ("skill-other-repo")
	// Test Targeted Overwrite [o]
	path2 := "skills/skill-other-repo"
	if m.AddModalIsOverwriteForTest(path2) {
		t.Fatalf("expected overwrite to be false initially")
	}
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = newM.(*tui.Model)
	if !m.AddModalIsOverwriteForTest(path2) {
		t.Fatalf("expected overwrite to be true after pressing 'o'")
	}
	if !m.AddModalIsSelectedForTest(2) {
		t.Fatalf("expected item 2 to be selected when marked for overwrite")
	}

	// Press 'o' again to toggle off
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = newM.(*tui.Model)
	if m.AddModalIsOverwriteForTest(path2) {
		t.Fatalf("expected overwrite to be false after toggling 'o' again")
	}

	// 6. Test Inline Rename [e] on item 2
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = newM.(*tui.Model)
	if !m.AddModalRenamingForTest() {
		t.Fatalf("expected Renaming=true after pressing 'e'")
	}

	// Press Esc to cancel
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = newM.(*tui.Model)
	if m.AddModalRenamingForTest() {
		t.Fatalf("expected Renaming=false after pressing Esc")
	}

	// Press 'e' again
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = newM.(*tui.Model)
	// Backspace and type valid new name "ms-other-repo"
	for i := 0; i < 30; i++ {
		newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = newM.(*tui.Model)
	}
	for _, ch := range "ms-other-repo" {
		newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = newM.(*tui.Model)
	}
	// Press Enter to confirm rename
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(*tui.Model)
	if m.AddModalRenamingForTest() {
		t.Fatalf("expected Renaming=false after confirming valid name")
	}
	alias := m.AddModalCustomAliasForTest(path2)
	if alias != "ms-other-repo" {
		t.Fatalf("expected alias 'ms-other-repo', got %q", alias)
	}

	// 7. For the other conflict item (item 3, cursor 1 in conflict view), mark with [o] for targeted overwrite!
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = newM.(*tui.Model)
	// Press 'o' on item 3
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = newM.(*tui.Model)
	if !m.AddModalIsOverwriteForTest("skills/skill-new") {
		t.Fatalf("expected skills/skill-new to be marked for overwrite")
	}

	// 8. Now all selected conflicts are resolved (item 2 is aliased, item 3 is targeted for overwrite)
	// Press Enter -> pre-flight check should pass and schedule tea.Cmd!
	newM, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(*tui.Model)
	if m.AddModalErrorForTest() != "" {
		t.Fatalf("expected no pre-flight error, got: %s", m.AddModalErrorForTest())
	}
	if cmd == nil {
		t.Fatalf("expected tea.Cmd to be scheduled for import")
	}
}

func TestTUI_BatchOverwriteCheckedItems(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "conflict-1")
	_ = svc.NewSkill(ctx, "conflict-2")
	_ = svc.NewSkill(ctx, "conflict-3")

	m := tui.NewModel(ctx, svc)

	statusMsg := tui.NewReloadStatusMsgForTest([]model.SkillStatus{
		{ID: "conflict-1", Source: model.SourceSpec{Type: model.SourceTypeGit, URL: "https://github.com/repoA/skills"}},
		{ID: "conflict-2", Source: model.SourceSpec{Type: model.SourceTypeGit, URL: "https://github.com/repoA/skills"}},
		{ID: "conflict-3", Source: model.SourceSpec{Type: model.SourceTypeGit, URL: "https://github.com/repoA/skills"}},
	})
	newM, _ := m.Update(statusMsg)
	m = newM.(*tui.Model)

	discMsg := tui.NewDiscoveredSkillsMsgForTest("https://github.com/repoB/skills", "main", []string{
		"conflict-1",
		"conflict-2",
		"conflict-3",
		"new-skill",
	})
	newM, _ = m.Update(discMsg)
	m = newM.(*tui.Model)

	// In Conflict view, 3 conflict items are visible
	if m.AddModalFilterForTest() != tui.AddFilterConflictForTest {
		t.Fatalf("expected Conflict filter initially")
	}
	if m.AddModalVisibleCountForTest() != 3 {
		t.Fatalf("expected 3 conflict items, got %d", m.AddModalVisibleCountForTest())
	}

	// None are checked initially
	for i := 0; i < 3; i++ {
		if m.AddModalIsSelectedForTest(i) {
			t.Errorf("expected conflict %d to be unchecked initially", i)
		}
	}

	// Check item 0 and item 1 using Space
	// Cursor is at 0
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // check item 0
	m = newM.(*tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // move to 1
	m = newM.(*tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // check item 1
	m = newM.(*tui.Model)

	if !m.AddModalIsSelectedForTest(0) || !m.AddModalIsSelectedForTest(1) || m.AddModalIsSelectedForTest(2) {
		t.Fatalf("expected items 0 and 1 checked, 2 unchecked")
	}

	// Press 'o' -> should batch mark both checked items (0 and 1) for overwrite!
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = newM.(*tui.Model)

	if !m.AddModalIsOverwriteForTest("skills/conflict-1") {
		t.Errorf("expected conflict-1 to be marked for overwrite")
	}
	if !m.AddModalIsOverwriteForTest("skills/conflict-2") {
		t.Errorf("expected conflict-2 to be marked for overwrite")
	}
	if m.AddModalIsOverwriteForTest("skills/conflict-3") {
		t.Errorf("expected conflict-3 to NOT be marked for overwrite")
	}

	// Press 'o' again -> should batch toggle off overwrite for both checked items
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = newM.(*tui.Model)

	if m.AddModalIsOverwriteForTest("skills/conflict-1") || m.AddModalIsOverwriteForTest("skills/conflict-2") {
		t.Errorf("expected overwrite to be toggled off for both items")
	}

	// Press 'a' in Conflict view -> selects all 3 conflict items!
	// (Since 0 and 1 are selected, 'a' selects all or deselects all. Let's make all selected.)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown}) // move to 2
	m = newM.(*tui.Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace}) // check item 2 as well
	m = newM.(*tui.Model)

	// Now all 3 conflict items are checked. Press 'o':
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	m = newM.(*tui.Model)

	if !m.AddModalIsOverwriteForTest("skills/conflict-1") ||
		!m.AddModalIsOverwriteForTest("skills/conflict-2") ||
		!m.AddModalIsOverwriteForTest("skills/conflict-3") {
		t.Errorf("expected all 3 conflict items to be marked for overwrite")
	}

	// Press Enter -> pre-flight check should pass since all selected conflicts are marked for overwrite!
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(*tui.Model)
	if m.AddModalErrorForTest() != "" {
		t.Fatalf("expected no pre-flight error, got: %s", m.AddModalErrorForTest())
	}
	if cmd == nil {
		t.Fatalf("expected tea.Cmd for import")
	}
}
