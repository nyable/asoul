package tui_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/app"
	"asoul/internal/cache"
	"asoul/internal/catalog"
	"asoul/internal/config"
	"asoul/internal/i18n"
	"asoul/internal/model"
	"asoul/internal/state"
	"asoul/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func setupTestService(t *testing.T) (*app.Service, string, func()) {
	i18n.Init("en")
	tmpDir, err := os.MkdirTemp("", "asoul-tui-test-*")
	if err != nil {
		t.Fatal(err)
	}

	// Isolate user-level state so tests never read the developer's real
	// ~/.config/asoul/rules or models.dev cache.
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmpDir, ".cache"))

	wsRoot := filepath.Join(tmpDir, "ws")
	_ = catalog.InitWorkspace(wsRoot)

	cfgFile := filepath.Join(tmpDir, "config.jsonc")
	stateFile := filepath.Join(tmpDir, "deployments.json")
	cacheDir := filepath.Join(tmpDir, "git-cache")

	cfgMgr, _ := config.NewManager(cfgFile)
	stateMgr, _ := state.NewManager(stateFile)
	cacheMgr, _ := cache.NewManager(cacheDir)

	svc := app.NewService(wsRoot, cfgMgr, stateMgr, cacheMgr)
	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
		i18n.Init("en")
	}
	return svc, wsRoot, cleanup
}

func newTestSkillsModel(ctx context.Context, svc *app.Service) *tui.Model {
	m := tui.NewModel(ctx, svc)
	m.SelectTabForTest(tui.TabSkillsForTest)
	return m
}

// pump executes a command and feeds the resulting messages back into the model
// until no further command is returned. It mirrors the Bubble Tea event loop so
// tests can observe state after async reloads complete.
func pump(m tea.Model, cmd tea.Cmd) tea.Model {
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			break
		}
		var next tea.Cmd
		m, next = m.Update(msg)
		cmd = next
	}
	return m
}

func TestTUIModelAndViews(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	// Add a managed skill and target
	_ = svc.NewSkill(ctx, "test-skill-a")
	_ = svc.TargetAdd("test-target", "/tmp/tgt")

	m := tui.NewModel(ctx, svc)

	// Test Initial View
	view := m.View()
	if !strings.Contains(view, "1. Overview") {
		t.Fatalf("expected view to contain overview tab, got:\n%s", view)
	}

	// Test Tab switching: press "2" (Sources)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	view = newM.View()
	if !strings.Contains(view, "2. Sources") {
		t.Fatalf("expected view to display sources tab, got:\n%s", view)
	}

	// Test Tab switching: press "3" (Skills)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	view = newM.View()
	if !strings.Contains(view, "test-skill-a") {
		t.Fatalf("expected view to display skills with test-skill-a, got:\n%s", view)
	}

	// Test Tab switching: press "4" (Channels)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view = newM.View()
	if !strings.Contains(view, "4. Channels") || !strings.Contains(view, "test-target") {
		t.Fatalf("expected view to display channels, got:\n%s", view)
	}

	// Test Tab switching: press "5" (Projects)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	view = newM.View()
	if !strings.Contains(view, "5. Projects") {
		t.Fatalf("expected view to display projects tab, got:\n%s", view)
	}

	// Test Tab switching: press "6" (System > Cache)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})
	view = newM.View()
	if !strings.Contains(view, "CACHE REPOSITORY") && !strings.Contains(view, "Git cache directory") {
		t.Fatalf("expected view to display cache info, got:\n%s", view)
	}

	// Test Tab switching: System > Diagnostics
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyTab})
	newM, doctorCmd := newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	newM = pump(newM, doctorCmd)
	view = newM.View()
	if !strings.Contains(view, "CHECK ITEM") {
		t.Fatalf("expected localized doctor table header, got:\n%s", view)
	}
	if strings.Contains(view, "table.header.") || strings.Contains(view, "doctor.msg.") || strings.Contains(view, "doctor.check.") {
		t.Fatalf("raw i18n key leaked into doctor view:\n%s", view)
	}

	// Test Tab switching: press "7" (Settings)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	view = newM.View()
	if !strings.Contains(view, "SETTING ITEM") {
		t.Fatalf("expected view to display settings view, got:\n%s", view)
	}

	// Test return to tab 3 (Skills)
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})

	// Test opening Add Modal with "a"
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	view = newM.View()
	if !strings.Contains(view, "Add Skill from Git Repository or Local Path") {
		t.Fatalf("expected add skill modal, got:\n%s", view)
	}

	// Cancel modal with Esc
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyEsc})
	view = newM.View()
	if strings.Contains(view, "Add Skill from Git Repository") {
		t.Fatalf("expected modal to be closed after Esc")
	}

	// Test opening New Skill Modal with "n"
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	view = newM.View()
	if !strings.Contains(view, "Create New Managed Skill") {
		t.Fatalf("expected new skill modal, got:\n%s", view)
	}

	// Cancel with Esc
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Test Help screen with "?"
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	view = newM.View()
	if !strings.Contains(view, "Keyboard Shortcuts") {
		t.Fatalf("expected help screen, got:\n%s", view)
	}

	_ = wsRoot
}

func TestTUIAddGitFlow(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// 1. Press "a" to open Add Modal
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !strings.Contains(m1.View(), "Source Path or Git URL") {
		t.Fatalf("expected add modal Step 0, got:\n%s", m1.View())
	}

	// 2. Simulate typing a github.com URL and pressing Enter
	// Simulate discovered skills response from async worker
	discMsg := tui.NewDiscoveredSkillsMsgForTest("https://github.com/example/skills.git", "main", []string{"helper-a", "helper-b"})
	m2, _ := m1.Update(discMsg)

	view := m2.View()
	if !strings.Contains(view, "Discovered 2 Skills") || !strings.Contains(view, "helper-a") {
		t.Fatalf("expected step 1 discovered skills checklist, got:\n%s", view)
	}

	// 3. Confirm selection with Enter and receive asyncNoticeMsg
	noticeMsg := tui.NewAsyncNoticeMsgForTest("Added 2 skills from https://github.com/example/skills.git")
	m3, _ := m2.Update(noticeMsg)

	// Modal must be closed and back to list
	view3 := m3.View()
	if strings.Contains(view3, "Discovered 2 Skills") {
		t.Fatalf("expected modal to close after asyncNoticeMsg, got:\n%s", view3)
	}
	if !strings.Contains(view3, "Added 2 skills") {
		t.Fatalf("expected notice message on main screen, got:\n%s", view3)
	}
}

func TestTUIScrolling(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Press "a" to open Add Modal
	mAdd, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// Simulate discovery of 20 skills
	var skillNames []string
	for i := 1; i <= 20; i++ {
		skillNames = append(skillNames, fmt.Sprintf("skill-%02d", i))
	}
	discMsg := tui.NewDiscoveredSkillsMsgForTest("https://github.com/example/repo", "main", skillNames)
	m1, _ := mAdd.Update(discMsg)

	// View at top
	viewTop := m1.View()
	if !strings.Contains(viewTop, "more skills below") {
		t.Fatalf("expected bottom scroll indicator, got:\n%s", viewTop)
	}
	if strings.Contains(viewTop, "more skills above") {
		t.Fatalf("did not expect top scroll indicator when at top, got:\n%s", viewTop)
	}
	// Verify first item is aligned properly without offset
	if !strings.Contains(viewTop, "│  ▶ [✓] skill-01") || !strings.Contains(viewTop, "│    [✓] skill-02") {
		t.Fatalf("expected aligned items in viewTop, got:\n%s", viewTop)
	}

	// Move cursor down 15 times
	curr := m1
	for i := 0; i < 15; i++ {
		curr, _ = curr.Update(tea.KeyMsg{Type: tea.KeyDown})
	}

	viewScrolled := curr.View()
	if !strings.Contains(viewScrolled, "more skills above") {
		t.Fatalf("expected top scroll indicator after scrolling down, got:\n%s", viewScrolled)
	}
	// Verify item immediately below top scroll indicator is aligned properly
	if !strings.Contains(viewScrolled, "│    [✓] skill-05") || !strings.Contains(viewScrolled, "│  ▶ [✓] skill-16") {
		t.Fatalf("expected aligned items in viewScrolled, got:\n%s", viewScrolled)
	}
}

