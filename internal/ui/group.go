package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nyable/asoul/internal/config"
	"github.com/nyable/asoul/internal/installer"
	"github.com/nyable/asoul/internal/skill"
)

// ==================== 分组菜单 ====================

type groupMenuModel struct {
	cfg  *config.Config
	list list.Model
}

func newGroupMenu(cfg *config.Config) groupMenuModel {
	items := []list.Item{
		menuItem{title: "1. 查看分组列表", desc: "显示所有分组"},
		menuItem{title: "2. 创建分组", desc: "创建新的分组"},
		menuItem{title: "3. 修改分组", desc: "修改现有分组"},
		menuItem{title: "4. 删除分组", desc: "删除分组"},
		menuItem{title: "0. 返回", desc: "返回主菜单"},
	}

	l := list.New(items, list.NewDefaultDelegate(), 50, 15)
	l.Title = "分组管理"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return groupMenuModel{cfg: cfg, list: l}
}

func (m groupMenuModel) Init() tea.Cmd { return nil }

func (m groupMenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return newMainMenu(m.cfg), nil
		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if ok {
				switch i.title {
				case "1. 查看分组列表":
					return newGroupListView(m.cfg), nil
				case "2. 创建分组":
					return newGroupCreateModel(m.cfg), nil
				case "3. 修改分组":
					return newGroupSelectModel(m.cfg, "edit"), nil
				case "4. 删除分组":
					return newGroupSelectModel(m.cfg, "delete"), nil
				case "0. 返回":
					return newMainMenu(m.cfg), nil
				}
			}
		default:
			// 数字键快速定位
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

func (m groupMenuModel) View() string {
	return m.list.View()
}

// ==================== 分组列表 ====================

type groupListItem struct {
	group config.Group
}

func (i groupListItem) Title() string       { return i.group.Name }
func (i groupListItem) Description() string { return i.group.Description }
func (i groupListItem) FilterValue() string { return i.group.Name + " " + i.group.Description }

type groupListModel struct {
	cfg  *config.Config
	list list.Model
}

func newGroupListView(cfg *config.Config) groupListModel {
	items := make([]list.Item, 0, len(cfg.Groups))
	for _, g := range cfg.Groups {
		items = append(items, groupListItem{group: g})
	}

	// 默认宽高 80x20
	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = "分组列表"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "查看详情")),
		}
	}

	return groupListModel{cfg: cfg, list: l}
}

func (m groupListModel) Init() tea.Cmd { return nil }

