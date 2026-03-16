package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/installer"
)

// ==================== 卸载路径输入 ====================

type uninstallPathInputModel struct {
	cfg         *config.Config
	inputBuffer string
}

func newUninstallPathInput(cfg *config.Config) uninstallPathInputModel {
	return uninstallPathInputModel{cfg: cfg, inputBuffer: "."}
}

func (m uninstallPathInputModel) Init() tea.Cmd { return nil }

func (m uninstallPathInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			targetPath := m.inputBuffer
			if targetPath == "" {
				targetPath = "."
			}
			return newUninstallSelector(m.cfg, targetPath), nil
		case "esc":
			return newMainMenu(m.cfg), nil
		case "backspace":
			if len(m.inputBuffer) > 0 {
				m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
			}
		default:
			if len(msg.String()) == 1 {
				m.inputBuffer += msg.String()
			}
		}
	}
	return m, nil
}

func (m uninstallPathInputModel) View() string {
	return fmt.Sprintf(
		"%s\n\n输入目标项目路径: %s\n\n%s",
		titleStyle.Render("卸载 SKILLS"),
		m.inputBuffer+"_",
		helpStyle.Render("enter 确认 | esc 返回"),
	)
}

// ==================== 卸载选择器 ====================

type uninstallSelectorModel struct {
	cfg        *config.Config
	targetPath string
	items      []string
	selected   map[string]bool
	cursor     int
	message    string
}

func newUninstallSelector(cfg *config.Config, targetPath string) uninstallSelectorModel {
	inst := installer.NewInstaller(cfg.SkillsPath, targetPath, installer.ModeCopy)
	installed, _ := inst.ListInstalled()

	return uninstallSelectorModel{
		cfg:        cfg,
		targetPath: targetPath,
		items:      installed,
		selected:   make(map[string]bool),
		cursor:     0,
	}
}

func (m uninstallSelectorModel) Init() tea.Cmd { return nil }

func (m uninstallSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return newMainMenu(m.cfg), nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ":
			if len(m.items) > 0 {
				id := m.items[m.cursor]
				m.selected[id] = !m.selected[id]
			}
		case "a":
			for _, id := range m.items {
				m.selected[id] = true
			}
		case "n":
			m.selected = make(map[string]bool)
		case "enter":
			selectedList := []string{}
			for id, sel := range m.selected {
				if sel {
					selectedList = append(selectedList, id)
				}
			}
			if len(selectedList) == 0 {
				m.message = "请至少选择一个 SKILL"
				return m, nil
			}
			// 执行卸载
			inst := installer.NewInstaller(m.cfg.SkillsPath, m.targetPath, installer.ModeCopy)
			removed, failed := inst.Uninstall(selectedList)

			var result string
			if len(removed) > 0 {
				result = fmt.Sprintf("成功卸载: %s\n", strings.Join(removed, ", "))
			}
			for id, err := range failed {
				result += fmt.Sprintf("卸载失败 [%s]: %v\n", id, err)
			}
			return newResultView(m.cfg, result), nil
		}
	}
	return m, nil
}

func (m uninstallSelectorModel) View() string {
	if len(m.items) == 0 {
		return titleStyle.Render("卸载 SKILLS") + "\n\n目标路径: " + m.targetPath + "\n\n没有已安装的 SKILLS\n\n" + helpStyle.Render("按任意键返回...")
	}

	view := titleStyle.Render("卸载 SKILLS") + "\n目标路径: " + m.targetPath + "\n\n"

	for i, id := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		check := "[ ]"
		if m.selected[id] {
			check = "[✓]"
		}
		view += fmt.Sprintf("%s%s %d. %s\n", cursor, check, i+1, id)
	}

	if m.message != "" {
		view += "\n" + m.message
	}

	view += "\n" + helpStyle.Render("space 选中 | a 全选 | n 全不选 | enter 确认 | esc 返回")
	return view
}