func TestTUIDeployMultiSelectFlow(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	// Add 3 skills and 1 target
	_ = svc.NewSkill(ctx, "deploy-skill-1")
	_ = svc.NewSkill(ctx, "deploy-skill-2")
	_ = svc.NewSkill(ctx, "deploy-skill-3")
	_ = svc.TargetAdd("test-target", "/tmp/deploy-target-test")

	m := newTestSkillsModel(ctx, svc)

	// 1. Press "d" to open Deploy Modal (defaults directly to manual multi-select)
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	view1 := m1.View()
	if !strings.Contains(view1, "Skills List (Multi-Select)") || !strings.Contains(view1, "deploy-skill-1") {
		t.Fatalf("expected deploy manual multi-select default, got:\n%s", view1)
	}

	// 2. Press 'a' to select all skills
	m2, _ := m1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	// 3. Press Enter to proceed to Target selection
	m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view3 := m3.View()
	if !strings.Contains(view3, "Select Target") {
		t.Fatalf("expected target selection, got:\n%s", view3)
	}

	// 4. Press Enter on target -> returns async execution Cmd
	m4, cmd := m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected deploy command to be scheduled")
	}

	// Execute command to verify execution
	msg := cmd()
	if msg == nil {
		t.Fatalf("expected non-nil msg from cmd")
	}

	// Update with success notice
	m5, _ := m4.Update(tui.NewAsyncNoticeMsgForTest("Successfully completed for 3 skills to test-target"))
	view5 := m5.View()
	if !strings.Contains(view5, "Successfully completed") {
		t.Fatalf("expected success notice on main screen, got:\n%s", view5)
	}
}

func TestTUIChineseLanguage(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	// Set language in config to zh-CN
	cfgFile := filepath.Join(filepath.Dir(wsRoot), "config.jsonc")
	cfgMgr, _ := config.NewManager(cfgFile)
	cfg, _ := cfgMgr.Load()
	cfg.Language = "zh-CN"
	_ = cfgMgr.Save(cfg)

	_ = svc.NewSkill(ctx, "zh-skill-demo")
	_ = svc.TargetAdd("zh-target", "/tmp/zh-target")

	// Create model with zh-CN set
	m := newTestSkillsModel(ctx, svc)
	view := m.View()

	// Verify Chinese tab and title
	if !strings.Contains(view, "3. 技能") {
		t.Fatalf("expected Chinese tab in view, got:\n%s", view)
	}
	if !strings.Contains(view, "技能名称") {
		t.Fatalf("expected Chinese table header in view, got:\n%s", view)
	}

	// Test deploy modal in Chinese (defaults to manual multi-select)
	m1, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	view1 := m1.View()
	if !strings.Contains(view1, "技能列表 (手动多选)") || !strings.Contains(view1, "zh-skill-demo") {
		t.Fatalf("expected Chinese deploy multi-select, got:\n%s", view1)
	}

	// Exit deploy modal with Esc
	mBack, _ := m1.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Test help modal in Chinese (page-specific, opened from Skills)
	mHelp, _ := mBack.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	viewHelp := mHelp.View()
	if !strings.Contains(viewHelp, "快捷键帮助") || !strings.Contains(viewHelp, "技能") {
		t.Fatalf("expected Chinese per-page help screen, got:\n%s", viewHelp)
	}
	// The page-specific shortcut list is scrollable; jump to the bottom to see it.
	mHelpBottom, _ := mHelp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if !strings.Contains(mHelpBottom.View(), "删除选中的技能分组") {
		t.Fatalf("expected page-specific skills help after scrolling, got:\n%s", mHelpBottom.View())
	}

	// Exit help with Esc
	mBack2, _ := mHelp.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Test settings tab in Chinese
	mSettings, _ := mBack2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	viewSettings := mSettings.View()
	if !strings.Contains(viewSettings, "7. 设置") || !strings.Contains(viewSettings, "配置项") || !strings.Contains(viewSettings, "当前设定值") {
		t.Fatalf("expected Chinese settings screen, got:\n%s", viewSettings)
	}
}

func TestTUITabPrefixFormat(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)
	view := m.View()

	// Check tabs 1 to 7 in English
	expectedEn := []string{"1. Overview", "2. Sources", "3. Skills", "4. Channels", "5. Projects", "6. System", "7. Settings"}
	for _, tab := range expectedEn {
		if !strings.Contains(view, tab) {
			t.Fatalf("expected view to contain %q, got:\n%s", tab, view)
		}
	}
}

func TestTUIUnifiedQuitAndBack(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "quit-skill-demo")

	m := newTestSkillsModel(ctx, svc)

	// 1. In viewList: "q" should quit
	_, cmdQ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmdQ == nil {
		t.Fatalf("expected q on main view to produce a quit command")
	}

	// 2. In viewList: "esc" with empty search should NOT quit (esc is Back, not Quit)
	_, cmdEsc := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmdEsc != nil {
		t.Fatalf("expected esc on main view not to produce a quit command")
	}

	// 3. Open Help with "?" -> "esc" returns to viewList
	mHelp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !strings.Contains(mHelp.View(), "Keyboard Shortcuts") {
		t.Fatalf("expected help view")
	}
	mBack, cmdBack := mHelp.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmdBack != nil {
		t.Fatalf("expected esc in help view to return to list without quit command")
	}
	if strings.Contains(mBack.View(), "Keyboard Shortcuts") {
		t.Fatalf("expected back to main view after esc in help")
	}
	// Help view -> "q" behaves like esc (back, not quit)
	mHelp2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	mHelpBack, cmdHelpQ := mHelp2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmdHelpQ != nil {
		t.Fatalf("expected q in help view not to quit")
	}
	if strings.Contains(mHelpBack.View(), "Keyboard Shortcuts") {
		t.Fatalf("expected q in help view to return to the list")
	}

	// 4. Open Detail with i -> "esc" returns to viewList
	m = newTestSkillsModel(ctx, svc)
	mDetail, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if !strings.Contains(mDetail.View(), "SKILL.md") {
		t.Fatalf("expected detail view, got:\n%s", mDetail.View())
	}
	mBack2, cmdEscDetail := mDetail.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmdEscDetail != nil {
		t.Fatalf("expected esc in detail view to return to list without quit command")
	}
	if !strings.Contains(mBack2.View(), "3. Skills") {
		t.Fatalf("expected return to main list after esc in detail view")
	}

	// 5. Open Detail with i -> "q" behaves like esc (back, not quit)
	mDetail2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	mDetailBack, cmdDetailQ := mDetail2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmdDetailQ != nil {
		t.Fatalf("expected q in detail view not to quit")
	}
	if !strings.Contains(mDetailBack.View(), "3. Skills") {
		t.Fatalf("expected q in detail view to return to the main list")
	}
}

