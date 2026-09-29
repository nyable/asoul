package tui_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/deploy"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func TestDeployConflictModalFlow(t *testing.T) {
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	skillID := "pdf"
	if err := svc.NewSkill(ctx, skillID); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	projRoot := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DeployProject(ctx, skillID, projRoot, []string{"standard"}, false); err != nil {
		t.Fatal(err)
	}

	targetSkillDir := filepath.Join(projRoot, ".agents", "skills", skillID)
	modifiedContent := "---\nname: pdf\ndescription: edited in target\n---\n# Target Edit\n"
	if err := os.WriteFile(filepath.Join(targetSkillDir, "SKILL.md"), []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	var m tea.Model = tui.NewModel(ctx, svc)

	// 1. Trigger conflict modal via conflict message
	conflict := &deploy.TargetConflictError{
		SkillID:     skillID,
		Target:      projRoot,
		Format:      "standard",
		TargetDir:   targetSkillDir,
		RelDest:     ".agents/skills/pdf",
		IsUntracked: false,
		IsProject:   true,
	}
	m, _ = m.Update(tui.NewDeployConflictMsgForTest(conflict))

	modelObj := m.(*tui.Model)
	if modelObj.CurrentViewForTest() != tui.ViewModalDeployConflictForTest {
		t.Fatalf("expected ViewModalDeployConflict, got %d", modelObj.CurrentViewForTest())
	}

	rendered := m.View()
	for _, expected := range []string{"pdf", "[v]", "[f]", "[b]", "[Esc]"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered modal missing %q:\n%s", expected, rendered)
		}
	}

	// 2. Press 'v' to view diff
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if cmd == nil {
		t.Fatal("expected diff command")
	}
	diffMsg := cmd()
	m, _ = m.Update(diffMsg)

	if modelObj.CurrentViewForTest() != tui.ViewTargetDiffForTest {
		t.Fatalf("expected ViewTargetDiff, got %d", modelObj.CurrentViewForTest())
	}

	diffView := m.View()
	if !strings.Contains(diffView, "edited in target") {
		t.Fatalf("expected diff view to contain 'edited in target', got:\n%s", diffView)
	}
	if !strings.Contains(diffView, "Unified Diff") {
		t.Fatalf("expected diff view to contain 'Unified Diff', got:\n%s", diffView)
	}
	if !strings.Contains(diffView, "变更概览") && !strings.Contains(diffView, "Diff Summary") {
		t.Fatalf("expected diff view to contain summary title, got:\n%s", diffView)
	}

	// 3. Press Esc in diff view -> must return to conflict modal
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if modelObj.CurrentViewForTest() != tui.ViewModalDeployConflictForTest {
		t.Fatalf("expected Esc from diff view to return to ViewModalDeployConflict, got %d", modelObj.CurrentViewForTest())
	}

	// 4. Press 'b' to adopt modifications into workspace
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if cmd == nil {
		t.Fatal("expected adopt command")
	}
	adoptMsg := cmd()
	m, _ = m.Update(adoptMsg)

	if modelObj.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected return to ViewList after adopt, got %d", modelObj.CurrentViewForTest())
	}

	// Verify workspace now has the target changes
	wsSkillDir := catalog.SkillDir(wsRoot, skillID)
	wsContent, err := os.ReadFile(filepath.Join(wsSkillDir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(wsContent) != modifiedContent {
		t.Fatalf("workspace content was not adopted, got:\n%s", string(wsContent))
	}
}

func TestDeployConflictModalForceDeploy(t *testing.T) {
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	skillID := "pdf"
	if err := svc.NewSkill(ctx, skillID); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	projRoot := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DeployProject(ctx, skillID, projRoot, []string{"standard"}, false); err != nil {
		t.Fatal(err)
	}

	targetSkillDir := filepath.Join(projRoot, ".agents", "skills", skillID)
	modifiedContent := "---\nname: pdf\ndescription: target was modified\n---\n"
	if err := os.WriteFile(filepath.Join(targetSkillDir, "SKILL.md"), []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	var m tea.Model = tui.NewModel(ctx, svc)
	conflict := &deploy.TargetConflictError{
		SkillID:     skillID,
		Target:      projRoot,
		Format:      "standard",
		TargetDir:   targetSkillDir,
		RelDest:     ".agents/skills/pdf",
		IsUntracked: false,
		IsProject:   true,
	}
	m, _ = m.Update(tui.NewDeployConflictMsgForTest(conflict))

	modelObj := m.(*tui.Model)
	if modelObj.CurrentViewForTest() != tui.ViewModalDeployConflictForTest {
		t.Fatalf("expected ViewModalDeployConflict, got %d", modelObj.CurrentViewForTest())
	}

	// Press 'f' to force deploy
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if cmd == nil {
		t.Fatal("expected force deploy command")
	}
	forceMsg := cmd()
	m, _ = m.Update(forceMsg)

	if modelObj.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected return to ViewList after force deploy, got %d", modelObj.CurrentViewForTest())
	}

	// Target should now be overwritten with workspace version
	wsSkillDir := catalog.SkillDir(wsRoot, skillID)
	wsContent, _ := os.ReadFile(filepath.Join(wsSkillDir, "SKILL.md"))
	targetContent, _ := os.ReadFile(filepath.Join(targetSkillDir, "SKILL.md"))
	if string(targetContent) != string(wsContent) {
		t.Fatalf("target was not overwritten with workspace content:\ntarget: %s\nws: %s", string(targetContent), string(wsContent))
	}
}

func TestDeployExecutionSingleConflictOpensModal(t *testing.T) {
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	skillID := "pdf"
	if err := svc.NewSkill(ctx, skillID); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	projRoot := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.DeployProject(ctx, skillID, projRoot, []string{"standard"}, false); err != nil {
		t.Fatal(err)
	}

	targetSkillDir := filepath.Join(projRoot, ".agents", "skills", skillID)
	modifiedContent := "---\nname: pdf\ndescription: user modified\n---\n"
	if err := os.WriteFile(filepath.Join(targetSkillDir, "SKILL.md"), []byte(modifiedContent), 0644); err != nil {
		t.Fatal(err)
	}

	var m tea.Model = newTestSkillsModel(ctx, svc)
	// Open deploy modal for pdf with 'd'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// In skill selection step, press Enter to proceed to target selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Switch to project tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Add the project path
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(projRoot)})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Format selection step: press Enter to execute deploy
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected deploy execution command")
	}

	// Run the deploy execution command
	msg := cmd()
	if msg == nil {
		t.Fatal("expected message from deploy command")
	}

	// Update model with the result
	m, _ = m.Update(msg)

	modelObj := m.(*tui.Model)
	if modelObj.CurrentViewForTest() != tui.ViewModalDeployConflictForTest {
		t.Fatalf("expected deploy conflict to automatically open ViewModalDeployConflict, got view=%d", modelObj.CurrentViewForTest())
	}
}

