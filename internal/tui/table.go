package tui

import (
	"path/filepath"
	"strings"

	"asoul/internal/i18n"
	"asoul/internal/model"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
)

var (
	tableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#B2BEC3"))

	tableSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4A4B6E"))

	sourceGitStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00CEC9"))

	sourceLocalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E056FD"))

	sourceManagedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF9F43"))
)

// padCell ensures content occupies exactly the given visual width in the terminal.
// It handles full-width East Asian characters (such as Chinese) as width 2,
// and ignores ANSI escape codes when calculating width.
func padCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w > width {
		s = truncate.StringWithTail(s, uint(width), "…")
		w = lipgloss.Width(s)
	}
	if w < width {
		s += strings.Repeat(" ", width-w)
	}
	return s
}

// computeSkillColWidths calculates column widths dynamically based on terminal width.
// Columns: nameW, statW, locHashW, sourceW, srcVerW (5 columns)
func computeSkillColWidths(termWidth int) (nameW, statW, locHashW, sourceW, srcVerW int) {
	if termWidth < 80 {
		termWidth = 80
	}

	statW = 10
	locHashW = 10

	// Fixed spacing: 2 chars cursor/indent + 4 chars checkbox + 4 separators (" ") = 10 fixed
	// statW(10) + locHashW(10) = 20
	// 10 + 20 = 30
	available := termWidth - 30
	if available < 50 {
		available = 50
	}

	// For 80 cols terminal:
	if termWidth <= 80 {
		nameW = 15
		sourceW = 18
		srcVerW = 17
		return
	}

	srcVerW = available * 30 / 100
	if srcVerW < 20 {
		srcVerW = 20
	} else if srcVerW > 30 {
		srcVerW = 30
	}

	rem := available - srcVerW
	nameW = rem * 38 / 100
	if nameW < 16 {
		nameW = 16
	} else if nameW > 30 {
		nameW = 30
	}

	sourceW = rem - nameW
	if sourceW < 18 {
		sourceW = 18
	}
	return
}

// computeTargetColWidths calculates target tab column widths.
func computeTargetColWidths(termWidth int) (nameW, pathW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	nameW = 24
	pathW = termWidth - 2 - 1 - nameW - 2
	if pathW < 30 {
		pathW = 30
	}
	return
}

// computeUpstreamsColWidths calculates upstream sources tab column widths.
func computeUpstreamsColWidths(termWidth int) (typeW, statusW, skillsW, refW, urlW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	typeW = 8
	statusW = 16
	refW = 12
	skillsW = 24
	fixed := 2 + typeW + 1 + statusW + 1 + skillsW + 1 + refW + 1
	urlW = termWidth - fixed - 4
	if urlW < 20 {
		urlW = 20
	}
	return
}

// computeCacheColWidths calculates cache tab column widths.
func computeCacheColWidths(termWidth int) (repoW, sizeW, urlW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	repoW = 26
	sizeW = 12
	urlW = termWidth - 2 - 2 - repoW - sizeW - 2
	if urlW < 24 {
		urlW = 24
	}
	return
}

// computeDoctorColWidths calculates doctor tab column widths.
func computeDoctorColWidths(termWidth int) (stW, itemW, msgW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	stW = 4
	itemW = 24
	msgW = termWidth - 2 - 2 - stW - itemW - 2
	if msgW < 30 {
		msgW = 30
	}
	return
}

// computeSettingsColWidths calculates settings tab column widths.
func computeSettingsColWidths(termWidth int) (itemW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	itemW = termWidth - 4 - 2
	if itemW < 30 {
		itemW = 30
	}
	return
}

// managedStatusLabel returns the localized label for a managed status.
func managedStatusLabel(status model.ManagedStatus) string {
	switch status {
	case model.StatusClean:
		return i18n.T("status.managed.clean")
	case model.StatusModified:
		return i18n.T("status.managed.modified")
	case model.StatusMissing:
		return i18n.T("status.managed.missing")
	case model.StatusInvalid:
		return i18n.T("status.managed.invalid")
	case model.StatusUnmanaged:
		return i18n.T("status.managed.unmanaged")
	default:
		return string(status)
	}
}

// upstreamStatusLabel returns the localized label for an upstream status.
func upstreamStatusLabel(status model.UpstreamStatus) string {
	switch status {
	case model.UpstreamUpToDate:
		return i18n.T("upstream.status.up_to_date")
	case model.UpstreamUpdateAvailable:
		return i18n.T("upstream.status.update_available")
	case model.UpstreamSourceChanged:
		return i18n.T("upstream.status.source_changed")
	case model.UpstreamUnreachable:
		return i18n.T("upstream.status.unreachable")
	case model.UpstreamInvalid:
		return i18n.T("upstream.status.invalid")
	default:
		return string(status)
	}
}

// sourceTypeLabel returns the localized label for a source type.
func sourceTypeLabel(src model.SourceType) string {
	switch src {
	case model.SourceTypeGit:
		return i18n.T("source.type.git")
	case model.SourceTypeLocal:
		return i18n.T("source.type.local")
	case model.SourceTypeManaged:
		return i18n.T("source.type.managed")
	default:
		return string(src)
	}
}

