package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/nyable/asoul/internal/config"
)

// Interactive 交互式界面
type Interactive struct {
	cfg *config.Config
}

// NewInteractive 创建交互式界面
func NewInteractive(cfg *config.Config) *Interactive {
	return &Interactive{cfg: cfg}
}

// Run 运行交互式界面
func (i *Interactive) Run() error {
	p := tea.NewProgram(newMainMenu(i.cfg), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