func TestTUISkillDetailCardAndScroll(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "scroll-test-skill")

	// Write a longer SKILL.md to test scrolling
	skillPath := filepath.Join(wsRoot, "skills", "scroll-test-skill", "SKILL.md")
	longContent := "---\nname: scroll-test-skill\ndescription: A test skill with many lines for scroll testing\n---\n\n# Heading 1\n"
	for i := 1; i <= 50; i++ {
		longContent += fmt.Sprintf("\nLine %02d: This is content to test scrolling inside the detail viewport.", i)
	}
	_ = os.WriteFile(skillPath, []byte(longContent), 0644)

	m := newTestSkillsModel(ctx, svc)

	// Enter detail view
	mDetail, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	view := mDetail.View()

	// Verify Fixed Header Card contents (compact layout without redundant title)
	if strings.Contains(view, "Skill Overview") || strings.Contains(view, "技能核心信息") {
		t.Fatalf("did not expect redundant title line in compact header card")
	}
	if !strings.Contains(view, "scroll-test-skill") {
		t.Fatalf("expected skill name in header card, got:\n%s", view)
	}
	if !strings.Contains(view, "scroll testing") {
		t.Fatalf("expected description in header card, got:\n%s", view)
	}
	if !strings.Contains(view, "SKILL.md") {
		t.Fatalf("expected path to SKILL.md in header card, got:\n%s", view)
	}
	// Verify merged pipeline line: 状态图标文本 来源 -> 本地路径
	if !strings.Contains(view, "->") {
		t.Fatalf("expected arrow '->' in merged status/source/path line, got:\n%s", view)
	}
	if !strings.Contains(view, "modified") {
		t.Fatalf("expected status 'modified' in header card, got:\n%s", view)
	}
	if !strings.Contains(view, "managed") {
		t.Fatalf("expected source 'managed' in header card, got:\n%s", view)
	}

	// Verify that field labels are removed
	if strings.Contains(view, "Name:") || strings.Contains(view, "技能名称:") {
		t.Fatalf("did not expect field name label in header card, got:\n%s", view)
	}
	if strings.Contains(view, "Description:") || strings.Contains(view, "技能描述:") {
		t.Fatalf("did not expect field desc label in header card, got:\n%s", view)
	}

	// Verify that document section divider is removed
	if strings.Contains(view, "SKILL.md Document") || strings.Contains(view, "SKILL.md 文档内容") {
		t.Fatalf("did not expect document section divider in detail view, got:\n%s", view)
	}

	// Verify footer progress indicator; generic arrow-key scroll hints are
	// intentionally omitted from the footer.
	if !strings.Contains(view, "[TOP]") {
		t.Fatalf("expected [TOP] progress hint in footer, got:\n%s", view)
	}
	if strings.Contains(view, "PgUp") {
		t.Fatalf("footer should not describe arrow-key scrolling, got:\n%s", view)
	}

	// Test scrolling down with "j"
	mScrolled, _ := mDetail.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	viewScrolled := mScrolled.View()
	if !strings.Contains(viewScrolled, "scroll-test-skill") {
		t.Fatalf("fixed header card must remain visible after scroll, got:\n%s", viewScrolled)
	}

	// Test exit/return with esc
	mList, cmdBack := mScrolled.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmdBack != nil {
		t.Fatalf("expected esc to return without quit command")
	}
	if !strings.Contains(mList.View(), "3. Skills") {
		t.Fatalf("expected return to skill list after pressing esc")
	}
}

func TestTUIChannelsAndProjectsTabs(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	sampleProjDir := filepath.Join(filepath.Dir(wsRoot), "sample-project")
	_ = os.MkdirAll(sampleProjDir, 0755)

	_ = svc.TargetAdd("custom-agent-tgt", "/tmp/custom-agent")
	_ = svc.ProjectAdd(sampleProjDir)

	m := tui.NewModel(ctx, svc)

	// Press "4" to go to Channels tab
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view2 := m2.View()

	if !strings.Contains(view2, "Agent Channel") || !strings.Contains(view2, "custom-agent-tgt") {
		t.Fatalf("expected Channels tab with the configured channel, got:\n%s", view2)
	}

	// Press "5" to go to Projects tab
	mProj, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	viewProj := mProj.View()
	if !strings.Contains(viewProj, "PROJECT PATH") || !strings.Contains(viewProj, "sample-project") {
		t.Fatalf("expected Projects tab with sample-project, got:\n%s", viewProj)
	}
}

func TestTUIGroupManagement(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-x")
	_ = svc.NewSkill(ctx, "skill-y")

	m := tui.NewModel(ctx, svc)

	// Go to Skills > Groups
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m3, _ = m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3, _ = m3.Update(tea.KeyMsg{Type: tea.KeyRight})
	view3 := m3.View()
	if !strings.Contains(view3, "Groups") {
		t.Fatalf("expected Groups tab, got:\n%s", view3)
	}

	// Press "a" to create a new group
	mAdd, _ := m3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	viewAdd := mAdd.View()
	if !strings.Contains(viewAdd, "Create Skill Group") {
		t.Fatalf("expected add group modal, got:\n%s", viewAdd)
	}

	// Type group name "dev-skills"
	curr := mAdd
	for _, ch := range "dev-skills" {
		curr, _ = curr.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// Press Enter to submit creation
	mCreated, cmdCreate := curr.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmdCreate == nil {
		t.Fatalf("expected cmdCreate not to be nil")
	}
	msg := cmdCreate()
	mAfterNotice, _ := mCreated.Update(msg)

	// Switch back to Skills > Groups to view the created group
	mAfterNotice, _ = mAfterNotice.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	mAfterNotice, _ = mAfterNotice.Update(tea.KeyMsg{Type: tea.KeyTab})
	mAfterNotice, _ = mAfterNotice.Update(tea.KeyMsg{Type: tea.KeyRight})
	viewAfter := mAfterNotice.View()
	if !strings.Contains(viewAfter, "dev-skills") {
		t.Fatalf("expected created group dev-skills in list, got:\n%s", viewAfter)
	}

	// Press 'e' to assign skills to group
	mAssign, _ := mAfterNotice.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	viewAssign := mAssign.View()
	if !strings.Contains(viewAssign, "Assign Skills to Group") || !strings.Contains(viewAssign, "skill-x") {
		t.Fatalf("expected group skills assignment modal, got:\n%s", viewAssign)
	}

	// Toggle first skill with Space
	mToggle, _ := mAssign.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Press Enter to save assignment
	mSaved, cmdSave := mToggle.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmdSave == nil {
		t.Fatalf("expected cmdSave not to be nil")
	}
	saveMsg := cmdSave()
	mFinal, _ := mSaved.Update(saveMsg)

	// Verify group has skills in service
	grp, err := svc.GroupGet("dev-skills")
	if err != nil || len(grp.Skills) == 0 {
		t.Fatalf("expected group dev-skills to have assigned skills, err: %v, grp: %+v", err, grp)
	}
	_ = mFinal
}

func TestTUIDeployGroupBatchSwitch(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "deploy-a")
	_ = svc.NewSkill(ctx, "deploy-b")
	_ = svc.GroupCreate("batch-group")
	_ = svc.GroupSetSkills("batch-group", []string{"deploy-a", "deploy-b"})
	_ = svc.TargetAdd("sample-agent", "/tmp/sample-agent")
	myProj := filepath.Join(filepath.Dir(wsRoot), "my-project")
	_ = os.MkdirAll(myProj, 0755)
	_ = svc.ProjectAdd(myProj)

	m := newTestSkillsModel(ctx, svc)

	// 1. Press "d" to open Deploy modal (default manual multi-select)
	mDeploy, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	viewD := mDeploy.View()
	if !strings.Contains(viewD, "Skills List (Multi-Select)") {
		t.Fatalf("expected manual multi-select default in deploy modal, got:\n%s", viewD)
	}

	// 2. Press Right to switch to Batch by Group
	mGroupMode, _ := mDeploy.Update(tea.KeyMsg{Type: tea.KeyRight})
	viewG := mGroupMode.View()
	if !strings.Contains(viewG, "batch-group") {
		t.Fatalf("expected batch-group in group mode deploy modal, got:\n%s", viewG)
	}

	// 3. Press Enter on the group to proceed to target selection
	mTarget, _ := mGroupMode.Update(tea.KeyMsg{Type: tea.KeyEnter})
	viewT := mTarget.View()
	if !strings.Contains(viewT, "Select Target") || !strings.Contains(viewT, "deploy-a, deploy-b") {
		t.Fatalf("expected target selection with resolved group skills, got:\n%s", viewT)
	}

	// 4. In target selection, press Right to switch to Project channel
	mTargetProj, _ := mTarget.Update(tea.KeyMsg{Type: tea.KeyRight})
	viewTP := mTargetProj.View()
	if !strings.Contains(viewTP, "my-project") {
		t.Fatalf("expected project targets after Tab switch in deploy modal, got:\n%s", viewTP)
	}
}

