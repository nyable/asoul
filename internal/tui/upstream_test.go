package tui_test

import (
	"context"
	"strings"
	"testing"

	"asoul/internal/model"
	"asoul/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestTUIUpstreams_TabNavigation(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// 1. Jump to Tab 2 (Upstreams)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if m.ActiveTabForTest() != tui.TabUpstreamsForTest {
		t.Fatalf("expected active tab to be Upstreams (%d), got %d", tui.TabUpstreamsForTest, m.ActiveTabForTest())
	}

	// View should contain Upstream header / empty notice
	view := m.View()
	if !strings.Contains(view, "2. Sources") && !strings.Contains(view, "2. 来源") {
		t.Fatalf("expected view to contain Upstream tab header, got:\n%s", view)
	}

	// 2. Press '3' to jump to Skills
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.ActiveTabForTest() != tui.TabSkillsForTest {
		t.Fatalf("expected active tab to be Skills (%d), got %d", tui.TabSkillsForTest, m.ActiveTabForTest())
	}

	// 3. Press '2' to jump back to Upstreams
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if m.ActiveTabForTest() != tui.TabUpstreamsForTest {
		t.Fatalf("expected active tab to return to Upstreams (%d), got %d", tui.TabUpstreamsForTest, m.ActiveTabForTest())
	}
}

func TestTUIUpstreams_SearchFilterAndNavigation(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Jump to Tab 2 (Upstreams)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	// Inject 2 upstreams
	m = step(m, tui.NewUpstreamReloadMsgForTest([]model.UpstreamInfo{
		{
			URL:    "https://github.com/microsoft/skills",
			Type:   model.SourceTypeGit,
			Status: model.UpstreamUpToDate,
			Skills: []string{"azure-vm", "azure-storage"},
		},
		{
			URL:    "https://github.com/anthropics/skills",
			Type:   model.SourceTypeGit,
			Status: model.UpstreamUpdateAvailable,
			Skills: []string{"claude-code"},
		},
	}))

	if m.UpstreamsCountForTest() != 2 {
		t.Fatalf("expected 2 upstreams, got %d", m.UpstreamsCountForTest())
	}
	if m.DisplayedUpstreamsCountForTest() != 2 {
		t.Fatalf("expected 2 displayed upstreams, got %d", m.DisplayedUpstreamsCountForTest())
	}

	view := m.View()
	if !strings.Contains(view, "microsoft/skills") || !strings.Contains(view, "anthropics/skills") {
		t.Fatalf("expected both upstreams in view, got:\n%s", view)
	}

	// Search filter for "anthropics"
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, ch := range "anthropics" {
		m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	if m.DisplayedUpstreamsCountForTest() != 1 {
		t.Fatalf("expected 1 displayed upstream after filter, got %d", m.DisplayedUpstreamsCountForTest())
	}
	view = m.View()
	if !strings.Contains(view, "anthropics/skills") {
		t.Fatalf("expected anthropics/skills in filtered view, got:\n%s", view)
	}
	if strings.Contains(view, "microsoft/skills") {
		t.Fatalf("expected microsoft/skills to be filtered out, got:\n%s", view)
	}

	// Clear search with Esc
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.DisplayedUpstreamsCountForTest() != 2 {
		t.Fatalf("expected 2 displayed upstreams after clearing search, got %d", m.DisplayedUpstreamsCountForTest())
	}

	// Test cursor movement
	if m.UpstreamCursorForTest() != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.UpstreamCursorForTest())
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.UpstreamCursorForTest() != 1 {
		t.Fatalf("expected cursor at 1 after down key, got %d", m.UpstreamCursorForTest())
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.UpstreamCursorForTest() != 0 {
		t.Fatalf("expected cursor at 0 after up key, got %d", m.UpstreamCursorForTest())
	}
}

func TestTUIUpstreams_EnterDiscoversAndOpensAddModal(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Jump to Tab 2 (Upstreams)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	// Inject upstream
	repoURL := "https://github.com/microsoft/skills"
	m = step(m, tui.NewUpstreamReloadMsgForTest([]model.UpstreamInfo{
		{
			URL:    repoURL,
			Type:   model.SourceTypeGit,
			Status: model.UpstreamUpToDate,
			Skills: []string{"azure-vm"},
		},
	}))

	// Press 's' to trigger discover
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("expected tea.Cmd returned on 's' for upstream discovery")
	}

	// Simulate receiving discovered skills from upstream
	discMsg := tui.NewDiscoveredSkillsMsgForTest(repoURL, "", []string{"azure-vm", "azure-storage", "azure-monitor"})
	newM, _ = newM.Update(discMsg)
	modelObj := newM.(*tui.Model)

	// Should transition to AddModal Step 1 (Discovered Skills)
	if modelObj.CurrentViewForTest() != tui.ViewModalAddForTest {
		t.Fatalf("expected current view to be ViewModalAdd (%d), got %d", tui.ViewModalAddForTest, modelObj.CurrentViewForTest())
	}
	if modelObj.AddModalStepForTest() != 1 {
		t.Fatalf("expected AddModal step to be 1, got %d", modelObj.AddModalStepForTest())
	}
	if modelObj.AddModalVisibleCountForTest() != 3 {
		t.Fatalf("expected 3 discovered skills in AddModal, got %d", modelObj.AddModalVisibleCountForTest())
	}
	if modelObj.AddModalSourceURLForTest() != repoURL {
		t.Fatalf("expected AddModal SourceURL %q, got %q", repoURL, modelObj.AddModalSourceURLForTest())
	}

	// Select the first skill and press Enter to import
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	newM, cmd = newM.Update(tea.KeyMsg{Type: tea.KeyEnter})
	modelObj = newM.(*tui.Model)
	if modelObj.AddModalErrorForTest() != "" {
		t.Fatalf("unexpected error on AddModal Enter: %s", modelObj.AddModalErrorForTest())
	}
	if cmd == nil {
		t.Fatal("expected tea.Cmd returned on Enter to import selected skills")
	}
}