// formatStatusBadge returns a beautifully styled status pill with symbol.
func formatStatusBadge(status model.ManagedStatus, isSelected bool) string {
	text := managedStatusLabel(status)
	var sym string
	var style lipgloss.Style

	switch status {
	case model.StatusClean:
		sym = "● "
		if isSelected {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true)
		} else {
			style = statusCleanStyle
		}
	case model.StatusModified:
		sym = "▲ "
		if isSelected {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFEAA7")).Bold(true)
		} else {
			style = statusModifiedStyle
		}
	case model.StatusInvalid, model.StatusMissing:
		sym = "✖ "
		if isSelected {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Bold(true)
		} else {
			style = statusErrorStyle
		}
	case model.StatusUnmanaged:
		sym = "○ "
		if isSelected {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")).Bold(true)
		} else {
			style = statusNoticeStyle
		}
	default:
		return text
	}

	return style.Render(sym + text)
}

// formatSourceType returns a styled source pill.
func formatSourceType(src model.SourceType, isSelected bool) string {
	str := sourceTypeLabel(src)
	if isSelected {
		return str
	}
	switch src {
	case model.SourceTypeGit:
		return sourceGitStyle.Render(str)
	case model.SourceTypeLocal:
		return sourceLocalStyle.Render(str)
	case model.SourceTypeManaged:
		return sourceManagedStyle.Render(str)
	default:
		return str
	}
}

// formatUpstreamStatus styles upstream status (alias formatSourceStatus).
func formatUpstreamStatus(upstream model.UpstreamStatus, isSelected bool) string {
	str := upstreamStatusLabel(upstream)
	if isSelected {
		return str
	}
	switch upstream {
	case model.UpstreamUpdateAvailable, model.UpstreamSourceChanged:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true).Render("↑ " + str)
	case model.UpstreamUpToDate:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render(str)
	default:
		return str
	}
}

// formatSourceStatus is an alias for formatUpstreamStatus.
func formatSourceStatus(status model.UpstreamStatus, isSelected bool) string {
	return formatUpstreamStatus(status, isSelected)
}

// formatSourceName extracts a user-friendly source name (e.g. repo slug or directory name).
func formatSourceName(spec model.SourceSpec) string {
	switch spec.Type {
	case model.SourceTypeGit:
		if spec.URL == "" {
			return "-"
		}
		u := strings.TrimSpace(spec.URL)
		u = strings.TrimSuffix(u, ".git")
		u = strings.TrimSuffix(u, "/")
		if idx := strings.LastIndex(u, ":"); idx != -1 {
			afterColon := u[idx+1:]
			if strings.Contains(afterColon, "/") {
				parts := strings.Split(afterColon, "/")
				if len(parts) >= 2 {
					return parts[len(parts)-2] + "/" + parts[len(parts)-1]
				}
				return afterColon
			}
		}
		if idx := strings.Index(u, "://"); idx != -1 {
			u = u[idx+3:]
			if slashIdx := strings.Index(u, "/"); slashIdx != -1 {
				u = u[slashIdx+1:]
			}
		}
		parts := strings.Split(u, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2] + "/" + parts[len(parts)-1]
		} else if len(parts) == 1 && parts[0] != "" {
			return parts[0]
		}
		return u
	case model.SourceTypeLocal:
		if spec.Path == "" {
			return "-"
		}
		return filepath.Base(spec.Path)
	case model.SourceTypeManaged:
		return "-"
	default:
		return "-"
	}
}

// formatLocalHash formats the local skill directory content hash.
func formatLocalHash(s model.SkillStatus) string {
	h := s.CurrentHash
	if h == "" {
		if s.ManagedStatus == model.StatusMissing {
			return "-"
		}
		h = s.RecordedHash
	}
	if h != "" {
		if len(h) > 10 {
			return h[:10]
		}
		return h
	}
	return "-"
}

// formatSourceHash formats the upstream/source commit or hash.
func formatSourceHash(s model.SkillStatus) string {
	switch s.Source.Type {
	case model.SourceTypeGit:
		commit := s.NewCommit
		if commit == "" {
			commit = s.RecordedCommit
		}
		if commit != "" {
			if len(commit) > 7 {
				return commit[:7]
			}
			return commit
		}
		return "-"
	case model.SourceTypeLocal:
		h := s.NewHash
		if h == "" {
			h = s.RecordedHash
		}
		if h != "" {
			if len(h) > 10 {
				return h[:10]
			}
			return h
		}
		return "-"
	default:
		return "-"
	}
}

