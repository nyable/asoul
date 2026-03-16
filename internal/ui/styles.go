package ui

import "github.com/charmbracelet/lipgloss"

// 简化样式定义，使用基础样式避免显示问题
var (
	titleStyle        = lipgloss.NewStyle().Bold(true)
	helpStyle         = lipgloss.NewStyle().Faint(true)
	detailBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)