func TestTUIDiffView_UpdateWithUKey(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "sample-skill")
	m := tui.NewModel(ctx, svc)

	// Open diff view for sample-skill
	m = step(m, tui.NewDiffLoadedMsgForTest("--- a/SKILL.md\n+++ b/SKILL.md\n@@ -1 +1 @@\n-old\n+new\n"))
	if m.CurrentViewForTest() != tui.ViewDiffForTest {
		t.Fatalf("expected view to be ViewDiff (%d), got %d", tui.ViewDiffForTest, m.CurrentViewForTest())
	}

	// Press 'u' in diff view to trigger skill update
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	modelObj := newM.(*tui.Model)

	// Should return to list view and trigger update command
	if modelObj.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to ViewList (%d), got %d", tui.ViewListForTest, modelObj.CurrentViewForTest())
	}
	if cmd == nil {
		t.Fatal("expected tea.Cmd returned on pressing 'u' in diff view")
	}
}

func TestTUIUpstreams_IOpensDetailModal(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Jump to Tab 2 (Upstreams)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	// Inject upstream with timestamps and cache info
	up := model.UpstreamInfo{
		URL:             "https://github.com/test-org/test-skills",
		Name:            "test-alias",
		Type:            model.SourceTypeGit,
		Ref:             "main",
		Status:          model.UpstreamUpToDate,
		CreatedAt:       "2026-09-01 10:00:00",
		UpdatedAt:       "2026-09-14 09:00:00",
		CachePath:       "/home/user/.cache/asoul/test-skills",
		CacheExists:     true,
		Commit:          "abc12345",
		AvailableSkills: []string{"skill-alpha", "skill-beta"},
		Skills:          []string{"skill-alpha"},
	}
	m = step(m, tui.NewUpstreamReloadMsgForTest([]model.UpstreamInfo{up}))

	// View should show skills summary in list
	view := m.View()
	if !strings.Contains(view, "共 2 个 (已引用 1)") && !strings.Contains(view, "2 total (1 ref)") {
		t.Fatalf("expected view to display skills summary, got:\n%s", view)
	}

	// Press i to open upstream details modal
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.CurrentViewForTest() != tui.ViewModalUpstreamDetailForTest {
		t.Fatalf("expected view to be ViewModalUpstreamDetail (%d), got %d", tui.ViewModalUpstreamDetailForTest, m.CurrentViewForTest())
	}

	modalUp, isOpen := m.UpstreamDetailModalForTest()
	if !isOpen {
		t.Fatal("expected UpstreamDetailModal to be open")
	}
	if modalUp.URL != up.URL {
		t.Errorf("expected modal URL %q, got %q", up.URL, modalUp.URL)
	}
	if modalUp.CreatedAt != "2026-09-01 10:00:00" {
		t.Errorf("expected CreatedAt '2026-09-01 10:00:00', got %q", modalUp.CreatedAt)
	}
	if modalUp.UpdatedAt != "2026-09-14 09:00:00" {
		t.Errorf("expected UpdatedAt '2026-09-14 09:00:00', got %q", modalUp.UpdatedAt)
	}

	// Modal view check: initially displays basic info & cache
	modalView := m.View()
	if !strings.Contains(modalView, "2026-09-01 10:00:00") {
		t.Errorf("expected modal view to display CreatedAt, got:\n%s", modalView)
	}
	if !strings.Contains(modalView, "2026-09-14 09:00:00") {
		t.Errorf("expected modal view to display UpdatedAt, got:\n%s", modalView)
	}

	// Scroll down to view skills section
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	modalView = m.View()
	if !strings.Contains(modalView, "拥有 2 个技能") && !strings.Contains(modalView, "contains 2 skills") {
		t.Errorf("expected modal view to display skills summary intro, got:\n%s", modalView)
	}
	if !strings.Contains(modalView, "skill-alpha") || !strings.Contains(modalView, "skill-beta") {
		t.Errorf("expected modal view to display available skills, got:\n%s", modalView)
	}

	// Press Esc to close
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to ViewList after Esc, got %d", m.CurrentViewForTest())
	}

	// Press i again to open, then Space to close
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.CurrentViewForTest() != tui.ViewModalUpstreamDetailForTest {
		t.Fatalf("expected view to be ViewModalUpstreamDetail after i, got %d", m.CurrentViewForTest())
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to ViewList after Space, got %d", m.CurrentViewForTest())
	}
}