// formatSourceCombined returns a combined source badge and name, e.g. "(git) openai/skills".
func formatSourceCombined(spec model.SourceSpec, isSelected bool) string {
	name := formatSourceName(spec)
	switch spec.Type {
	case model.SourceTypeGit:
		tag := "(git)"
		if !isSelected {
			tag = sourceGitStyle.Render("(git)")
		}
		if name == "-" {
			return tag
		}
		return tag + " " + name
	case model.SourceTypeLocal:
		tag := "(local)"
		if !isSelected {
			tag = sourceLocalStyle.Render("(local)")
		}
		if name == "-" {
			return tag
		}
		return tag + " " + name
	case model.SourceTypeManaged:
		tag := "(managed)"
		if !isSelected {
			tag = sourceManagedStyle.Render("(managed)")
		}
		if name == "-" {
			return tag
		}
		return tag + " " + name
	default:
		return name
	}
}

// formatSourceVersion formats the source hash first followed by status in parentheses, e.g. "34040c9 (up-to-date)".
func formatSourceVersion(s model.SkillStatus, isSelected bool) string {
	if s.Source.Type == model.SourceTypeManaged {
		return "-"
	}

	// 1. Determine Hash
	h := "-"
	switch s.Source.Type {
	case model.SourceTypeGit:
		commit := s.NewCommit
		if commit == "" {
			commit = s.RecordedCommit
		}
		if commit != "" {
			if len(commit) > 7 {
				commit = commit[:7]
			}
			h = commit
		}
	case model.SourceTypeLocal:
		sh := s.NewHash
		if sh == "" {
			sh = s.RecordedHash
		}
		if sh != "" {
			if len(sh) > 8 {
				sh = sh[:8]
			}
			h = sh
		}
	}

	// 2. Determine Status inside parentheses
	rawStatus := string(s.Upstream)
	if rawStatus == "" {
		rawStatus = "-"
	}

	var statusTag string
	if isSelected {
		switch s.Upstream {
		case model.UpstreamUpdateAvailable, model.UpstreamSourceChanged:
			statusTag = "(↑ " + rawStatus + ")"
		default:
			statusTag = "(" + rawStatus + ")"
		}
	} else {
		switch s.Upstream {
		case model.UpstreamUpToDate:
			statusTag = lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render("(" + rawStatus + ")")
		case model.UpstreamUpdateAvailable, model.UpstreamSourceChanged:
			statusTag = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDCB6E")).Bold(true).Render("(↑ " + rawStatus + ")")
		case model.UpstreamUnreachable, model.UpstreamInvalid:
			statusTag = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Render("(" + rawStatus + ")")
		default:
			statusTag = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Render("(-)")
		}
	}

	return h + " " + statusTag
}

// formatTargetEnabledBadge returns a styled enabled/disabled pill.
func formatTargetEnabledBadge(enabled bool, isSelected bool) string {
	if enabled {
		if isSelected {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true).Render("● " + i18n.T("target.status.enabled"))
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#00B894")).Render("● " + i18n.T("target.status.enabled"))
	}
	if isSelected {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#B2BEC3")).Bold(true).Render("○ " + i18n.T("target.status.disabled"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Render("○ " + i18n.T("target.status.disabled"))
}

// computeAgentColWidths calculates column widths for Agent channel table:
// statusW, nameW, configW, featW, skillsW
func computeAgentColWidths(termWidth int) (statusW, nameW, configW, featW, skillsW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	statusW = 10
	nameW = 16
	featW = 14
	skillsW = 8
	fixed := 2 + 4 + statusW + nameW + featW + skillsW
	configW = termWidth - fixed - 2
	if configW < 20 {
		configW = 20
	}
	return
}

// computeProjectColWidths calculates column widths for Project target table:
// statusW, nameW, pathW, featW, skillsW
func computeProjectColWidths(termWidth int) (statusW, nameW, pathW, featW, skillsW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	statusW = 10
	nameW = 16
	featW = 14
	skillsW = 8
	fixed := 2 + 4 + statusW + nameW + featW + skillsW
	pathW = termWidth - fixed - 2
	if pathW < 20 {
		pathW = 20
	}
	return
}

// computeGroupColWidths calculates column widths for Group management table.
func computeGroupColWidths(termWidth int) (nameW, countW, skillsW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	nameW = 20
	countW = 10
	fixed := 2 + 3 + nameW + countW
	skillsW = termWidth - fixed - 2
	if skillsW < 24 {
		skillsW = 24
	}
	return
}

// computeModelColWidths calculates column widths for Model configuration table grouped by provider.
func computeModelColWidths(termWidth int) (modelW, ctxW, priceW, capsW, varW, statusW int) {
	if termWidth < 80 {
		termWidth = 80
	}
	if termWidth < 110 {
		ctxW = 9
		priceW = 11
		capsW = 10
		varW = 6
		statusW = 18
		fixed := 4 + 1 + ctxW + 1 + priceW + 1 + capsW + 1 + varW + 1 + statusW + 2
		modelW = termWidth - fixed
		if modelW < 15 {
			modelW = 15
		}
		return
	}
	ctxW = 12
	priceW = 16
	capsW = 19
	varW = 8
	statusW = 20
	fixed := 4 + 1 + ctxW + 1 + priceW + 1 + capsW + 1 + varW + 1 + statusW + 2
	modelW = termWidth - fixed
	if modelW < 24 {
		modelW = 24
	}
	return
}