func TestTUISettingsTabAndNavigation(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// 1. Switch to tab 8 with "8"
	m7, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	view := m7.View()
	if !strings.Contains(view, "SETTING ITEM") || !strings.Contains(view, "Interface Language") {
		t.Fatalf("expected settings tab to display SETTING ITEM table, got:\n%s", view)
	}

	// Check footer hint
	if !strings.Contains(view, "toggle") && !strings.Contains(view, "切换") {
		t.Fatalf("expected settings footer, got:\n%s", view)
	}

	// 2. Switch back to tab 1 and test shortcut "s"
	m1, _ := m7.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if strings.Contains(m1.View(), "SETTING ITEM") {
		t.Fatalf("expected tab 1, not settings")
	}

	mS, _ := m1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !strings.Contains(mS.View(), "SETTING ITEM") {
		t.Fatalf("expected shortcut 's' to jump to settings tab, got:\n%s", mS.View())
	}

	// 3. Test cursor movement
	mDown, _ := mS.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	viewDown := mDown.View()
	if !strings.Contains(viewDown, "Skill Workspace") && !strings.Contains(viewDown, "工作区") {
		t.Fatalf("expected Skill Workspace selected, got:\n%s", viewDown)
	}
}

func TestTUISettingsLanguageSwitch(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Switch to settings
	mSettings, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})

	// Press space on Language setting to cycle language
	mCycled, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	viewCycled := mCycled.View()

	// Should now be in Chinese or English
	if !strings.Contains(viewCycled, "配置项") && !strings.Contains(viewCycled, "SETTING ITEM") {
		t.Fatalf("expected language toggled, got view:\n%s", viewCycled)
	}

	// Test 'e' to open Language selection modal
	mEnter, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	viewModal := mEnter.View()
	if !strings.Contains(viewModal, "Select Interface Language") && !strings.Contains(viewModal, "选择界面显示语言") {
		t.Fatalf("expected language selection modal, got:\n%s", viewModal)
	}

	// Press "1" in language modal to explicitly select Simplified Chinese
	mZh, zhCmd := mEnter.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	mZh = pump(mZh, zhCmd)
	viewZh := mZh.View()
	if !strings.Contains(viewZh, "配置项") || !strings.Contains(viewZh, "7. 设置") {
		t.Fatalf("expected Chinese interface after selecting option 1, got:\n%s", viewZh)
	}

	// Verify persistence in config
	cfg, err := svc.Config()
	if err != nil || cfg == nil || cfg.Language != "zh-CN" {
		t.Fatalf("expected cfg.Language to be zh-CN, got %v (err: %v)", cfg, err)
	}
}

func TestTUISettingsEditDefaultRoot(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Switch to settings
	mSettings, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})

	// Move to Workspace (item 1)
	mItem1, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Press 'e' to open add workspace modal
	mModal, _ := mItem1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	viewModal := mModal.View()
	if !strings.Contains(viewModal, "Workspace") && !strings.Contains(viewModal, "工作区") {
		t.Fatalf("expected edit workspace modal, got:\n%s", viewModal)
	}

	// Enter new path: simulate typing
	for _, ch := range "/tmp/custom-skills-root" {
		mModal, _ = mModal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	mSaved, saveCmd := mModal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mSaved = pump(mSaved, saveCmd)

	// Modal should close and return to list
	viewSaved := mSaved.View()
	if !strings.Contains(viewSaved, "custom-skills-root") {
		t.Fatalf("expected updated path in view, got:\n%s", viewSaved)
	}

	// Verify persistence in config
	cfg, err := svc.Config()
	if err != nil || cfg == nil || !strings.Contains(cfg.DefaultRoot, "custom-skills-root") {
		t.Fatalf("expected cfg.DefaultRoot to be updated, got %v", cfg)
	}
	// Verify active workspace is also switched immediately
	if !strings.Contains(svc.WorkspaceRoot(), "custom-skills-root") {
		t.Fatalf("expected svc.WorkspaceRoot to be switched immediately, got %v", svc.WorkspaceRoot())
	}
}

func TestTUISettingsWorkspaceSwitchAndInit(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Switch to settings
	mSettings, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})

	// Press "w" to switch workspace
	mW, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	viewW := mW.View()
	if !strings.Contains(viewW, "Workspace") && !strings.Contains(viewW, "工作区") {
		t.Fatalf("expected switch workspace modal, got:\n%s", viewW)
	}

	// Press Esc to cancel
	mCancel, _ := mW.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(mCancel.View(), "Change Skill Workspace Path") || strings.Contains(mCancel.View(), "修改技能工作区路径") {
		t.Fatalf("expected modal closed on Esc")
	}

	// Press "I" when already initialized
	mInit, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	viewInit := mInit.View()
	if !strings.Contains(viewInit, "already") && !strings.Contains(viewInit, "无需") {
		t.Fatalf("expected already initialized notice, got:\n%s", viewInit)
	}
}

func TestTUISettingsResetDefaults(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// Switch to settings
	mSettings, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})

	// Press "R" to trigger Reset Defaults confirmation
	mConfirm, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	viewConfirm := mConfirm.View()
	if !strings.Contains(viewConfirm, "Reset to Recommended Defaults") && !strings.Contains(viewConfirm, "恢复推荐默认配置") {
		t.Fatalf("expected reset defaults confirmation modal, got:\n%s", viewConfirm)
	}

	// Confirm with "y"
	mDone, cmd := mConfirm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd != nil {
		msg := cmd()
		mDone, _ = mDone.Update(msg)
	}

	// Verify standard targets created in config
	cfg, err := svc.Config()
	if err != nil || cfg == nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
	if _, ok := cfg.Targets["codex"]; !ok {
		t.Fatalf("expected codex target after reset defaults, got %v", cfg.Targets)
	}
	if _, ok := cfg.Targets["opencode"]; !ok {
		t.Fatalf("expected opencode target after reset defaults, got %v", cfg.Targets)
	}
}

