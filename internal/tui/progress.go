package tui

import (
	"strings"

	"asoul/internal/i18n"
	"asoul/internal/progress"

	"github.com/charmbracelet/lipgloss"
)

var (
	progressLabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0984E3"))
	progressBarStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894"))
	progressDetailStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894"))
)

var progressSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func progressSpinner(frame int) string {
	if len(progressSpinnerFrames) == 0 {
		return "⟳"
	}
	if frame < 0 {
		frame = -frame
	}
	return progressSpinnerFrames[frame%len(progressSpinnerFrames)]
}

// progressBarWidth derives a readable bar width from the available terminal
// width while staying within a stable range across modal sizes.
func progressBarWidth(maxWidth int) int {
	w := maxWidth - 8
	if w > 40 {
		w = 40
	}
	if w < 12 {
		w = 12
	}
	return w
}

// renderProgressBar renders either a determinate bar with a percentage and
// counts ("████░░░░  50% (100/200)") or, when no reliable total is known, an
// animated segment sweeping across the track.
func renderProgressBar(current, total, width, frame int) string {
	if width < 8 {
		width = 8
	}
	const fill = "█"
	const empty = "░"

	if total <= 0 {
		seg := width / 3
		if seg < 3 {
			seg = 3
		}
		if seg > width {
			seg = width
		}
		period := width + seg
		pos := 0
		if period > 0 {
			pos = frame % period
		}
		var b strings.Builder
		for i := 0; i < width; i++ {
			if i >= pos-seg && i < pos {
				b.WriteString(fill)
			} else {
				b.WriteString(empty)
			}
		}
		return progressBarStyle.Render(b.String())
	}

	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	filled := current * width / total
	pct := current * 100 / total

	var b strings.Builder
	for i := 0; i < width; i++ {
		if i < filled {
			b.WriteString(fill)
		} else {
			b.WriteString(empty)
		}
	}
	label := i18n.T("progress.percent", pct, current, total)
	return progressBarStyle.Render(b.String()) + " " + progressDetailStyle.Render(label)
}

func progressPhaseLabel(p progress.Phase) string {
	if key := p.I18nKey(); key != "" {
		return i18n.T(key)
	}
	return ""
}

// progressDetailText prefers a concrete detail (such as a skill ID) and falls
// back to a localized phase label. Git tool phases drop their raw English line
// so the detail stays localized and the counts are not duplicated by the bar.
func progressDetailText(phase progress.Phase, raw string) string {
	switch phase {
	case progress.PhaseUnknown,
		progress.PhaseDeploy, progress.PhaseCheck, progress.PhaseRemove,
		progress.PhaseExtract, progress.PhaseValidate, progress.PhaseInstall, progress.PhaseScan:
		if raw != "" {
			return raw
		}
	}
	if label := progressPhaseLabel(phase); label != "" {
		return label
	}
	return raw
}

// sendProgress posts a structured progress update from a background task.
func (m *Model) sendProgress(phase progress.Phase, current, total int, detail string) {
	if m.program == nil {
		return
	}
	m.program.Send(progressMsg{phase: phase, current: current, total: total, detail: detail})
}

// progressCallback adapts a service progress callback into a Bubble Tea
// message so reports produced on the background goroutine are applied on the
// main event loop.
func (m *Model) progressCallback() progress.Func {
	return func(u progress.Update) {
		if m.program == nil {
			return
		}
		m.program.Send(progressMsg{phase: u.Phase, current: u.Current, total: u.Total, detail: u.Text})
	}
}

// progressBlock renders the shared loading indicator (animated spinner, task
// label, progress bar and optional detail line) used by every loading surface.
func (m *Model) progressBlock() string {
	label := m.notice
	if label == "" {
		label = i18n.T("modal.loading.default_notice")
	}
	var b strings.Builder
	b.WriteString(progressLabelStyle.Render(progressSpinner(m.progressFrame) + " " + label))
	b.WriteString("\n")
	b.WriteString(renderProgressBar(m.progressCurrent, m.progressTotal, progressBarWidth(m.width), m.progressFrame))
	if detail := progressDetailText(m.progressPhase, m.gitProgress); detail != "" {
		b.WriteString("\n")
		b.WriteString(progressDetailStyle.Render("➜ " + detail))
	}
	return b.String()
}
