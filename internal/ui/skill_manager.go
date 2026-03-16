package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nyable/asoul/internal/config"
)

// ==================== SKILL 管理菜单 ====================

type skillManagerModel struct {
	cfg  *config.Config
	list list.Model
}

func newSkillManager(cfg *config.Config) skillManagerModel {
	items := []list.Item{
		menuItem{title: "1. 查看所有 SKILLS", desc: "查看仓库中所有可用的 SKILLS"},
		menuItem{title: "2. 查看已安装 SKILLS", desc: "查看并管理项目中已安装的 SKILLS"},
		menuItem{title: "0. 返回主菜单", desc: "返回上一级"},
	}

	// 设置默认宽高，防止未收到 WindowSizeMsg 时不显示
	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = "SKILL 管理"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return skillManagerModel{cfg: cfg, list: l}
}

func (m skillManagerModel) Init() tea.Cmd {
	return nil
}

func (m skillManagerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc": // 返回主菜单
			return newMainMenu(m.cfg), nil
		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if ok {
				switch i.title {
				case "1. 查看所有 SKILLS":
					return newSkillList(m.cfg), nil
				case "2. 查看已安装 SKILLS":
					return newProjectSelector(m.cfg), nil // 先选择项目
				case "0. 返回主菜单":
					return newMainMenu(m.cfg), nil
				}
			}
		default:
			// 快速定位
			key := msg.String()
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

func (m skillManagerModel) View() string {
	return m.list.View()
}