func TestTUIMultiWorkspaceManagementFlow(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	m := tui.NewModel(ctx, svc)

	// 1. Switch to settings
	mSettings, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})

	// 2. Move to workspace setting item (item 1) and press w to open SelectWorkspaceModal
	mWsItem, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mSelector, _ := mWsItem.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	viewSelector := mSelector.View()
	if !strings.Contains(viewSelector, "Workspaces") && !strings.Contains(viewSelector, "工作区") {
		t.Fatalf("expected workspace selection modal, got:\n%s", viewSelector)
	}

	// 3. Press 'a' inside selector to open add workspace modal
	mAdd, _ := mSelector.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	viewAdd := mAdd.View()
	if !strings.Contains(viewAdd, "Add") && !strings.Contains(viewAdd, "添加") {
		t.Fatalf("expected add workspace modal, got:\n%s", viewAdd)
	}

	// 4. Type a new workspace path and hit enter
	newWS := filepath.Join(t.TempDir(), "secondary-ws")
	for _, ch := range newWS {
		mAdd, _ = mAdd.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	mAdded, addCmd := mAdd.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mAdded = pump(mAdded, addCmd)

	// Modal closes, now active workspace should be newWS
	if !strings.Contains(svc.WorkspaceRoot(), "secondary-ws") {
		t.Fatalf("expected svc.WorkspaceRoot to be secondary-ws, got %v", svc.WorkspaceRoot())
	}
	workspaces := svc.Workspaces()
	if len(workspaces) < 2 {
		t.Fatalf("expected at least 2 workspaces, got %v", workspaces)
	}

	// 5. Open workspace selector modal again via 'w'
	mSelector2, _ := mAdded.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	viewSelector2 := mSelector2.View()
	if !strings.Contains(viewSelector2, "secondary-ws") {
		t.Fatalf("expected secondary-ws in selector list, got:\n%s", viewSelector2)
	}

	// 6. Test initializing from modal with 'i'
	mInit, initCmd := mSelector2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	mInit = pump(mInit, initCmd)
	if !svc.WorkspacePathInitialized(newWS) {
		t.Fatalf("expected newWS to be initialized after pressing 'i'")
	}
	_ = mInit

	// 7. Test removing workspace with 'x'
	mRemove, _ := mSelector2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	viewRemove := mRemove.View()
	_ = viewRemove

	// 8. Test Esc closes modal
	mEsc, _ := mSelector2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	viewEsc := mEsc.View()
	if strings.Contains(viewEsc, "Workspaces Management") || strings.Contains(viewEsc, "工作区管理与切换") {
		t.Fatalf("expected modal closed on Esc")
	}
}

func TestTUIWorkspaceNoDuplicateDisplay(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	// Add workspace with relative path and equivalent absolute path
	_ = svc.AddWorkspace("my-custom-ws")
	cwd, _ := os.Getwd()
	absCustom := filepath.Join(cwd, "my-custom-ws")
	_ = svc.AddWorkspace(absCustom)

	// In svc.Workspaces(), there should be no duplicates
	allWS := svc.Workspaces()
	for i := 0; i < len(allWS); i++ {
		for j := i + 1; j < len(allWS); j++ {
			if allWS[i] == allWS[j] || strings.Contains(allWS[i], "my-custom-ws") && strings.Contains(allWS[j], "my-custom-ws") {
				t.Fatalf("found duplicate workspace in list: %s vs %s (all: %v)", allWS[i], allWS[j], allWS)
			}
		}
	}

	// Open TUI model and view selector modal
	m := tui.NewModel(ctx, svc)
	mSettings, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'7'}})
	mWsItem, _ := mSettings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mSelector, _ := mWsItem.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	view := mSelector.View()

	// Count occurrences of "my-custom-ws" in selector view
	count := strings.Count(view, "my-custom-ws")
	if count != 1 {
		t.Fatalf("expected my-custom-ws to appear exactly 1 time in selector modal, appeared %d times:\n%s", count, view)
	}
	_ = wsRoot
}

func TestTUIModelsTab(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	// Create test opencode.json in workspace
	opencodePath := filepath.Join(wsRoot, "opencode.json")
	sampleConfig := []byte(`{
		"provider": {
			"custom-ai": {
				"models": {
					"gpt-5": {},
					"claude-3-7-sonnet": {
						"limit": { "context": 200000 }
					}
				}
			}
		}
	}`)
	if err := os.WriteFile(opencodePath, sampleConfig, 0644); err != nil {
		t.Fatal(err)
	}
	_ = svc.TargetAdd("opencode", wsRoot)

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 120, Height: 60})

	// 1. Initial view should have tab 4 (Channels) in English
	view := m.View()
	if !strings.Contains(view, "4. Channels") {
		t.Fatalf("expected view to contain '4. Channels', got:\n%s", view)
	}

	// 2. Switch to tab 4 with key '4'
	m8, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view8 := m8.View()
	if !strings.Contains(view8, "opencode") {
		t.Fatalf("expected view to show the opencode channel, got:\n%s", view8)
	}
	if !strings.Contains(view8, "custom-ai") || !strings.Contains(view8, "gpt-5") {
		t.Fatalf("expected view to show custom-ai provider and gpt-5 model, got:\n%s", view8)
	}

	// 3. Test shortcut 'o' to toggle override mode
	mToggle, _ := m8.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	viewToggle := mToggle.View()
	if mToggle.(*tui.Model).ModelOverrideForTest() {
		t.Fatalf("expected override mode disabled after 'o', got view:\n%s", viewToggle)
	}
	if !strings.Contains(viewToggle, "Fill Missing") && !strings.Contains(viewToggle, "补齐缺失") {
		t.Fatalf("expected incremental mode indicator in view, got:\n%s", viewToggle)
	}

	// 4. Switch back with '1' and test shortcut 'm'
	m1, _ := mToggle.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	mFromShortcut, _ := m1.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	viewFromM := mFromShortcut.View()
	if !strings.Contains(viewFromM, "gpt-5") {
		t.Fatalf("expected 'm' to navigate to Models tab, got:\n%s", viewFromM)
	}
}

func TestTUITargetDetailModal(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	tgt := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "test-channel",
		ConfigDir: filepath.Join(filepath.Dir(wsRoot), ".config", "test-agent"),
		Paths: map[string]string{
			"skills": "${config_dir}/skills",
			"rules":  "${config_dir}/rules",
		},
	}
	_ = svc.TargetAddConfig("test-agent", tgt)

	m := tui.NewModel(ctx, svc)
	m = step(m, tea.WindowSizeMsg{Width: 100, Height: 40})

	// Press "4" to go to Channels tab
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

	// Press 'I' to open target details
	mDetail, detailCmd := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	mDetail = pump(mDetail, detailCmd)
	viewDetail := mDetail.View()

	if !strings.Contains(viewDetail, "Target Details") && !strings.Contains(viewDetail, "目标详情") {
		t.Fatalf("expected Target Details modal, got:\n%s", viewDetail)
	}
	if !strings.Contains(viewDetail, "test-agent") || !strings.Contains(viewDetail, "test-channel") {
		t.Fatalf("expected test-agent name and test-channel in modal, got:\n%s", viewDetail)
	}
	if !strings.Contains(viewDetail, "${config_dir}/skills") || !strings.Contains(viewDetail, "${config_dir}/rules") {
		t.Fatalf("expected artifact paths template in modal, got:\n%s", viewDetail)
	}

	// Press Esc to close modal
	mBack, _ := mDetail.Update(tea.KeyMsg{Type: tea.KeyEsc})
	viewBack := mBack.View()
	if strings.Contains(viewBack, "Artifact Mappings") || strings.Contains(viewBack, "产物映射") {
		t.Fatalf("expected to return to list after Esc, got:\n%s", viewBack)
	}
}

func TestTUIChannelDeployBlockColumns(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	tgt := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "test-channel",
		ConfigDir: filepath.Join(filepath.Dir(wsRoot), ".config", "custom-agent"),
		Paths: map[string]string{
			"skills": "${config_dir}/skills",
			"rules":  "${config_dir}/rules",
		},
	}
	_ = svc.TargetAddConfig("custom-agent", tgt)

	m := tui.NewModel(ctx, svc)

	// Switch to tab 4 (Channels)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view := m2.View()

	// The compact summary replaces the old deploy block: channel name, config
	// dir path and skill count. Unsupported channels show no sync badge/table.
	if !strings.Contains(view, "custom-agent") {
		t.Fatalf("expected channel name 'custom-agent' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "SKILLS:") {
		t.Fatalf("expected SKILLS count in compact summary, got:\n%s", view)
	}
	if !strings.Contains(view, ".config/custom-agent") {
		t.Fatalf("expected config dir path in compact summary, got:\n%s", view)
	}
	if strings.Contains(view, "Data sync") || strings.Contains(view, "数据同步") {
		t.Fatalf("unsupported channel should not show a data sync badge, got:\n%s", view)
	}
}

