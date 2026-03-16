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

// ==================== SKILL 多选器 ====================

// 全局过滤模式：true=只过滤标题，false=过滤全部（标题+描述）
var filterTitleOnly = false

type skillItem struct {
	skill    skill.Skill
	selected bool
	index    int
}

func (s skillItem) Title() string {
	check := "[ ]"
	if s.selected {
		check = "[✓]"
	}
	return fmt.Sprintf("%s %d. %s", check, s.index+1, s.skill.ID())
}

func (s skillItem) Description() string {
	return truncate(s.skill.Description, 70)
}

func (s skillItem) FilterValue() string {
	if filterTitleOnly {
		return s.skill.ID()
	}
	return s.skill.ID() + " " + s.skill.Description
}

type skillSelectorModel struct {
	cfg             *config.Config
	mode            string // "install" or "uninstall"
	items           []skillItem
	list            list.Model
	showDetail      bool
	detailSkill     *skill.Skill
	targetPath      string
	inputMode       bool
	inputBuffer     string
	installMode     installer.Mode
	selectMode      bool
	modeList        list.Model // 用于选择安装模式
	message         string
	filterTitleOnly bool             // true: 只过滤标题, false: 过滤全部
	prevFilterState list.FilterState // 用于检测过滤状态变化
}

func newSkillSelector(cfg *config.Config, mode string) skillSelectorModel {
	var skills []skill.Skill
	var err error

	if mode == "install" {
		skills, err = skill.ScanSkills(cfg.SkillsPath)
	} else {
		// 卸载模式：列出当前目录已安装的
		inst := installer.NewInstaller(cfg.SkillsPath, ".", installer.ModeCopy)
		installed, _ := inst.ListInstalled()
		for _, id := range installed {
			skills = append(skills, skill.Skill{FolderName: id, Name: id, Description: "(已安装)"})
		}
	}

	if err != nil {
		skills = []skill.Skill{}
	}

	items := make([]skillItem, len(skills))
	listItems := make([]list.Item, len(skills))
	for i, s := range skills {
		items[i] = skillItem{skill: s, selected: false, index: i}
		listItems[i] = items[i]
	}

	delegate := list.NewDefaultDelegate()
	// 禁用过滤关键字高亮，避免位置显示问题
	delegate.Styles.FilterMatch = lipgloss.NewStyle()
	l := list.New(listItems, delegate, 80, 20)

	// 自定义精确匹配过滤函数（包含匹配，而非模糊匹配）
	l.Filter = func(term string, targets []string) []list.Rank {
		term = strings.ToLower(term)
		var ranks []list.Rank
		for i, target := range targets {
			targetLower := strings.ToLower(target)
			if strings.Contains(targetLower, term) {
				ranks = append(ranks, list.Rank{Index: i, MatchedIndexes: []int{}})
			}
		}
		return ranks
	}

	if mode == "install" {
		l.Title = "选择要安装的 SKILLS"
	} else {
		l.Title = "选择要卸载的 SKILLS"
	}
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "查看详情")),
			key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "选中/取消")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "确认")),
		}
	}

	// 初始化安装模式选择列表
	modeItems := []list.Item{
		menuItem{title: "1. 复制模式", desc: "创建独立副本"},
		menuItem{title: "2. 链接模式", desc: "创建软链接"},
	}
	ml := list.New(modeItems, list.NewDefaultDelegate(), 50, 12)
	ml.Title = "选择安装模式"
	ml.SetShowStatusBar(false)
	ml.SetFilteringEnabled(false)

	return skillSelectorModel{
		cfg:         cfg,
		mode:        mode,
		items:       items,
		list:        l,
		modeList:    ml,
		installMode: installer.ModeCopy,
	}
}

func (m skillSelectorModel) Init() tea.Cmd {
	return nil
}