func (m groupListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			return newGroupMenu(m.cfg), nil
		case "enter":
			i, ok := m.list.SelectedItem().(groupListItem)
			if ok {
				return newGroupDetailView(m.cfg, i.group, m), nil
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

func (m groupListModel) View() string {
	return m.list.View()
}

// ==================== 分组详情 ====================

type groupDetailModel struct {
	cfg    *config.Config
	group  config.Group
	parent tea.Model
	width  int
}

func newGroupDetailView(cfg *config.Config, group config.Group, parent tea.Model) groupDetailModel {
	return groupDetailModel{
		cfg:    cfg,
		group:  group,
		parent: parent,
		width:  80, // 默认宽度
	}
}

func (m groupDetailModel) Init() tea.Cmd { return nil }

func (m groupDetailModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m groupDetailModel) View() string {
	renderWidth := m.width
	if renderWidth > 100 {
		renderWidth = 100
	}
	if renderWidth < 50 {
		renderWidth = 50
	}
	return RenderGroupDetail(m.group, renderWidth) + "\n\n" + helpStyle.Render("按 Enter 或 Esc 返回")
}

// ==================== 分组创建 ====================

type groupCreateModel struct {
	cfg         *config.Config
	step        int // 0: 输入名称, 1: 输入描述, 2: 选择 SKILLS
	name        string
	description string
	inputBuffer string
	skills      []skillItem
	list        list.Model
	message     string
}

func newGroupCreateModel(cfg *config.Config) groupCreateModel {
	allSkills, _ := skill.ScanSkills(cfg.SkillsPath)
	items := make([]skillItem, len(allSkills))
	listItems := make([]list.Item, len(allSkills))
	for i, s := range allSkills {
		items[i] = skillItem{skill: s, selected: false, index: i}
		listItems[i] = items[i]
	}

	l := list.New(listItems, list.NewDefaultDelegate(), 80, 15)
	l.Title = "选择要包含的 SKILLS"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	return groupCreateModel{cfg: cfg, step: 0, skills: items, list: l}
}

func (m groupCreateModel) Init() tea.Cmd { return nil }

func (m groupCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.step {
		case 0, 1: // 输入名称或描述
			switch msg.String() {
			case "enter":
				if m.step == 0 {
					if m.inputBuffer == "" {
						m.message = "名称不能为空"
						return m, nil
					}
					if m.cfg.GetGroup(m.inputBuffer) != nil {
						m.message = "分组已存在"
						return m, nil
					}
					m.name = m.inputBuffer
					m.inputBuffer = ""
					m.step = 1
					m.message = ""
				} else {
					m.description = m.inputBuffer
					m.inputBuffer = ""
					m.step = 2
				}
			case "esc":
				return newGroupMenu(m.cfg), nil
			case "backspace":
				if len(m.inputBuffer) > 0 {
					m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.inputBuffer += msg.String()
				}
			}
		case 2: // 选择 SKILLS
			switch msg.String() {
			case "esc":
				return newGroupMenu(m.cfg), nil
			case " ":
				if i := m.list.Index(); i >= 0 && i < len(m.skills) {
					m.skills[i].selected = !m.skills[i].selected
					m.updateListItems()
				}
			case "enter":
				// 保存分组
				selectedSkills := []string{}
				for _, item := range m.skills {
					if item.selected {
						selectedSkills = append(selectedSkills, item.skill.ID())
					}
				}
				m.cfg.AddGroup(config.Group{
					Name:        m.name,
					Description: m.description,
					Skills:      selectedSkills,
				})
				if err := m.cfg.Save(); err != nil {
					return newResultView(m.cfg, "保存失败: "+err.Error()), nil
				}
				return newResultView(m.cfg, fmt.Sprintf("分组 '%s' 创建成功！", m.name)), nil
			case "a":
				for i := range m.skills {
					m.skills[i].selected = true
				}
				m.updateListItems()
			case "n":
				for i := range m.skills {
					m.skills[i].selected = false
				}
				m.updateListItems()
			}
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
	}

	if m.step == 2 {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *groupCreateModel) updateListItems() {
	listItems := make([]list.Item, len(m.skills))
	for i, item := range m.skills {
		listItems[i] = item
	}
	m.list.SetItems(listItems)
}

func (m groupCreateModel) View() string {
	switch m.step {
	case 0:
		view := titleStyle.Render("创建分组") + "\n\n"
		view += "输入分组名称: " + m.inputBuffer + "_\n"
		if m.message != "" {
			view += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.message)
		}
		view += "\n\n" + helpStyle.Render("enter 确认 | esc 取消")
		return view
	case 1:
		view := titleStyle.Render("创建分组: "+m.name) + "\n\n"
		view += "输入分组描述: " + m.inputBuffer + "_\n"
		view += "\n" + helpStyle.Render("enter 确认 | esc 取消")
		return view
	case 2:
		return m.list.View() + "\n" + helpStyle.Render("space 选中 | a 全选 | n 全不选 | enter 确认 | esc 取消")
	}
	return ""
}

// ==================== 分组选择器 ====================

type groupSelectModel struct {
	cfg  *config.Config
	mode string // "edit", "delete", "install"
	list list.Model
}

func newGroupSelectModel(cfg *config.Config, mode string) groupSelectModel {
	if len(cfg.Groups) == 0 {
		// 无分组时直接返回
	}

	items := make([]list.Item, len(cfg.Groups))
	for i, g := range cfg.Groups {
		items[i] = menuItem{
			title: fmt.Sprintf("%d. %s (%d)", i+1, g.Name, len(g.Skills)),
			desc:  g.Description,
		}
	}

	title := "选择分组"
	switch mode {
	case "edit":
		title = "选择要修改的分组"
	case "delete":
		title = "选择要删除的分组"
	case "install":
		title = "选择要安装的分组"
	case "uninstall":
		title = "选择要卸载的分组"
	}

	l := list.New(items, list.NewDefaultDelegate(), 50, 15)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return groupSelectModel{cfg: cfg, mode: mode, list: l}
}

func (m groupSelectModel) Init() tea.Cmd { return nil }

func (m groupSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.cfg.Groups) == 0 {
		switch msg.(type) {
		case tea.KeyMsg:
			return newGroupMenu(m.cfg), nil
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return newGroupMenu(m.cfg), nil
		case "enter":
			idx := m.list.Index()
			if idx >= 0 && idx < len(m.cfg.Groups) {
				group := &m.cfg.Groups[idx]
				switch m.mode {
				case "edit":
					return newGroupEditModel(m.cfg, group.Name), nil
				case "delete":
					return newGroupDeleteConfirm(m.cfg, group.Name), nil
				case "install":
					return newGroupInstallModel(m.cfg, group.Name), nil
				case "uninstall":
					return newGroupUninstallModel(m.cfg, group.Name), nil
				}
			}
		default:
			// 数字键快速定位
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

func (m groupSelectModel) View() string {
	if len(m.cfg.Groups) == 0 {
		return "没有分组\n\n" + helpStyle.Render("按任意键返回...")
	}
	return m.list.View()
}

// ==================== 分组编辑 ====================

// ==================== 分组编辑 ====================

type groupEditModel struct {
	cfg          *config.Config
	originalName string
	step         int // 0: Name, 1: Desc, 2: Skills
	newName      string
	newDesc      string
	inputBuffer  string
	skills       []skillItem
	list         list.Model
	message      string
}

func newGroupEditModel(cfg *config.Config, groupName string) groupEditModel {
	group := cfg.GetGroup(groupName)
	selectedSet := make(map[string]bool)
	description := ""

	if group != nil {
		for _, s := range group.Skills {
			selectedSet[s] = true
		}
		description = group.Description
	}

	allSkills, _ := skill.ScanSkills(cfg.SkillsPath)
	items := make([]skillItem, len(allSkills))
	listItems := make([]list.Item, len(allSkills))
	for i, s := range allSkills {
		items[i] = skillItem{skill: s, selected: selectedSet[s.ID()], index: i}
		listItems[i] = items[i]
	}

	l := list.New(listItems, list.NewDefaultDelegate(), 80, 15)
	l.Title = "修改包含的 SKILLS"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	return groupEditModel{
		cfg:          cfg,
		originalName: groupName,
		step:         0,
		newName:      groupName,
		newDesc:      description,
		inputBuffer:  groupName, // 初始为原名
		skills:       items,
		list:         l,
	}
}

func (m groupEditModel) Init() tea.Cmd { return nil }

func (m groupEditModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.step {
		case 0, 1: // 编辑名称或描述
			switch msg.String() {
			case "enter":
				if m.step == 0 {
					// 验证名称
					val := strings.TrimSpace(m.inputBuffer)
					if val == "" {
						m.message = "名称不能为空"
						return m, nil
					}
					// 如果名称改变了，检查是否存在其他同名分组
					if val != m.originalName && m.cfg.GetGroup(val) != nil {
						m.message = "分组名称已存在"
						return m, nil
					}
					m.newName = val
					m.step = 1
					m.inputBuffer = m.newDesc // 切换到描述编辑
					m.message = ""
				} else {
					m.newDesc = m.inputBuffer
					m.step = 2
					m.inputBuffer = ""
				}
			case "esc":
				return newGroupMenu(m.cfg), nil
			case "backspace":
				if len(m.inputBuffer) > 0 {
					m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.inputBuffer += msg.String()
				}
			}
		case 2: // 选择 SKILLS
			switch msg.String() {
			case "esc":
				return newGroupMenu(m.cfg), nil
			case " ":
				if i := m.list.Index(); i >= 0 && i < len(m.skills) {
					m.skills[i].selected = !m.skills[i].selected
					m.updateListItems()
				}
			case "enter":
				// 保存所有修改
				// 先删除旧分组（无论是否改名，都先删除以实现覆盖更新）
				m.cfg.DeleteGroup(m.originalName)

				// 收集 skills
				selectedSkills := []string{}
				for _, item := range m.skills {
					if item.selected {
						selectedSkills = append(selectedSkills, item.skill.ID())
					}
				}

				// 添加/更新新的
				m.cfg.AddGroup(config.Group{
					Name:        m.newName,
					Description: m.newDesc,
					Skills:      selectedSkills,
				})

				if err := m.cfg.Save(); err != nil {
					return newResultView(m.cfg, "保存失败: "+err.Error()), nil
				}
				return newResultView(m.cfg, fmt.Sprintf("分组 '%s' 已更新！", m.newName)), nil
			case "a":
				for i := range m.skills {
					m.skills[i].selected = true
				}
				m.updateListItems()
			case "n":
				for i := range m.skills {
					m.skills[i].selected = false
				}
				m.updateListItems()
			}
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
	}

	if m.step == 2 {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *groupEditModel) updateListItems() {
	listItems := make([]list.Item, len(m.skills))
	for i, item := range m.skills {
		listItems[i] = item
	}
	m.list.SetItems(listItems)
}

func (m groupEditModel) View() string {
	switch m.step {
	case 0:
		view := titleStyle.Render("修改分组: "+m.originalName) + "\n\n"
		view += "修改名称: " + m.inputBuffer + "_\n"
		if m.message != "" {
			view += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.message)
		}
		view += "\n\n" + helpStyle.Render("enter 确认 | esc 取消")
		return view
	case 1:
		view := titleStyle.Render("修改分组: "+m.newName) + "\n\n"
		view += "修改描述: " + m.inputBuffer + "_\n"
		view += "\n" + helpStyle.Render("enter 确认 | esc 取消")
		return view
	case 2:
		return m.list.View() + "\n" + helpStyle.Render("space 选中 | a 全选 | n 全不选 | enter 保存 | esc 取消")
	}
	return ""
}

// ==================== 分组删除确认 ====================

type groupDeleteConfirmModel struct {
	cfg       *config.Config
	groupName string
}

func newGroupDeleteConfirm(cfg *config.Config, groupName string) groupDeleteConfirmModel {
	return groupDeleteConfirmModel{cfg: cfg, groupName: groupName}
}

func (m groupDeleteConfirmModel) Init() tea.Cmd { return nil }

func (m groupDeleteConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			if m.cfg.DeleteGroup(m.groupName) {
				if err := m.cfg.Save(); err != nil {
					return newResultView(m.cfg, "保存失败: "+err.Error()), nil
				}
				return newResultView(m.cfg, fmt.Sprintf("分组 '%s' 已删除！", m.groupName)), nil
			}
		case "n", "N", "esc":
			return newGroupMenu(m.cfg), nil
		}
	}
	return m, nil
}

func (m groupDeleteConfirmModel) View() string {
	return fmt.Sprintf(
		"%s\n\n确认删除分组 '%s'?\n\n%s",
		titleStyle.Render("删除分组"),
		m.groupName,
		helpStyle.Render("y 确认 | n 取消"),
	)
}

// ==================== 分组安装 ====================

type groupInstallModel struct {
	cfg         *config.Config
	groupName   string
	step        int // 0: 输入路径, 1: 选择模式
	targetPath  string
	inputBuffer string
	modeList    list.Model
}

func newGroupInstallModel(cfg *config.Config, groupName string) groupInstallModel {
	// 初始化安装模式选择列表
	modeItems := []list.Item{
		menuItem{title: "1. 复制模式", desc: "创建独立副本"},
		menuItem{title: "2. 链接模式", desc: "创建软链接"},
	}
	ml := list.New(modeItems, list.NewDefaultDelegate(), 50, 12)
	ml.Title = "选择安装模式"
	ml.SetShowStatusBar(false)
	ml.SetFilteringEnabled(false)

	return groupInstallModel{cfg: cfg, groupName: groupName, step: 0, inputBuffer: ".", modeList: ml}
}

func (m groupInstallModel) Init() tea.Cmd { return nil }

func (m groupInstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.step {
		case 0: // 输入路径
			switch msg.String() {
			case "enter":
				m.targetPath = m.inputBuffer
				if m.targetPath == "" {
					m.targetPath = "."
				}
				m.step = 1
			case "esc":
				return newGroupMenu(m.cfg), nil
			case "backspace":
				if len(m.inputBuffer) > 0 {
					m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.inputBuffer += msg.String()
				}
			}
		case 1: // 选择模式
			switch msg.String() {
			case "esc":
				m.step = 0
			case "enter":
				i, ok := m.modeList.SelectedItem().(menuItem)
				if ok {
					switch i.title {
					case "1. 复制模式":
						return m.doInstall(installer.ModeCopy)
					case "2. 链接模式":
						return m.doInstall(installer.ModeLink)
					}
				}
			default:
				// 数字键快速定位
				key := msg.String()
				if len(key) == 1 && key >= "1" && key <= "9" {
					idx := int(key[0] - '1')
					if idx < len(m.modeList.Items()) {
						m.modeList.Select(idx)
					}
				}
			}
		}
	case tea.WindowSizeMsg:
		if m.step == 1 {
			m.modeList.SetWidth(msg.Width)
			m.modeList.SetHeight(msg.Height - 2)
		}
	}

	if m.step == 1 {
		var cmd tea.Cmd
		m.modeList, cmd = m.modeList.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m groupInstallModel) doInstall(mode installer.Mode) (tea.Model, tea.Cmd) {
	inst := installer.NewInstaller(m.cfg.SkillsPath, m.targetPath, mode)
	installed, failed := inst.InstallGroup(m.cfg, m.groupName)

	var result string
	if len(installed) > 0 {
		result = fmt.Sprintf("成功安装: %s\n", strings.Join(installed, ", "))
	}
	for id, err := range failed {
		result += fmt.Sprintf("安装失败 [%s]: %v\n", id, err)
	}

	return newResultView(m.cfg, result), nil
}

func (m groupInstallModel) View() string {
	switch m.step {
	case 0:
		return fmt.Sprintf(
			"%s\n\n输入目标项目路径: %s\n\n%s",
			titleStyle.Render("安装分组: "+m.groupName),
			m.inputBuffer+"_",
			helpStyle.Render("enter 确认 | esc 取消"),
		)
	case 1:
		return m.modeList.View()
	}
	return ""
}

// ==================== 分组卸载 ====================

type groupUninstallModel struct {
	cfg         *config.Config
	groupName   string
	targetPath  string
	inputBuffer string
}

func newGroupUninstallModel(cfg *config.Config, groupName string) groupUninstallModel {
	return groupUninstallModel{cfg: cfg, groupName: groupName, inputBuffer: "."}
}

func (m groupUninstallModel) Init() tea.Cmd { return nil }

func (m groupUninstallModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.targetPath = m.inputBuffer
			if m.targetPath == "" {
				m.targetPath = "."
			}
			return m.doUninstall()
		case "esc":
			return newGroupMenu(m.cfg), nil
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

func (m groupUninstallModel) doUninstall() (tea.Model, tea.Cmd) {
	inst := installer.NewInstaller(m.cfg.SkillsPath, m.targetPath, installer.ModeCopy)
	removed, skipped, failed := inst.UninstallGroup(m.cfg, m.groupName)

	var result strings.Builder
	if len(removed) > 0 {
		result.WriteString(fmt.Sprintf("成功卸载: %s\n", strings.Join(removed, ", ")))
	}
	if len(skipped) > 0 {
		result.WriteString(fmt.Sprintf("已跳过（未安装）: %s\n", strings.Join(skipped, ", ")))
	}
	for id, err := range failed {
		result.WriteString(fmt.Sprintf("卸载失败 [%s]: %v\n", id, err))
	}
	if result.Len() == 0 {
		result.WriteString("分组中没有 SKILLS")
	}

	return newResultView(m.cfg, result.String()), nil
}

func (m groupUninstallModel) View() string {
	return fmt.Sprintf(
		"%s\n\n输入目标项目路径: %s\n\n%s",
		titleStyle.Render("卸载分组: "+m.groupName),
		m.inputBuffer+"_",
		helpStyle.Render("enter 确认 | esc 取消"),
	)
}
