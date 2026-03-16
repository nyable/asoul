package ui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/installer"
	"github.com/nyable/asoul/internal/skill"
)

// ==================== 项目选择器 ====================

type projectSelectorModel struct {
	cfg       *config.Config
	textInput textinput.Model
	err       error
}

func newProjectSelector(cfg *config.Config) projectSelectorModel {
	ti := textinput.New()
	ti.Placeholder = "例如: ./my-project"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 40

	// 尝试获取当前目录作为默认值
	wd, _ := os.Getwd()
	ti.SetValue(wd)

	return projectSelectorModel{
		cfg:       cfg,
		textInput: ti,
	}
}

func (m projectSelectorModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m projectSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return newSkillManager(m.cfg), nil
		case tea.KeyEnter:
			path := m.textInput.Value()
			if path == "" {
				m.err = fmt.Errorf("路径不能为空")
				return m, nil
			}
			return newInstalledList(m.cfg, path), nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m projectSelectorModel) View() string {
	errStr := errorMessage(m.err)
	if errStr != "" {
		errStr = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(errStr)
	}

	return fmt.Sprintf(
		"\n%s\n\n%s\n%s\n\n(按 Enter 确认, Esc 返回)",
		titleStyle.Render("选择目标项目"),
		m.textInput.View(),
		errStr,
	)
}

// ==================== 已安装 SKILLS 列表 ====================

type installedListModel struct {
	cfg         *config.Config
	projectPath string
	list        list.Model
	installer   *installer.Installer
	message     string
}

func newInstalledList(cfg *config.Config, projectPath string) installedListModel {
	inst := installer.NewInstaller(cfg.SkillsPath, projectPath, installer.ModeCopy)

	// 获取已安装列表
	installedIDs, err := inst.ListInstalled()
	if err != nil {
		installedIDs = []string{}
	}

	// 尝试获取 SKILL 详情
	skillsMap := make(map[string]skill.Skill)
	if len(installedIDs) > 0 {
		skills, _, _ := skill.GetSkillsByIDs(cfg.SkillsPath, installedIDs)
		for _, s := range skills {
			skillsMap[s.ID()] = s
		}
	}

	items := make([]list.Item, 0, len(installedIDs))
	for _, id := range installedIDs {
		name := id
		desc := "已安装的 SKILL"

		if s, ok := skillsMap[id]; ok {
			name = s.DisplayName()
			desc = s.Description
		}

		items = append(items, ViewSkillItem{
			IDStr:   id,
			NameStr: name,
			DescStr: desc,
		})
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = fmt.Sprintf("已安装 SKILLS: %s", projectPath)
	l.SetShowStatusBar(true)

	// 自定义按键帮助
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "卸载")),
		}
	}
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "卸载")),
		}
	}

	return installedListModel{
		cfg:         cfg,
		projectPath: projectPath,
		list:        l,
		installer:   inst,
	}
}

func (m installedListModel) Init() tea.Cmd {
	return nil
}

func (m installedListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			return newProjectSelector(m.cfg), nil
		case "x", "delete":
			// 卸载选中
			if selected, ok := m.list.SelectedItem().(ViewSkillItem); ok {
				// 执行卸载
				removed, _ := m.installer.Uninstall([]string{selected.IDStr})
				if len(removed) > 0 {
					m.message = fmt.Sprintf("已卸载: %s", selected.IDStr)
					// 这行如果出错，说明 list 为空
					if len(m.list.Items()) > 0 {
						idx := m.list.Index()
						m.list.RemoveItem(idx)
						// 选中前一个
						if idx > 0 {
							m.list.Select(idx - 1)
						} else {
							m.list.ResetSelected()
						}
					}
				} else {
					m.message = "卸载失败"
				}
				return m, nil
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

func (m installedListModel) View() string {
	msgStr := ""
	if m.message != "" {
		msgStr = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("204")).Render(m.message)
	}
	return m.list.View() + msgStr
}