func TestTUIUpstreams_RefreshKey(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Jump to Tab 2 (Upstreams)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	// Press 'r' to refresh from cache
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})

	notice := m.NoticeForTest()
	if !strings.Contains(notice, "刷新") && !strings.Contains(notice, "Refreshed") {
		t.Fatalf("expected cache refresh notice on pressing 'r', got: %q", notice)
	}
}

func TestTUIUpstreamDetailModal_FixedWidthAndWrapping(t *testing.T) {
	up := model.UpstreamInfo{
		URL:             "https://github.com/very-long-org-organization-name/super-duper-long-repository-skills-name.git",
		Type:            model.SourceTypeGit,
		Ref:             "feature/super-extra-long-branch-name-for-testing-modal-wrapping",
		Status:          model.UpstreamUpdateAvailable,
		CreatedAt:       "2026-09-01 10:00:00",
		UpdatedAt:       "2026-09-14 09:00:00",
		CachePath:       "/home/user/.cache/asoul/git/github.com_very-long-org-organization-name_super-duper-long-repository-skills-name",
		CacheExists:     true,
		Commit:          "abc1234567890abcdef1234567890abcdef123456",
		NewCommit:       "def9876543210fedcba9876543210fedcba987654",
		AvailableSkills: []string{"skill-alpha-one", "skill-beta-two", "skill-gamma-three", "skill-delta-four", "skill-epsilon-five", "skill-zeta-six", "skill-eta-seven", "skill-theta-eight"},
		Skills:          []string{"skill-alpha-one", "skill-beta-two"},
	}

	// 1. Wide terminal: modal width must be capped at 76
	renderedWide := tui.RenderUpstreamDetailModalForTest(up, 120, 60)
	wideWidth := lipgloss.Width(renderedWide)
	if wideWidth != 76 {
		t.Fatalf("expected fixed modal width 76 in wide terminal (120 cols), got %d", wideWidth)
	}

	// Verify all lines in the rendered box are exactly 76 cols
	for idx, line := range strings.Split(renderedWide, "\n") {
		if line == "" {
			continue
		}
		lineWidth := lipgloss.Width(line)
		if lineWidth != 76 {
			t.Errorf("line %d has width %d, expected 76:\n%s", idx, lineWidth, line)
		}
	}

	// Verify that skills wrapped across multiple lines
	if !strings.Contains(renderedWide, "skill-alpha-one") || !strings.Contains(renderedWide, "skill-theta-eight") {
		t.Fatalf("expected all available skills in wide modal view")
	}
	// Verify indented lines for multiline skills
	if !strings.Contains(renderedWide, "      skill-") {
		t.Fatalf("expected indented skill items in multiline view, got:\n%s", renderedWide)
	}

	// 2. Narrow terminal (e.g. 56 cols): modal width should adapt to 52 (56 - 4)
	renderedNarrow := tui.RenderUpstreamDetailModalForTest(up, 56, 60)
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

	// 3. Footer wrapping: verify footer keybindings exist and wrap nicely
	if !strings.Contains(renderedWide, "Esc/Space") || !strings.Contains(renderedWide, "Enter") {
		t.Fatalf("expected footer key hints in rendered modal")
	}
}