func TestTUIChannelEnableDisableAndFilter(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	tgt1 := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "agent-a",
		ConfigDir: filepath.Join(filepath.Dir(wsRoot), ".config", "agent-a"),
		Paths:     map[string]string{"skills": "${config_dir}/skills"},
	}
	tgt2 := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "agent-b",
		ConfigDir: filepath.Join(filepath.Dir(wsRoot), ".config", "agent-b"),
		Paths:     map[string]string{"skills": "${config_dir}/skills"},
	}
	_ = svc.TargetAddConfig("agent-a", tgt1)
	_ = svc.TargetAddConfig("agent-b", tgt2)

	m := tui.NewModel(ctx, svc)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

	// Initially both are enabled and filter is Enabled Only (default)
	view := m2.View()
	if !strings.Contains(view, "agent-a") || !strings.Contains(view, "agent-b") {
		t.Fatalf("expected both agent-a and agent-b in view, got:\n%s", view)
	}

	// Press 't' on agent-a to toggle it off (disable)
	mDisabled, disableCmd := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	mDisabled = pump(mDisabled, disableCmd)
	if svc.IsTargetEnabled("agent-a") {
		t.Fatalf("expected agent-a to be disabled in config")
	}

	// Since default filter is Enabled Only, agent-a should immediately disappear from view
	if names := mDisabled.(*tui.Model).ChannelNamesForTest(); len(names) != 1 || names[0] != "agent-b" {
		t.Fatalf("expected only agent-b in Enabled-only filter, got %v", names)
	}

	// Press 'f' to switch filter to Disabled Only
	mFilterDisabled, _ := mDisabled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if names := mFilterDisabled.(*tui.Model).ChannelNamesForTest(); len(names) != 1 || names[0] != "agent-a" {
		t.Fatalf("expected only agent-a in Disabled-only filter, got %v", names)
	}

	// Press 'f' again to switch filter to All
	mFilterAll, _ := mFilterDisabled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if names := mFilterAll.(*tui.Model).ChannelNamesForTest(); len(names) != 2 {
		t.Fatalf("expected both channels in All filter, got %v", names)
	}

	// Test detail modal toggle on agent-a
	mDetail, openDetailCmd := mFilterAll.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	mDetail = pump(mDetail, openDetailCmd)
	viewDetail := mDetail.View()
	if !strings.Contains(viewDetail, "Target Details - agent-a") {
		t.Fatalf("expected agent-a detail modal, got:\n%s", viewDetail)
	}
	// Toggle in modal with Space
	mDetailToggled, detailToggleCmd := mDetail.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	mDetailToggled = pump(mDetailToggled, detailToggleCmd)
	if !svc.IsTargetEnabled("agent-a") {
		t.Fatalf("expected agent-a to be re-enabled after Space in detail modal")
	}
	// Close detail modal with Enter
	mBackToList, _ := mDetailToggled.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Press 'f' to cycle filter to Enabled Only
	mFilterBackEnabled, _ := mBackToList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	viewBackEnabled := mFilterBackEnabled.View()
	if !strings.Contains(viewBackEnabled, "agent-a") || !strings.Contains(viewBackEnabled, "agent-b") {
		t.Fatalf("both agent-a and agent-b should be visible in Enabled-only filter now, got:\n%s", viewBackEnabled)
	}
}

func TestTUIProjectEnableDisableAndFilter(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	p1 := filepath.Join(filepath.Dir(wsRoot), "project-one")
	p2 := filepath.Join(filepath.Dir(wsRoot), "project-two")
	_ = os.MkdirAll(p1, 0755)
	_ = os.MkdirAll(p2, 0755)

	_ = svc.ProjectAdd(p1)
	_ = svc.ProjectAdd(p2)

	m := tui.NewModel(ctx, svc)
	// Go to Projects tab
	mProj, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})
	viewProj := mProj.View()

	// Verify project table headers
	if !strings.Contains(viewProj, "STATUS") || !strings.Contains(viewProj, "PROJECT") || !strings.Contains(viewProj, "PROJECT PATH") || !strings.Contains(viewProj, "FORMATS") || !strings.Contains(viewProj, "SKILLS") {
		t.Fatalf("expected 5 project table headers, got:\n%s", viewProj)
	}

	// Disable project-one via service
	_ = svc.SetProjectEnabled(p1, false)
	mProjReload, _ := mProj.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'5'}})

	// Since default filter is Enabled Only, disabled project-one should NOT appear
	viewEnabled := mProjReload.View()
	if strings.Contains(viewEnabled, "project-one") {
		t.Fatalf("disabled project-one should not appear in default Enabled-only filter, got:\n%s", viewEnabled)
	}
	if !strings.Contains(viewEnabled, "project-two") {
		t.Fatalf("enabled project-two should appear in Enabled-only filter, got:\n%s", viewEnabled)
	}

	// Press 'f' for Disabled Only
	mFilterDisabled, _ := mProjReload.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	viewDisabled := mFilterDisabled.View()
	if !strings.Contains(viewDisabled, "project-one") {
		t.Fatalf("disabled project-one should appear in Disabled filter, got:\n%s", viewDisabled)
	}
	if strings.Contains(viewDisabled, "project-two") {
		t.Fatalf("enabled project-two should NOT appear in Disabled filter, got:\n%s", viewDisabled)
	}

	// Press 'f' for All
	mFilterAll, _ := mFilterDisabled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	viewAll := mFilterAll.View()
	if !strings.Contains(viewAll, "project-one") || !strings.Contains(viewAll, "project-two") {
		t.Fatalf("both project-one and project-two should appear in All filter, got:\n%s", viewAll)
	}
}

func TestTUIDeployModalExcludesDisabledTargets(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "deployable-skill")
	_ = svc.TargetAdd("enabled-agent", "/tmp/enabled-agent")
	_ = svc.TargetAdd("disabled-agent", "/tmp/disabled-agent")
	_ = svc.SetTargetEnabled("disabled-agent", false)

	pEnabled := filepath.Join(filepath.Dir(wsRoot), "enabled-proj")
	pDisabled := filepath.Join(filepath.Dir(wsRoot), "disabled-proj")
	_ = os.MkdirAll(pEnabled, 0755)
	_ = os.MkdirAll(pDisabled, 0755)
	_ = svc.ProjectAdd(pEnabled)
	_ = svc.ProjectAdd(pDisabled)
	_ = svc.SetProjectEnabled(pDisabled, false)

	m := newTestSkillsModel(ctx, svc)
	// Press 'd' to open deploy modal
	mDeploy, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	// Press Enter to go to step 2 (select targets)
	mStep2, _ := mDeploy.Update(tea.KeyMsg{Type: tea.KeyEnter})
	viewStep2 := mStep2.View()

	// Should contain enabled-agent, but NOT disabled-agent
	if !strings.Contains(viewStep2, "enabled-agent") {
		t.Fatalf("expected enabled-agent in deploy targets, got:\n%s", viewStep2)
	}
	if strings.Contains(viewStep2, "disabled-agent") {
		t.Fatalf("disabled-agent should NOT be in deploy targets, got:\n%s", viewStep2)
	}

	// Switch to projects tab in deploy modal
	mStep2Proj, _ := mStep2.Update(tea.KeyMsg{Type: tea.KeyRight})
	viewStep2Proj := mStep2Proj.View()
	if !strings.Contains(viewStep2Proj, "enabled-proj") {
		t.Fatalf("expected enabled-proj in deploy projects, got:\n%s", viewStep2Proj)
	}
	if strings.Contains(viewStep2Proj, "disabled-proj") {
		t.Fatalf("disabled-proj should NOT be in deploy projects, got:\n%s", viewStep2Proj)
	}
}

