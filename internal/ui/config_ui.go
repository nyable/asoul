package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nyable/asoul/internal/config"
)

// ==================== 配置菜单 ====================

type configMenuModel struct {
	cfg  *config.Config
	list list.Model
}

func newConfigMenu(cfg *config.Config) configMenuModel {
	items := []list.Item{
		menuItem{title: "1. 查看当前配置", desc: "显示配置信息"},
		menuItem{title: "2. 设置 SKILLS 路径", desc: "修改 SKILLS 源目录"},
		menuItem{title: "0. 返回", desc: "返回主菜单"},
	}

	l := list.New(items, list.NewDefaultDelegate(), 50, 12)
	l.Title = "配置设置"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return configMenuModel{cfg: cfg, list: l}
}

func (m configMenuModel) Init() tea.Cmd { return nil }

func (m configMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return newMainMenu(m.cfg), nil
		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if ok {
				switch i.title {
				case "1. 查看当前配置":
					return newConfigView(m.cfg), nil
				case "2. 设置 SKILLS 路径":
					return newSkillsPathInput(m.cfg), nil
				case "0. 返回":
					return newMainMenu(m.cfg), nil
				}
			}
		default:
			// 数字键快速定位
			key := msg.String()
			if len(key) == 1 && key >= "1" && key <= "9" {
				idx := int(key[0] - '1')
				if idx < len(m.list.Items()) {
					m.list.Select(idx)
				}
			} else if key == "0" {
				// 0 对应最后一个"返回"
				m.list.Select(len(m.list.Items()) - 1)
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

func (m configMenuModel) View() string {
	return m.list.View()
}

// ==================== 配置查看 ====================

type configViewModel struct {
	cfg     *config.Config
	content string
}

func newConfigView(cfg *config.Config) configViewModel {
	configPath, _ := config.GetConfigPath()

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("当前配置") + "\n\n")
	sb.WriteString(fmt.Sprintf("SKILLS 路径: %s\n", cfg.SkillsPath))
	sb.WriteString(fmt.Sprintf("分组数量: %d\n", len(cfg.Groups)))
	sb.WriteString(fmt.Sprintf("配置文件: %s\n", configPath))

	return configViewModel{cfg: cfg, content: sb.String()}
}

func (m configViewModel) Init() tea.Cmd { return nil }

func (m configViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		return newConfigMenu(m.cfg), nil
	}
	return m, nil
}

func (m configViewModel) View() string {
	return m.content + "\n" + helpStyle.Render("按任意键返回...")
}

// ==================== 设置 SKILLS 路径 ====================

type skillsPathInputModel struct {
	cfg         *config.Config
	inputBuffer string
}

func newSkillsPathInput(cfg *config.Config) skillsPathInputModel {
	return skillsPathInputModel{cfg: cfg, inputBuffer: cfg.SkillsPath}
}

func (m skillsPathInputModel) Init() tea.Cmd { return nil }

func (m skillsPathInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.inputBuffer != "" {
				m.cfg.SetSkillsPath(m.inputBuffer)
				if err := m.cfg.Save(); err != nil {
					return newResultView(m.cfg, "保存失败: "+err.Error()), nil
				}
				return newResultView(m.cfg, fmt.Sprintf("SKILLS 路径已设置为:\n%s", m.inputBuffer)), nil
			}
		case "esc":
			return newConfigMenu(m.cfg), nil
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

func (m skillsPathInputModel) View() string {
	return fmt.Sprintf(
		"%s\n\n当前路径: %s\n\n输入新的 SKILLS 路径: %s\n\n%s",
		titleStyle.Render("设置 SKILLS 路径"),
		m.cfg.SkillsPath,
		m.inputBuffer+"_",
		helpStyle.Render("enter 确认 | esc 取消"),
	)
}