func TestTUIUpstreamDetailModal_HeightAndScrolling(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	// Initialize terminal to standard 80x24
	m = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Jump to Tab 2 (Upstreams)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	// Inject an upstream with many skills that exceed a 24-row screen
	var skills []string
	for i := 1; i <= 25; i++ {
		skills = append(skills, "upstream-skill-item-"+string(rune('a'+i-1)))
	}
	up := model.UpstreamInfo{
		URL:             "https://github.com/example/skills-repo",
		Type:            model.SourceTypeGit,
		Ref:             "main",
		Status:          model.UpstreamUpToDate,
		CreatedAt:       "2026-09-01 10:00:00",
		UpdatedAt:       "2026-09-14 09:00:00",
		CachePath:       "/home/user/.cache/asoul/skills-repo",
		CacheExists:     true,
		Commit:          "abc12345",
		AvailableSkills: skills,
		Skills:          skills[:5],
	}
	m = step(m, tui.NewUpstreamReloadMsgForTest([]model.UpstreamInfo{up}))

	// Press i to open detail modal
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.CurrentViewForTest() != tui.ViewModalUpstreamDetailForTest {
		t.Fatalf("expected view to be ViewModalUpstreamDetail, got %d", m.CurrentViewForTest())
	}

	// 1. Verify that rendered modal height does not exceed termHeight - 3 (21 rows)
	renderedModal := tui.RenderUpstreamDetailModalForTest(up, 80, 24)
	modalH := lipgloss.Height(renderedModal)
	if modalH > 21 {
		t.Fatalf("expected modal height <= 21 in 24-row terminal, got %d", modalH)
	}

	// 2. Verify that the full view output does NOT exceed terminal height (24 rows)
	view := m.View()
	viewLines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(viewLines) > 24 {
		t.Fatalf("expected total view height <= 24 rows, got %d lines", len(viewLines))
	}

	// 3. Top of window must NOT be overflowed: tabs bar and modal title must be present
	if !strings.Contains(view, "2. Sources") && !strings.Contains(view, "2. 来源") {
		t.Fatalf("tabs bar overflowed off top of window:\n%s", view)
	}
	if !strings.Contains(view, "📦") {
		t.Fatalf("modal title overflowed off top of window:\n%s", view)
	}

	// 4. Verify scrolling keys
	if m.UpstreamDetailModalViewportOffsetForTest() != 0 {
		t.Fatalf("expected initial viewport offset 0, got %d", m.UpstreamDetailModalViewportOffsetForTest())
	}

	// Down key scrolls by 1 line
	m = step(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.UpstreamDetailModalViewportOffsetForTest() != 1 {
		t.Fatalf("expected viewport offset 1 after Down, got %d", m.UpstreamDetailModalViewportOffsetForTest())
	}

	// 'j' scrolls down another line
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.UpstreamDetailModalViewportOffsetForTest() != 2 {
		t.Fatalf("expected viewport offset 2 after 'j', got %d", m.UpstreamDetailModalViewportOffsetForTest())
	}

	// Up key scrolls back up
	m = step(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.UpstreamDetailModalViewportOffsetForTest() != 1 {
		t.Fatalf("expected viewport offset 1 after Up, got %d", m.UpstreamDetailModalViewportOffsetForTest())
	}

	// 'k' scrolls back up to 0
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.UpstreamDetailModalViewportOffsetForTest() != 0 {
		t.Fatalf("expected viewport offset 0 after 'k', got %d", m.UpstreamDetailModalViewportOffsetForTest())
	}

	// 'G' (end) jumps to bottom
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	bottomOffset := m.UpstreamDetailModalViewportOffsetForTest()
	if bottomOffset <= 0 {
		t.Fatalf("expected positive viewport offset after 'G', got %d", bottomOffset)
	}

	// 'g' (home) jumps to top
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if m.UpstreamDetailModalViewportOffsetForTest() != 0 {
		t.Fatalf("expected viewport offset 0 after 'g', got %d", m.UpstreamDetailModalViewportOffsetForTest())
	}

	// PgDown scrolls half view
	m = step(m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.UpstreamDetailModalViewportOffsetForTest() <= 0 {
		t.Fatalf("expected positive viewport offset after PgDown, got %d", m.UpstreamDetailModalViewportOffsetForTest())
	}

	// Esc closes modal
	m = step(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to ViewList after Esc, got %d", m.CurrentViewForTest())
	}
}

func TestTUIUpstream_IOpensDetail(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})

	repoURL := "https://github.com/microsoft/skills"
	inject := func(mm *tui.Model) *tui.Model {
		return step(mm, tui.NewUpstreamReloadMsgForTest([]model.UpstreamInfo{
			{
				URL:    repoURL,
				Type:   model.SourceTypeGit,
				Status: model.UpstreamUpToDate,
				Skills: []string{"azure-vm"},
			},
		}))
	}
	m = inject(m)

	// 1. In Upstreams tab, pressing Enter/Space no longer opens the detail modal.
	mEnter := step(m, tea.KeyMsg{Type: tea.KeyEnter})
	if mEnter.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("Enter must not open upstream detail on the main list, got %d", mEnter.CurrentViewForTest())
	}
	mSpace := step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if mSpace.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("Space must not open upstream detail on the main list, got %d", mSpace.CurrentViewForTest())
	}

	// 2. "i" opens UpstreamDetailModal (the unified detail key).
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.CurrentViewForTest() != tui.ViewModalUpstreamDetailForTest {
		t.Fatalf("expected view to be ViewModalUpstreamDetail on i, got %d", m.CurrentViewForTest())
	}

	// 3. Pressing Enter inside UpstreamDetailModal triggers discover.
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected tea.Cmd returned on Enter inside detail modal for upstream discovery")
	}
	modelObj := newM.(*tui.Model)
	if modelObj.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to list while loading discover, got %d", modelObj.CurrentViewForTest())
	}

	// 4. Reopen with "i"; Space inside the detail modal closes it.
	m = tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = inject(m)
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if m.CurrentViewForTest() != tui.ViewModalUpstreamDetailForTest {
		t.Fatalf("expected view to be ViewModalUpstreamDetail on i, got %d", m.CurrentViewForTest())
	}
	m = step(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to ViewList after Space in detail modal, got %d", m.CurrentViewForTest())
	}
}
