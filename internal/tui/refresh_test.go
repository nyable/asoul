package tui_test

import (
	"context"
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

func TestTUIRefreshShortcuts(t *testing.T) {
	i18n.Init("zh-CN")
	defer i18n.Init("en")

	tmpDir := t.TempDir()
	wsRoot := filepath.Join(tmpDir, "ws")
	_ = catalog.InitWorkspace(wsRoot)

	cfgFile := filepath.Join(tmpDir, "config.jsonc")
	stateFile := filepath.Join(tmpDir, "deployments.json")
	cacheDir := filepath.Join(tmpDir, "git-cache")

	cfgMgr, _ := config.NewManager(cfgFile)
	stateMgr, _ := state.NewManager(stateFile)
	cacheMgr, _ := cache.NewManager(cacheDir)
	svc := app.NewService(wsRoot, cfgMgr, stateMgr, cacheMgr)

	ctx := context.Background()

	// 1. Create skill and configure target
	_ = svc.NewSkill(ctx, "skill-a")
	targetSkillsDir := filepath.Join(tmpDir, "custom-agent", "skills")
	_ = os.MkdirAll(targetSkillsDir, 0755)

	_ = svc.TargetAddConfig("agent-x", model.TargetConfig{
		Type:      model.TargetTypeAgent,
		Channel:   "agent-x",
		ConfigDir: filepath.Join(tmpDir, "custom-agent"),
	})
	_ = svc.Deploy(ctx, "skill-a", "agent-x", false)

	var m tea.Model = tui.NewModel(ctx, svc)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Switch to tabChannels (tab 4)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	viewTargets := m.View()

	if !strings.Contains(viewTargets, "[r] 刷新") {
		t.Fatalf("expected channels footer to contain '[r] 刷新', got:\n%s", viewTargets)
	}

	// 2. Delete skill-a on disk
	_ = os.RemoveAll(filepath.Join(targetSkillsDir, "skill-a"))

	// Press 'r' to refresh targets
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	viewAfterRefresh := m.View()

	if !strings.Contains(viewAfterRefresh, "已刷新分发目标与项目信息") {
		t.Fatalf("expected notice '已刷新分发目标与项目信息', got:\n%s", viewAfterRefresh)
	}

	// 3. Open target details modal
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	m = pump(m, cmd)
	modalView := m.View()
	if !strings.Contains(modalView, "[r] 刷新") {
		t.Fatalf("expected modal hint to contain '[r] 刷新', got:\n%s", modalView)
	}

	// Press 'r' inside modal
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = pump(m, cmd)
	modalAfterR := m.View()
	if !strings.Contains(modalAfterR, "暂无已分发") {
		t.Fatalf("expected modal to show 0 deployed skills, got:\n%s", modalAfterR)
	}

	// Close modal
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// 4. Switch to Skills > Groups and test 'r'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	viewGroups := m.View()
	if !strings.Contains(viewGroups, "[r] 刷新") {
		t.Fatalf("expected groups footer to contain '[r] 刷新', got:\n%s", viewGroups)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !strings.Contains(m.View(), "已刷新技能分组信息") {
		t.Fatalf("expected notice '已刷新技能分组信息', got:\n%s", m.View())
	}

	// 5. Switch to tabCache (tab 6) and test 'r'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})
	viewCache := m.View()
	if !strings.Contains(viewCache, "[r] 刷新") {
		t.Fatalf("expected cache footer to contain '[r] 刷新', got:\n%s", viewCache)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if !strings.Contains(m.View(), "已刷新缓存库列表与大小") {
		t.Fatalf("expected notice '已刷新缓存库列表与大小', got:\n%s", m.View())
	}
}
