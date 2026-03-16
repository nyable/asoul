package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nyable/asoul/internal/config"
)

// ==================== 结果显示 ====================

type resultViewModel struct {
	cfg     *config.Config
	message string
}

func newResultView(cfg *config.Config, message string) resultViewModel {
	return resultViewModel{cfg: cfg, message: message}
}

func (m resultViewModel) Init() tea.Cmd { return nil }

func (m resultViewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		return newMainMenu(m.cfg), nil
	}
	return m, nil
}

func (m resultViewModel) View() string {
	// 使用边框样式让结果更突出
	resultStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(70)

	content := titleStyle.Render("操作结果") + "\n\n" + m.message
	return resultStyle.Render(content) + "\n\n" + helpStyle.Render("按任意键返回主菜单...")
}