func TestTUIProjectNoImplicitCurrentDirectory(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	// Initially, NO projects are added
	m := tui.NewModel(ctx, svc)
	// Go to targets tab
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	// Enter the secondary level and switch to Project channel
	mFocus, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	mProj, _ := mFocus.Update(tea.KeyMsg{Type: tea.KeyRight})
	viewProj := mProj.View()

	// Must NOT contain Current Directory or any implicit items
	if strings.Contains(viewProj, "Current Directory") {
		t.Fatalf("expected no implicit Current Directory in project list, got:\n%s", viewProj)
	}
	if !strings.Contains(viewProj, "No project paths cached") && !strings.Contains(viewProj, "未缓存任何项目路径") {
		t.Fatalf("expected empty project list state, got:\n%s", viewProj)
	}

	// Now explicitly add a project
	sampleProj := filepath.Join(filepath.Dir(wsRoot), "my-app")
	_ = os.MkdirAll(sampleProj, 0755)
	_ = svc.ProjectAdd(sampleProj)

	mReload, _ := mProj.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	mReload, _ = mReload.Update(tea.KeyMsg{Type: tea.KeyTab})
	mReload, _ = mReload.Update(tea.KeyMsg{Type: tea.KeyRight})
	viewReload := mReload.View()

	if !strings.Contains(viewReload, "my-app") {
		t.Fatalf("expected explicitly added my-app in project list, got:\n%s", viewReload)
	}

	// Press 'x' to remove it -> should open confirmation modal, NOT give "Cannot remove"
	mRemove, _ := mReload.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	viewRemove := mRemove.View()
	if strings.Contains(viewRemove, "Cannot remove current directory") {
		t.Fatalf("unexpected error removing project, got:\n%s", viewRemove)
	}
	if !strings.Contains(viewRemove, "Remove Project") && !strings.Contains(viewRemove, "移除项目工程") {
		t.Fatalf("expected remove project confirmation modal, got:\n%s", viewRemove)
	}
}

func TestTUITargetDeployDirectLockedFlowAgent(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-one")
	tgt := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "my-agent",
		ConfigDir: filepath.Join(filepath.Dir(wsRoot), ".config", "my-agent"),
		Paths:     map[string]string{"skills": "${config_dir}/skills"},
	}
	_ = svc.TargetAddConfig("my-agent", tgt)

	m := tui.NewModel(ctx, svc)
	// Go to targets tab
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

	// Press 'd' directly on the agent target
	mDeploy, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	viewDeploy := mDeploy.View()

	// Should open deploy modal with locked target title/hint
	if !strings.Contains(viewDeploy, "my-agent") {
		t.Fatalf("expected locked target my-agent in deploy view, got:\n%s", viewDeploy)
	}
	if !strings.Contains(viewDeploy, "Directly") && !strings.Contains(viewDeploy, "直接分发") && !strings.Contains(viewDeploy, "locked") && !strings.Contains(viewDeploy, "锁定") {
		t.Fatalf("expected locked target hint in deploy view, got:\n%s", viewDeploy)
	}

	// Press Space to toggle select skill-one
	mDeploy, _ = mDeploy.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	// Press Enter -> should directly trigger deploy (executeAgentDeploy) and return async command, skipping select target step!
	mAfterEnter, cmd := mDeploy.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected async deploy command to be triggered directly on Enter")
	}

	// The view should return to list and be loading
	viewAfterEnter := mAfterEnter.View()
	if strings.Contains(viewAfterEnter, "Select Target") || strings.Contains(viewAfterEnter, "选择分发目标") {
		t.Fatalf("target was locked! Should NOT show target selection step, got:\n%s", viewAfterEnter)
	}
}

func TestTUITargetDeployDirectLockedFlowProject(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-proj")
	projDir := filepath.Join(filepath.Dir(wsRoot), "target-proj")
	_ = os.MkdirAll(projDir, 0755)
	_ = svc.ProjectAdd(projDir)

	m := tui.NewModel(ctx, svc)
	// Go to targets tab
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	// Enter the secondary level and switch to Project subtab
	mFocus, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	mProj, _ := mFocus.Update(tea.KeyMsg{Type: tea.KeyRight})

	// Press 'd' on project target
	mDeploy, _ := mProj.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	viewDeploy := mDeploy.View()

	if !strings.Contains(viewDeploy, "target-proj") {
		t.Fatalf("expected locked target target-proj in deploy view, got:\n%s", viewDeploy)
	}

	// Press Space to select skill-proj
	mDeploy, _ = mDeploy.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	// Press Enter -> should go straight to DeployStepSelectFormat, skipping DeployStepSelectTarget!
	mFormat, _ := mDeploy.Update(tea.KeyMsg{Type: tea.KeyEnter})
	viewFormat := mFormat.View()

	if !strings.Contains(viewFormat, "standard") && !strings.Contains(viewFormat, "auto") {
		t.Fatalf("expected project format selection step, got:\n%s", viewFormat)
	}
	if strings.Contains(viewFormat, "Select Target") || strings.Contains(viewFormat, "选择分发目标") {
		t.Fatalf("target selection step should have been skipped, got:\n%s", viewFormat)
	}

	// Press Esc in format step -> should return to DeployStepSelectSkills (not to target selection!)
	mBack, _ := mFormat.Update(tea.KeyMsg{Type: tea.KeyEsc})
	viewBack := mBack.View()
	if !strings.Contains(viewBack, "target-proj") {
		t.Fatalf("expected back to skills selection with locked target, got:\n%s", viewBack)
	}
}

func TestTUITargetDeployDisabledTargetBlocked(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-one")
	tgt := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "disabled-agent",
		ConfigDir: filepath.Join(filepath.Dir(wsRoot), ".config", "disabled-agent"),
		Paths:     map[string]string{"skills": "${config_dir}/skills"},
	}
	_ = svc.TargetAddConfig("disabled-agent", tgt)
	_ = svc.SetTargetEnabled("disabled-agent", false)

	m := tui.NewModel(ctx, svc)
	// Go to targets tab
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	// Switch filter to Disabled Only so we can see and select it
	mDisabledFilter, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	// Press 'd' on disabled agent target -> should block and show notice
	mDeployAttempt, _ := mDisabledFilter.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	viewAttempt := mDeployAttempt.View()

	if !strings.Contains(viewAttempt, "disabled") && !strings.Contains(viewAttempt, "禁用") {
		t.Fatalf("expected disabled warning notice when pressing 'd' on disabled target, got:\n%s", viewAttempt)
	}
	if strings.Contains(viewAttempt, "Skills List") || strings.Contains(viewAttempt, "技能列表") {
		t.Fatalf("deploy modal should not open for disabled target, got:\n%s", viewAttempt)
	}
}