func (m skillSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 详情模式
	if m.showDetail {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "esc", "?":
				m.showDetail = false
				m.detailSkill = nil
			}
		}
		return m, nil
	}

	// 输入目标路径模式
	if m.inputMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				m.targetPath = m.inputBuffer
				if m.targetPath == "" {
					m.targetPath = "."
				}
				m.inputMode = false
				m.selectMode = true
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

	// 选择安装模式
	if m.selectMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.selectMode = false
			case "enter":
				i, ok := m.modeList.SelectedItem().(menuItem)
				if ok {
					switch i.title {
					case "1. 复制模式":
						m.installMode = installer.ModeCopy
						return m.doInstall()
					case "2. 链接模式":
						m.installMode = installer.ModeLink
						return m.doInstall()
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
		case tea.WindowSizeMsg:
			m.modeList.SetWidth(msg.Width)
			m.modeList.SetHeight(msg.Height - 2)
		}
		var cmd tea.Cmd
		m.modeList, cmd = m.modeList.Update(msg)
		return m, cmd
	}

	// 正常列表模式
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// 如果正在过滤中，不处理自定义快捷键
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "q":
			return newMainMenu(m.cfg), nil
		case "esc":
			// 如果在过滤应用状态，让 list 组件处理 Esc 以清除过滤
			if m.list.FilterState() == list.FilterApplied {
				break // 让下面的 m.list.Update(msg) 处理
			}
			return newMainMenu(m.cfg), nil
		case "?":
			// 使用 SelectedItem 获取实际选中的 item，避免过滤后索引错位
			if item, ok := m.list.SelectedItem().(skillItem); ok {
				m.showDetail = true
				m.detailSkill = &m.items[item.index].skill
			}
		case " ":
			// 使用 SelectedItem 获取实际选中的 item，避免过滤后索引错位
			if item, ok := m.list.SelectedItem().(skillItem); ok {
				m.items[item.index].selected = !m.items[item.index].selected
				// 使用 SetItem 更新单个 item，并返回 cmd 触发重新渲染
				cmd := m.list.SetItem(item.index, m.items[item.index])
				return m, cmd
			}
		case "enter":
			selected := m.getSelected()
			if len(selected) == 0 {
				m.message = "请至少选择一个 SKILL"
			} else {
				m.inputMode = true
				m.inputBuffer = "."
			}
		case "a":
			// 全选
			for i := range m.items {
				m.items[i].selected = true
			}
			m.updateListItems()
		case "n":
			// 全不选
			for i := range m.items {
				m.items[i].selected = false
			}
			m.updateListItems()
		case "tab":
			// 切换过滤模式
			filterTitleOnly = !filterTitleOnly
			if filterTitleOnly {
				m.message = "[过滤模式: 仅标题]"
			} else {
				m.message = "[过滤模式: 全部]"
			}
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
	}

	var cmd tea.Cmd
	prevState := m.prevFilterState
	m.list, cmd = m.list.Update(msg)

	// 检测过滤状态变化：从 FilterApplied 变为 Unfiltered 时，同步更新列表显示勾选状态
	currentState := m.list.FilterState()
	if prevState == list.FilterApplied && currentState == list.Unfiltered {
		m.updateListItems()
		m.message = "" // 清除过滤状态下的提示消息
	}
	m.prevFilterState = currentState

	return m, cmd
}

func (m *skillSelectorModel) updateListItems() {
	listItems := make([]list.Item, len(m.items))
	for i, item := range m.items {
		listItems[i] = item
	}
	m.list.SetItems(listItems)
}

func (m skillSelectorModel) getSelected() []string {
	var selected []string
	for _, item := range m.items {
		if item.selected {
			selected = append(selected, item.skill.ID())
		}
	}
	return selected
}

func (m skillSelectorModel) doInstall() (tea.Model, tea.Cmd) {
	selected := m.getSelected()
	inst := installer.NewInstaller(m.cfg.SkillsPath, m.targetPath, m.installMode)

	var result string
	if m.mode == "install" {
		installed, failed := inst.Install(selected)
		if len(installed) > 0 {
			result = fmt.Sprintf("成功安装: %s\n", strings.Join(installed, ", "))
		}
		for id, err := range failed {
			result += fmt.Sprintf("安装失败 [%s]: %v\n", id, err)
		}
	} else {
		removed, failed := inst.Uninstall(selected)
		if len(removed) > 0 {
			result = fmt.Sprintf("成功卸载: %s\n", strings.Join(removed, ", "))
		}
		for id, err := range failed {
			result += fmt.Sprintf("卸载失败 [%s]: %v\n", id, err)
		}
	}

	return newResultView(m.cfg, result), nil
}

