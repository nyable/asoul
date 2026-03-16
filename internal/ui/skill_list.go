package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/skill"
)

// ==================== 所有 SKILL 列表 ====================

type skillListModel struct {
	cfg   *config.Config
	list  list.Model
	isErr bool
	err   error
}

func newSkillList(cfg *config.Config) skillListModel {
	skills, err := skill.ScanSkills(cfg.SkillsPath)
	if err != nil {
		return skillListModel{cfg: cfg, isErr: true, err: err}
	}

	items := make([]list.Item, 0, len(skills))
	for _, s := range skills {
		items = append(items, ViewSkillItem{
			IDStr:   s.ID(),
			NameStr: s.DisplayName(),
			DescStr: s.Description,
		})
	}

	// 默认宽高 80x20
	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = "所有可用 SKILLS"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	// 自定义 help
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "查看详情")),
		}
	}

	return skillListModel{cfg: cfg, list: l}
}

func (m skillListModel) Init() tea.Cmd {
	return nil
}

func (m skillListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			return newSkillManager(m.cfg), nil
		case "enter":
			// 查看详情
			i, ok := m.list.SelectedItem().(ViewSkillItem)
			if ok {
				return newSkillDetail(m.cfg, i.IDStr, m), nil
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

func (m skillListModel) View() string {
	if m.isErr {
		return fmt.Sprintf("错误: %v\n按任意键返回", m.err)
	}
	return m.list.View()
}

// ==================== SKILL 详情视图 ====================

type skillDetailModel struct {
	cfg    *config.Config
	skill  skill.Skill
	parent tea.Model
	err    error
	width  int
}

func newSkillDetail(cfg *config.Config, skillID string, parent tea.Model) skillDetailModel {
	// 获取 SKILL 详情
	var s skill.Skill
	var err error

	skills, _, err := skill.GetSkillsByIDs(cfg.SkillsPath, []string{skillID})
	if err != nil || len(skills) == 0 {
		err = fmt.Errorf("无法加载 SKILL 详情: %v", err)
	} else {
		s = skills[0]
	}

	return skillDetailModel{
		cfg:    cfg,
		skill:  s,
		parent: parent,
		err:    err,
		width:  80, // 默认宽度
	}
}

func (m skillDetailModel) Init() tea.Cmd {
	return nil
}

func (m skillDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "enter", "backspace":
			return m.parent, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
	}
	return m, nil
}

func (m skillDetailModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("错误: %v\n\n(按 Enter 或 Esc 返回)", m.err)
	}

	// 限制宽度以保持美观
	renderWidth := m.width
	if renderWidth > 100 {
		renderWidth = 100
	}
	if renderWidth < 50 {
		renderWidth = 50
	}

	return RenderSkillDetail(m.skill, renderWidth) + "\n\n" + helpStyle.Render("(按 Enter 或 Esc 返回)")
}