func TestTUITargetUndeployDisabledTargetAllowedAndRecordDriven(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	_ = svc.NewSkill(ctx, "skill-one")
	tgt := model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "disabled-agent",
		ConfigDir: filepath.Join(filepath.Dir(wsRoot), ".config", "disabled-agent"),
		Paths:     map[string]string{"skills": "${config_dir}/skills"},
	}
	_ = svc.TargetAddConfig("disabled-agent", tgt)

	// Deploy skill-one while enabled
	if err := svc.Deploy(ctx, "skill-one", "disabled-agent", false); err != nil {
		t.Fatalf("initial deploy failed: %v", err)
	}

	// Disable target
	_ = svc.SetTargetEnabled("disabled-agent", false)

	// Also simulate skill-two deployed in state but deleted from workspace
	skillsDir := tgt.SkillsDir()
	skillTwoDir := filepath.Join(skillsDir, "skill-two")
	_ = os.MkdirAll(skillTwoDir, 0755)
	_ = os.WriteFile(filepath.Join(skillTwoDir, "SKILL.md"), []byte("---\nname: skill-two\ndescription: removed from ws\n---"), 0644)
	// Record deployment in state
	statePath := filepath.Join(filepath.Dir(wsRoot), "deployments.json")
	stateMgr, err := state.NewManager(statePath)
	if err == nil {
		_ = stateMgr.RecordDeployment("disabled-agent", "skill-two", "dummy-hash")
	}

	m := tui.NewModel(ctx, svc)
	// Go to targets tab
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	// Switch filter to disabled only
	mDisabledFilter, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})

	// Press 'D' (undeploy) on disabled agent target -> should NOT block!
	mUndeploy, _ := mDisabledFilter.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	viewUndeploy := mUndeploy.View()

	// Should open undeploy modal
	if !strings.Contains(viewUndeploy, "Undeploy") && !strings.Contains(viewUndeploy, "卸载") {
		t.Fatalf("expected undeploy modal to open for disabled target, got:\n%s", viewUndeploy)
	}
	// Should contain skill-one
	if !strings.Contains(viewUndeploy, "skill-one") {
		t.Fatalf("expected skill-one in undeploy list, got:\n%s", viewUndeploy)
	}
}

func TestTUIModelsTabPreviewConfirmFlow(t *testing.T) {
	ctx := context.Background()
	svc, wsRoot, cleanup := setupTestService(t)
	defer cleanup()

	// Mock XDG_CACHE_HOME with local catalog containing openai/gpt-5
	tmpCache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpCache)
	asoulCacheDir := filepath.Join(tmpCache, "asoul")
	_ = os.MkdirAll(asoulCacheDir, 0755)
	mockCatalog := []byte(`{
		"openai": {
			"id": "openai",
			"name": "OpenAI",
			"models": {
				"gpt-5": {
					"id": "openai/gpt-5",
					"name": "GPT-5",
					"limit": {"context": 400000, "output": 128000},
					"cost": {"input": 5.0, "output": 15.0},
					"reasoning": true
				}
			}
		}
	}`)
	if err := os.WriteFile(filepath.Join(asoulCacheDir, "models_dev.json"), mockCatalog, 0644); err != nil {
		t.Fatal(err)
	}

	// Create test opencode.json in workspace with un-enriched gpt-5
	opencodePath := filepath.Join(wsRoot, "opencode.json")
	initialConfig := []byte(`{
		"provider": {
			"custom-ai": {
				"models": {
					"gpt-5": {}
				}
			}
		}
	}`)
	if err := os.WriteFile(opencodePath, initialConfig, 0644); err != nil {
		t.Fatal(err)
	}
	_ = svc.TargetAdd("opencode", wsRoot)

	m := tui.NewModel(ctx, svc)

	// 1. Switch to tab 4 (Channels)
	m8, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	view8 := m8.View()
	// Should show pending provider badge [待补全: openai]
	if !strings.Contains(view8, "openai") {
		t.Fatalf("expected view to show matched data source 'openai', got:\n%s", view8)
	}

	// 2. Press i to view ModelDetailModal
	mEnter, _ := m8.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if mEnter.(*tui.Model).CurrentViewForTest() != tui.ViewModalModelDetailForTest {
		t.Fatalf("expected view to be ViewModalModelDetail (%d), got %d", tui.ViewModalModelDetailForTest, mEnter.(*tui.Model).CurrentViewForTest())
	}
	viewModal := mEnter.View()
	if !strings.Contains(viewModal, "gpt-5") {
		t.Fatalf("expected detail modal to show model ID gpt-5, got:\n%s", viewModal)
	}

	// Press Esc to close model detail modal
	mList, _ := mEnter.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if mList.(*tui.Model).CurrentViewForTest() != tui.ViewListForTest {
		t.Fatalf("expected view to return to ViewList, got %d", mList.(*tui.Model).CurrentViewForTest())
	}

	// 2b. Press 'e' to trigger DryRun preview
	mDry, cmdDryRun := mList.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmdDryRun == nil {
		t.Fatal("expected tea.Cmd for dry run enrich, got nil")
	}

	msgDryRun := cmdDryRun()
	mDiff, _ := mDry.Update(msgDryRun)
	viewDiff := mDiff.View()

	if !strings.Contains(viewDiff, "Unified Diff") {
		t.Fatalf("expected diff view to open, got:\n%s", viewDiff)
	}
	if !strings.Contains(viewDiff, "Confirm & Backup") && !strings.Contains(viewDiff, "确认写入并备份") {
		t.Fatalf("expected confirm footer in diff view, got:\n%s", viewDiff)
	}

	// 3. Test cancel with 'n'
	mCancel, _ := mDiff.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	viewCancel := mCancel.View()
	if strings.Contains(viewCancel, "Unified Diff") {
		t.Fatalf("expected 'n' to exit diff view")
	}
	// Verify file was NOT modified
	currBytes, _ := os.ReadFile(opencodePath)
	if string(currBytes) != string(initialConfig) {
		t.Fatalf("file should not have been modified on cancel")
	}

	// 4. Re-open diff with 'e' and confirm with 'y'
	mE, cmdDryRun2 := mCancel.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmdDryRun2 == nil {
		t.Fatal("expected cmd on 'e'")
	}
	mDiff2, _ := mE.Update(cmdDryRun2())
	mWrite, cmdWrite := mDiff2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmdWrite == nil {
		t.Fatal("expected cmd on 'y' confirm")
	}

	// Execute write command
	msgWrite := cmdWrite()
	mDone, _ := mWrite.Update(msgWrite)
	viewDone := mDone.View()

	// Should show success notice mentioning data source and backup
	if !strings.Contains(viewDone, "openai") {
		t.Fatalf("expected success notice with data source 'openai', got:\n%s", viewDone)
	}

	// Verify file is now enriched on disk
	enrichedBytes, _ := os.ReadFile(opencodePath)
	if !strings.Contains(string(enrichedBytes), "400000") {
		t.Fatalf("expected enriched limit context 400000 in file, got:\n%s", string(enrichedBytes))
	}

	// Verify view now displays [已补全: openai]
	if !strings.Contains(viewDone, "openai") {
		t.Fatalf("expected [已补全: openai] badge in view, got:\n%s", viewDone)
	}
}

func TestDoctorViewLocalization(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	i18n.Init("zh-CN")
	defer i18n.Init("en")

	m := tui.NewModel(ctx, svc)
	// NewModel may re-init the language from config; force zh-CN for rendering.
	i18n.Init("zh-CN")

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})
	newM, _ = newM.Update(tea.KeyMsg{Type: tea.KeyTab})
	newM, doctorCmd := newM.Update(tea.KeyMsg{Type: tea.KeyRight})
	newM = pump(newM, doctorCmd)
	view := newM.View()

	if !strings.Contains(view, "诊断项") {
		t.Fatalf("expected Chinese doctor table header, got:\n%s", view)
	}
	if strings.Contains(view, "CHECK ITEM") || strings.Contains(view, "table.header.") ||
		strings.Contains(view, "doctor.msg.") || strings.Contains(view, "doctor.check.") {
		t.Fatalf("raw or English i18n key leaked into Chinese doctor view:\n%s", view)
	}
}