func (m skillSelectorModel) View() string {
	if m.showDetail && m.detailSkill != nil {
		return m.viewDetail()
	}

	if m.inputMode {
		return m.viewInput()
	}

	if m.selectMode {
		return m.viewSelectMode()
	}

	view := m.list.View()
	help := helpStyle.Render("\n  ? 详情 | space 选中 | a 全选 | n 全不选 | / 过滤 | Tab 切换过滤 | enter 确认 | esc 返回")
	if m.message != "" {
		view += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.message)
	}
	return view + help
}

func (m skillSelectorModel) viewDetail() string {
	return RenderSkillDetail(*m.detailSkill, 85) + "\n\n" + helpStyle.Render("按 esc 或 ? 返回")
}

func (m skillSelectorModel) viewInput() string {
	return fmt.Sprintf(
		"%s\n\n输入目标项目路径: %s\n\n%s",
		titleStyle.Render("安装 SKILLS"),
		m.inputBuffer+"_",
		helpStyle.Render("enter 确认 | esc 取消"),
	)
}

func (m skillSelectorModel) viewSelectMode() string {
	return m.modeList.View()
}

// ==================== 安装模式选择器 ====================

type installModeSelectorModel struct {
	cfg  *config.Config
	list list.Model
}

func newInstallModeSelector(cfg *config.Config) installModeSelectorModel {
	items := []list.Item{
		menuItem{title: "1. 单个选择", desc: "从所有 SKILLS 中选择要安装的"},
		menuItem{title: "2. 按分组安装", desc: "选择分组，安装分组中的所有 SKILLS"},
	}

	l := list.New(items, list.NewDefaultDelegate(), 50, 15)
	l.Title = "安装 SKILLS"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return installModeSelectorModel{cfg: cfg, list: l}
}

func (m installModeSelectorModel) Init() tea.Cmd { return nil }

func (m installModeSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return newMainMenu(m.cfg), nil
		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if ok {
				switch i.title {
				case "1. 单个选择":
					return newSkillSelector(m.cfg, "install"), nil
				case "2. 按分组安装":
					if len(m.cfg.Groups) == 0 {
						return newResultView(m.cfg, "没有分组，请先创建分组"), nil
					}
					return newGroupSelectModel(m.cfg, "install"), nil
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

func (m installModeSelectorModel) View() string {
	return m.list.View()
}

// ==================== 卸载模式选择器 ====================

type uninstallModeSelectorModel struct {
	cfg  *config.Config
	list list.Model
}

func newUninstallModeSelector(cfg *config.Config) uninstallModeSelectorModel {
	items := []list.Item{
		menuItem{title: "1. 单个选择", desc: "从已安装的 SKILLS 中选择要卸载的"},
		menuItem{title: "2. 按分组卸载", desc: "选择分组，卸载分组中的 SKILLS"},
	}

	l := list.New(items, list.NewDefaultDelegate(), 50, 15)
	l.Title = "卸载 SKILLS"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return uninstallModeSelectorModel{cfg: cfg, list: l}
}

func (m uninstallModeSelectorModel) Init() tea.Cmd { return nil }

func (m uninstallModeSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			return newMainMenu(m.cfg), nil
		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if ok {
				switch i.title {
				case "1. 单个选择":
					return newUninstallPathInput(m.cfg), nil
				case "2. 按分组卸载":
					if len(m.cfg.Groups) == 0 {
						return newResultView(m.cfg, "没有分组，请先创建分组"), nil
					}
					return newGroupSelectModel(m.cfg, "uninstall"), nil
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

func (m uninstallModeSelectorModel) View() string {
	return m.list.View()
}