func TestBatchDeployNoticeContainsFailureReason(t *testing.T) {
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	ctx := context.Background()
	_ = svc.NewSkill(ctx, "pdf")
	_ = svc.NewSkill(ctx, "helper")

	tmpDir := t.TempDir()
	projRoot := filepath.Join(tmpDir, "project")
	_ = os.MkdirAll(projRoot, 0755)
	_, _, _ = svc.DeployProject(ctx, "pdf", projRoot, []string{"standard"}, false)

	// Modify pdf in target
	targetPdfDir := filepath.Join(projRoot, ".agents", "skills", "pdf")
	_ = os.WriteFile(filepath.Join(targetPdfDir, "SKILL.md"), []byte("modified"), 0644)

	var m tea.Model = newTestSkillsModel(ctx, svc)
	// Open deploy modal
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Select both skills using space on second item
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Switch to project tab
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(projRoot)})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Press Enter to execute batch deploy
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected deploy command")
	}
	msg := cmd()
	m, _ = m.Update(msg)

	rendered := m.View()
	modelObj := m.(*tui.Model)
	if modelObj.CurrentViewForTest() != tui.ViewModalDeployConflictForTest {
		t.Fatalf("expected batch conflict to automatically open ViewModalDeployConflict, got view=%d", modelObj.CurrentViewForTest())
	}
	if !strings.Contains(rendered, "1/2") || !strings.Contains(rendered, "pdf") {
		t.Fatalf("expected batch conflict modal with progress and pdf, got:\n%s", rendered)
	}

	// Press Esc to skip conflicting skill
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if modelObj.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected Esc to return to ViewList after skipping, got view=%d", modelObj.CurrentViewForTest())
	}
	if !strings.Contains(m.View(), "Deployed 1 skills") {
		t.Fatalf("expected notice about successful partial deployment, got:\n%s", m.View())
	}
}
