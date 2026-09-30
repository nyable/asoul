package tui

import (
	"strings"

	"asoul/internal/i18n"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// All navigation indexes are computed from the same rendered content as the
// viewport, so line numbers/summary/changes-only cannot shift jump destinations.
func (m *Model) handleDiffNavigation(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.diffSearching {
		switch msg.String() {
		case "esc":
			m.diffSearching = false
			m.diffSearch.Blur()
		case "enter":
			m.diffSearching = false
			m.diffSearch.Blur()
			m.jumpDiffMatch(1, true)
		default:
			var cmd tea.Cmd
			m.diffSearch, cmd = m.diffSearch.Update(msg)
			return true, cmd
		}
		return true, nil
	}
	key := msg.String()
	if m.diffPrefix != "" {
		prefix := m.diffPrefix
		m.diffPrefix = ""
		if prefix == "g" && key == "g" {
			m.diffViewport.GotoTop()
			return true, nil
		}
		if (prefix == "]" || prefix == "[") && key == "c" {
			direction := 1
			if prefix == "[" {
				direction = -1
			}
			m.jumpDiffHunk(direction)
			return true, nil
		}
	}
	switch key {
	case "/":
		m.diffSearch = textinput.New()
		m.diffSearch.Prompt = "/"
		m.diffSearch.Width = max(10, m.width-10)
		m.diffSearching = true
		return true, m.diffSearch.Focus()
	case "n", "N":
		// In confirm mode n without a search remains the cancel key.
		if m.diffSearch.Value() == "" {
			return false, nil
		}
		direction := 1
		if key == "N" {
			direction = -1
		}
		m.jumpDiffMatch(direction, false)
		return true, nil
	case "[", "]":
		m.diffPrefix = key
		return true, nil
	case "g":
		m.diffPrefix = key
		m.diffLastJump = -1
		m.diffViewport.GotoTop()
		return true, nil
	case "up", "down", "k", "j", "pgup", "pgdown", "ctrl+u", "ctrl+d", "G", "home", "end":
		m.diffLastJump = -1
	case "left", "h":
		m.diffViewport.ScrollLeft(8)
		return true, nil
	case "right", "l":
		m.diffViewport.ScrollRight(8)
		return true, nil
	}
	return false, nil
}

func (m *Model) jumpDiffHunk(direction int) {
	var positions []int
	var files []int
	for i, line := range strings.Split(m.diffRendered, "\n") {
		plain := ansi.Strip(line)
		if strings.Contains(plain, "@@ -") {
			positions = append(positions, i)
		}
		if strings.Contains(plain, "diff --git ") {
			files = append(files, i)
		}
	}
	if len(positions) == 0 {
		positions = files
	}
	m.jumpDiffPositions(positions, direction, false)
}

func (m *Model) jumpDiffMatch(direction int, includeCurrent bool) {
	query := strings.ToLower(m.diffSearch.Value())
	if query == "" {
		return
	}
	var positions []int
	for i, line := range strings.Split(m.diffRendered, "\n") {
		if strings.Contains(strings.ToLower(ansi.Strip(line)), query) {
			positions = append(positions, i)
		}
	}
	if len(positions) == 0 {
		m.notice = i18n.T("notice.diff_search_empty", m.diffSearch.Value())
		return
	}
	m.jumpDiffPositions(positions, direction, includeCurrent)
}

func (m *Model) jumpDiffPositions(positions []int, direction int, includeCurrent bool) {
	if len(positions) == 0 {
		return
	}
	current := m.diffViewport.YOffset
	if m.diffLastJump >= current && m.diffLastJump-current < m.diffViewport.Height {
		current = m.diffLastJump
	}
	pos := positions[0]
	if direction > 0 {
		for _, p := range positions {
			if p > current || (includeCurrent && p == current) {
				pos = p
				break
			}
		}
	} else {
		pos = positions[len(positions)-1]
		for i := len(positions) - 1; i >= 0; i-- {
			if positions[i] < current {
				pos = positions[i]
				break
			}
		}
	}
	m.diffLastJump = pos
	m.diffViewport.SetYOffset(pos)
}
