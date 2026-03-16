package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nyable/asoul/internal/config"
)

// ==================== 菜单项 ====================

type menuItem struct {
	title string
	desc  string
}

func (m menuItem) Title() string       { return m.title }
func (m menuItem) Description() string { return m.desc }
func (m menuItem) FilterValue() string { return m.title }

// ==================== 主菜单 ====================

type mainMenuModel struct {
	cfg      *config.Config
	list     list.Model
	quitting bool
}

func newMainMenu(cfg *config.Config) mainMenuModel {
	items := []list.Item{
		menuItem{title: "1. 安装 SKILLS", desc: "安装 SKILLS 到目标项目"},
		menuItem{title: "2. 卸载 SKILLS", desc: "从目标项目卸载 SKILLS"},
		menuItem{title: "3. 管理 SKILLS", desc: "查看所有 SKILLS 和项目已安装的 SKILLS"},
		menuItem{title: "4. 管理分组", desc: "创建、修改、删除分组"},
		menuItem{title: "5. 配置设置", desc: "查看和修改配置"},
		menuItem{title: "0. 退出", desc: "退出程序"},
	}

	l := list.New(items, list.NewDefaultDelegate(), 50, 15)
	l.Title = "asoul - SKILLS 管理工具"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return mainMenuModel{cfg: cfg, list: l}
}

func (m mainMenuModel) Init() tea.Cmd {
	return nil
}

func (m mainMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if ok {
				switch i.title {
				case "1. 安装 SKILLS":
					return newInstallModeSelector(m.cfg), nil
				case "2. 卸载 SKILLS":
					return newUninstallModeSelector(m.cfg), nil
				case "3. 管理 SKILLS":
					return newSkillManager(m.cfg), nil
				case "4. 管理分组":
					return newGroupMenu(m.cfg), nil
				case "5. 配置设置":
					return newConfigMenu(m.cfg), nil
				case "0. 退出":
					m.quitting = true
					return m, tea.Quit
				}
			}
		default:
			// 快速定位：根据序号匹配
			key := msg.String()
			// 简单的数字检查
			if len(key) == 1 && key >= "0" && key <= "9" {
				for i, item := range m.list.Items() {
					if mItem, ok := item.(menuItem); ok {
						if strings.HasPrefix(mItem.title, key+".") {
							m.list.Select(i)
							break
						}
					}
				}
			}
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 2)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m mainMenuModel) View() string {
	if m.quitting {
		return "再见！\n"
	}
	return m.list.View()
}
