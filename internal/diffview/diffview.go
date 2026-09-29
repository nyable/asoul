package diffview

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"asoul/internal/i18n"

	"github.com/charmbracelet/lipgloss"
)

// FileStat contains change statistics for a single file in the diff.
type FileStat struct {
	Path      string
	Additions int
	Deletions int
	IsBinary  bool
}

// Summary contains aggregate statistics for a diff.
type Summary struct {
	Files      []FileStat
	TotalFiles int
	Additions  int
	Deletions  int
}

// RenderOptions controls how the diff is rendered.
type RenderOptions struct {
	Color          bool
	IncludeSummary bool
	ChangesOnly    bool // Hide context lines and keep only changed lines
	LineNumbers    bool // Show old/new line number gutter
	Width          int
}

// diffLineKind classifies a rendered diff line.
type diffLineKind int

const (
	diffLineMeta diffLineKind = iota
	diffLineFileHeader
	diffLineHunk
	diffLineContext
	diffLineAdd
	diffLineDel
	diffLineSeparator
)

// diffLine is a parsed diff line together with its old/new line numbers.
type diffLine struct {
	kind  diffLineKind
	text  string
	oldNo int
	newNo int
}

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// parseLines splits a raw diff into numbered lines, supporting both git
// unified diffs (with @@ hunk headers) and the simple "--- original/+++
// updated" format produced for config edits.
func parseLines(rawDiff string) []diffLine {
	lines := strings.Split(rawDiff, "\n")

	gitFormat := false
	for _, l := range lines {
		t := strings.TrimRight(l, "\r")
		if strings.HasPrefix(t, "diff --git ") || strings.HasPrefix(t, "@@") {
			gitFormat = true
			break
		}
	}

	var out []diffLine
	oldNo, newNo := 0, 0
	body := false

	for _, l := range lines {
		t := strings.TrimRight(l, "\r")
		switch {
		case strings.HasPrefix(t, "diff --git "):
			out = append(out, diffLine{kind: diffLineMeta, text: t})
			body = false
		case strings.HasPrefix(t, "@@"):
			oldStart, newStart := parseHunkHeader(t)
			if oldStart > 0 {
				oldNo = oldStart
			}
			if newStart > 0 {
				newNo = newStart
			}
			body = true
			out = append(out, diffLine{kind: diffLineHunk, text: t})
		case strings.HasPrefix(t, "--- "):
			out = append(out, diffLine{kind: diffLineFileHeader, text: t})
		case strings.HasPrefix(t, "+++ "):
			out = append(out, diffLine{kind: diffLineFileHeader, text: t})
			if !gitFormat {
				oldNo, newNo = 1, 1
				body = true
			}
		case strings.HasPrefix(t, "+"):
			out = append(out, diffLine{kind: diffLineAdd, text: t, newNo: newNo})
			newNo++
		case strings.HasPrefix(t, "-"):
			out = append(out, diffLine{kind: diffLineDel, text: t, oldNo: oldNo})
			oldNo++
		case strings.HasPrefix(t, "\\ No newline"):
			out = append(out, diffLine{kind: diffLineMeta, text: t})
		case strings.HasPrefix(t, "Binary files "):
			out = append(out, diffLine{kind: diffLineMeta, text: t})
		case body:
			out = append(out, diffLine{kind: diffLineContext, text: t, oldNo: oldNo, newNo: newNo})
			if oldNo > 0 {
				oldNo++
			}
			if newNo > 0 {
				newNo++
			}
		default:
			out = append(out, diffLine{kind: diffLineMeta, text: t})
		}
	}
	return out
}

func parseHunkHeader(line string) (oldStart, newStart int) {
	m := hunkHeaderPattern.FindStringSubmatch(line)
	if m == nil {
		return 0, 0
	}
	oldStart, _ = strconv.Atoi(m[1])
	newStart, _ = strconv.Atoi(m[2])
	return oldStart, newStart
}

// changeLines keeps only headers and changed lines, inserting a separator
// whenever a run of context lines was skipped so fragments stay distinct.
func changeLines(lines []diffLine) []diffLine {
	var out []diffLine
	skipped := false
	for _, l := range lines {
		switch l.kind {
		case diffLineContext:
			skipped = true
		case diffLineAdd, diffLineDel:
			if skipped && len(out) > 0 {
				out = append(out, diffLine{kind: diffLineSeparator, text: "⋯"})
			}
			skipped = false
			out = append(out, l)
		case diffLineHunk, diffLineFileHeader:
			skipped = false
			out = append(out, l)
		case diffLineMeta:
			if strings.HasPrefix(l.text, "Binary files ") || strings.HasPrefix(l.text, "diff --git ") {
				skipped = false
				out = append(out, l)
			}
		}
	}
	return out
}

// cleanGitPath cleans a path token by stripping quotes and git a/ or b/ prefixes.
func cleanGitPath(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") && len(p) >= 2 {
		p = p[1 : len(p)-1]
	}
	p = strings.TrimPrefix(p, "a/")
	p = strings.TrimPrefix(p, "b/")
	return p
}

// parseGitDiffHeader extracts the two paths from a 'diff --git <pathA> <pathB>' line.
func parseGitDiffHeader(line string) (string, string) {
	rem := strings.TrimPrefix(line, "diff --git ")
	rem = strings.TrimSpace(rem)
	if strings.HasPrefix(rem, "\"") {
		idx := strings.Index(rem[1:], "\"")
		if idx != -1 {
			pathA := rem[1 : idx+1]
			remB := strings.TrimSpace(rem[idx+2:])
			if strings.HasPrefix(remB, "\"") && strings.HasSuffix(remB, "\"") && len(remB) >= 2 {
				return pathA, remB[1 : len(remB)-1]
			}
			return pathA, remB
		}
	}
	parts := strings.Split(rem, " ")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return rem, rem
}

// extractCleanPath computes a readable relative path from old and new path candidates.
func extractCleanPath(pathA, pathB string) string {
	pathA = cleanGitPath(pathA)
	pathB = cleanGitPath(pathB)
	if pathA == "/dev/null" || pathA == "dev/null" {
		return pathB
	}
	if pathB == "/dev/null" || pathB == "dev/null" {
		return pathA
	}
	if (pathA == "original" && pathB == "updated") || (pathA == "old" && pathB == "new") {
		return "(config)"
	}
	if pathA == pathB {
		return pathA
	}

	partsA := strings.Split(filepath.ToSlash(pathA), "/")
	partsB := strings.Split(filepath.ToSlash(pathB), "/")
	var common []string
	iA, iB := len(partsA)-1, len(partsB)-1
	for iA >= 0 && iB >= 0 && partsA[iA] == partsB[iB] {
		common = append([]string{partsA[iA]}, common...)
		iA--
		iB--
	}
	if len(common) > 0 {
		return strings.Join(common, "/")
	}
	if pathA != "" && pathB != "" && pathA != pathB {
		return filepath.Base(pathA) + " -> " + filepath.Base(pathB)
	}
	if pathB != "" {
		return filepath.Base(pathB)
	}
	return filepath.Base(pathA)
}

// Parse extracts file-level and aggregate statistics from a raw diff string.
func Parse(rawDiff string) Summary {
	var summary Summary
	if strings.TrimSpace(rawDiff) == "" {
		return summary
	}

	lines := strings.Split(rawDiff, "\n")
	currIdx := -1
	var pendingOld, pendingNew string

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		if strings.HasPrefix(trimmed, "diff --git ") {
			pathA, pathB := parseGitDiffHeader(trimmed)
			cleanPath := extractCleanPath(pathA, pathB)
			summary.Files = append(summary.Files, FileStat{Path: cleanPath})
			currIdx = len(summary.Files) - 1
			pendingOld = ""
			pendingNew = ""
			continue
		}

		if strings.HasPrefix(trimmed, "--- ") {
			p := strings.TrimPrefix(trimmed, "--- ")
			if idx := strings.IndexAny(p, "\t\r\n"); idx != -1 {
				p = p[:idx]
			}
			pendingOld = strings.TrimSpace(p)
			continue
		}

		if strings.HasPrefix(trimmed, "+++ ") {
			p := strings.TrimPrefix(trimmed, "+++ ")
			if idx := strings.IndexAny(p, "\t\r\n"); idx != -1 {
				p = p[:idx]
			}
			pendingNew = strings.TrimSpace(p)

			if currIdx == -1 || summary.Files[currIdx].Path == "" {
				cleanPath := extractCleanPath(pendingOld, pendingNew)
				summary.Files = append(summary.Files, FileStat{Path: cleanPath})
				currIdx = len(summary.Files) - 1
			}
			continue
		}

		if strings.HasPrefix(trimmed, "Binary files ") {
			if currIdx >= 0 && currIdx < len(summary.Files) {
				summary.Files[currIdx].IsBinary = true
			}
			continue
		}

		if strings.HasPrefix(trimmed, "+++") || strings.HasPrefix(trimmed, "---") {
			continue
		}

		if strings.HasPrefix(trimmed, "+") {
			summary.Additions++
			if currIdx >= 0 && currIdx < len(summary.Files) {
				summary.Files[currIdx].Additions++
			} else {
				summary.Files = append(summary.Files, FileStat{Path: "(content)", Additions: 1})
				currIdx = len(summary.Files) - 1
			}
			continue
		}

		if strings.HasPrefix(trimmed, "-") {
			summary.Deletions++
			if currIdx >= 0 && currIdx < len(summary.Files) {
				summary.Files[currIdx].Deletions++
			} else {
				summary.Files = append(summary.Files, FileStat{Path: "(content)", Deletions: 1})
				currIdx = len(summary.Files) - 1
			}
			continue
		}
	}

	summary.TotalFiles = len(summary.Files)
	if summary.TotalFiles == 0 && (summary.Additions > 0 || summary.Deletions > 0) {
		summary.Files = append(summary.Files, FileStat{
			Path:      "(content)",
			Additions: summary.Additions,
			Deletions: summary.Deletions,
		})
		summary.TotalFiles = 1
	}

	return summary
}

// FormatBadge returns a compact badge suitable for title bars or dividers, e.g. '[ 2 files | +18 -4 ]'.
func FormatBadge(s Summary, color bool) string {
	if s.TotalFiles == 0 && s.Additions == 0 && s.Deletions == 0 {
		label := i18n.T("diff.badge_no_changes")
		if !color {
			return fmt.Sprintf("[ %s ]", label)
		}
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#747D8C"))
		return dimStyle.Render(fmt.Sprintf("[ %s ]", label))
	}

	var filesStr string
	if s.TotalFiles == 1 {
		filesStr = i18n.T("diff.badge_file_one")
	} else {
		filesStr = fmt.Sprintf(i18n.T("diff.badge_files_multi"), s.TotalFiles)
	}

	if !color {
		return fmt.Sprintf("[ %s | +%d -%d ]", filesStr, s.Additions, s.Deletions)
	}

	bracketStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#747D8C"))
	filesStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9")).Bold(true)
	pipeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72"))
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#2ED573")).Bold(true)
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4757")).Bold(true)

	return bracketStyle.Render("[ ") +
		filesStyle.Render(filesStr) + " " +
		pipeStyle.Render("|") + " " +
		addStyle.Render(fmt.Sprintf("+%d", s.Additions)) + " " +
		delStyle.Render(fmt.Sprintf("-%d", s.Deletions)) +
		bracketStyle.Render(" ]")
}

// FormatSummary formats a top-level summary banner containing total files changed, additions/deletions, and per-file stats.
func FormatSummary(s Summary, color bool, width int) string {
	if width <= 0 {
		width = 80
	}
	boxWidth := width - 4
	if boxWidth < 38 {
		boxWidth = 38
	}
	if boxWidth > 90 {
		boxWidth = 90
	}

	var content strings.Builder

	title := "📊 " + i18n.T("diff.summary_title")
	if color {
		title = lipgloss.NewStyle().Foreground(lipgloss.Color("#70A1FF")).Bold(true).Render(title)
	}
	content.WriteString(title)
	content.WriteString("\n")

	if s.TotalFiles == 0 && s.Additions == 0 && s.Deletions == 0 {
		noDiff := i18n.T("diff.no_differences")
		if color {
			noDiff = lipgloss.NewStyle().Foreground(lipgloss.Color("#747D8C")).Render(noDiff)
		}
		content.WriteString(noDiff)
	} else {
		var filesPart string
		if s.TotalFiles == 1 {
			filesPart = i18n.T("diff.file_changed_one")
		} else {
			filesPart = fmt.Sprintf(i18n.T("diff.files_changed"), s.TotalFiles)
		}

		addPart := fmt.Sprintf(i18n.T("diff.additions"), s.Additions)
		delPart := fmt.Sprintf(i18n.T("diff.deletions"), s.Deletions)

		if color {
			filesPart = lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9")).Bold(true).Render(filesPart)
			dot := lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Render(" • ")
			addPart = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ED573")).Bold(true).Render(addPart)
			delPart = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4757")).Bold(true).Render(delPart)
			content.WriteString(filesPart + dot + addPart + dot + delPart)
		} else {
			content.WriteString(filesPart + " • " + addPart + " • " + delPart)
		}

		maxDisplay := 6
		displayed := 0
		maxPathLen := boxWidth - 22
		if maxPathLen < 15 {
			maxPathLen = 15
		}

		for _, f := range s.Files {
			if displayed >= maxDisplay {
				remaining := len(s.Files) - displayed
				moreStr := fmt.Sprintf(i18n.T("diff.and_more_files"), remaining)
				if color {
					moreStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#747D8C")).Faint(true).Render("  " + moreStr)
				} else {
					moreStr = "  " + moreStr
				}
				content.WriteString("\n" + moreStr)
				break
			}

			displayed++
			content.WriteString("\n")

			p := f.Path
			if len(p) > maxPathLen {
				p = "..." + p[len(p)-(maxPathLen-3):]
			}

			if f.IsBinary {
				binStr := i18n.T("diff.binary_file")
				if color {
					content.WriteString(fmt.Sprintf("  • %-*s %s", maxPathLen, p,
						lipgloss.NewStyle().Foreground(lipgloss.Color("#747D8C")).Render(binStr)))
				} else {
					content.WriteString(fmt.Sprintf("  • %-*s %s", maxPathLen, p, binStr))
				}
				continue
			}

			if color {
				pathStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9")).Render(fmt.Sprintf("%-*s", maxPathLen, p))
				addStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#2ED573")).Render(fmt.Sprintf("+%-4d", f.Additions))
				delStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4757")).Render(fmt.Sprintf("-%d", f.Deletions))
				content.WriteString("  • " + pathStyled + " " + addStyled + " " + delStyled)
			} else {
				content.WriteString(fmt.Sprintf("  • %-*s +%-4d -%d", maxPathLen, p, f.Additions, f.Deletions))
			}
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(boxWidth)

	if color {
		boxStyle = boxStyle.BorderForeground(lipgloss.Color("#6C5CE7"))
	}

	return boxStyle.Render(content.String())
}

// ColorizeLines styles each line of the diff according to its diff semantics.
func ColorizeLines(rawDiff string, color bool) string {
	if !color {
		return rawDiff
	}

	styleAdded := lipgloss.NewStyle().Foreground(lipgloss.Color("#2ED573"))
	styleDeleted := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4757"))
	styleHunk := lipgloss.NewStyle().Foreground(lipgloss.Color("#00CEC9")).Bold(true)
	styleHunkCtx := lipgloss.NewStyle().Foreground(lipgloss.Color("#81ECEC")).Faint(true)
	styleDiffHeader := lipgloss.NewStyle().Foreground(lipgloss.Color("#70A1FF")).Bold(true)
	styleFileOld := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Bold(true)
	styleFileNew := lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true)
	styleMeta := lipgloss.NewStyle().Foreground(lipgloss.Color("#747D8C"))
	styleContext := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))

	lines := strings.Split(rawDiff, "\n")
	var sb strings.Builder

	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}

		trimmed := strings.TrimRight(line, "\r")

		if strings.HasPrefix(trimmed, "diff --git ") {
			sb.WriteString(styleDiffHeader.Render(trimmed))
			continue
		}

		if strings.HasPrefix(trimmed, "--- ") || trimmed == "---" {
			sb.WriteString(styleFileOld.Render(trimmed))
			continue
		}

		if strings.HasPrefix(trimmed, "+++ ") || trimmed == "+++" {
			sb.WriteString(styleFileNew.Render(trimmed))
			continue
		}

		if strings.HasPrefix(trimmed, "@@") {
			idx := strings.Index(trimmed[2:], "@@")
			if idx != -1 {
				hunkRange := trimmed[:2+idx+2]
				trailing := trimmed[2+idx+2:]
				styled := styleHunk.Render(hunkRange)
				if strings.TrimSpace(trailing) != "" {
					styled += " " + styleHunkCtx.Render(strings.TrimSpace(trailing))
				}
				sb.WriteString(styled)
			} else {
				sb.WriteString(styleHunk.Render(trimmed))
			}
			continue
		}

		if strings.HasPrefix(trimmed, "+") {
			sb.WriteString(styleAdded.Render(trimmed))
			continue
		}

		if strings.HasPrefix(trimmed, "-") {
			sb.WriteString(styleDeleted.Render(trimmed))
			continue
		}

		if strings.HasPrefix(trimmed, "index ") ||
			strings.HasPrefix(trimmed, "new file mode ") ||
			strings.HasPrefix(trimmed, "deleted file mode ") ||
			strings.HasPrefix(trimmed, "similarity index ") ||
			strings.HasPrefix(trimmed, "rename from ") ||
			strings.HasPrefix(trimmed, "rename to ") ||
			strings.HasPrefix(trimmed, "Binary files ") ||
			strings.HasPrefix(trimmed, "\\ No newline at end of file") {
			sb.WriteString(styleMeta.Render(trimmed))
			continue
		}

		sb.WriteString(styleContext.Render(trimmed))
	}

	return sb.String()
}

// FilterChanges keeps only the lines that identify changes: file and hunk
// headers, binary markers, and added/removed lines. Context lines and file
// metadata (index, modes, rename info) are dropped.
func FilterChanges(rawDiff string) string {
	if rawDiff == "" {
		return ""
	}

	lines := strings.Split(rawDiff, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(trimmed, "diff --git "),
			strings.HasPrefix(trimmed, "@@"),
			strings.HasPrefix(trimmed, "+++"),
			strings.HasPrefix(trimmed, "---"),
			strings.HasPrefix(trimmed, "Binary files "):
			kept = append(kept, trimmed)
		case strings.HasPrefix(trimmed, "+"), strings.HasPrefix(trimmed, "-"):
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, "\n")
}

// Sentinel diff payloads used by callers to signal "no differences". They are
// internal contract values and are never rendered directly: Render substitutes
// the localized diff.no_differences message for them.
const (
	NoDifferences       = "No differences found."
	NoDifferencesTarget = "(No differences found between workspace skill and target skill)"
)

// Render formats the entire diff with an optional top summary banner and syntax colorization.
func Render(rawDiff string, opts RenderOptions) string {
	trimmed := strings.TrimSpace(rawDiff)
	if trimmed == "" || trimmed == NoDifferences || trimmed == NoDifferencesTarget {
		if opts.IncludeSummary {
			return FormatSummary(Summary{}, opts.Color, opts.Width)
		}
		return i18n.T("diff.no_differences")
	}

	summary := Parse(rawDiff)
	lines := parseLines(rawDiff)
	if opts.ChangesOnly {
		lines = changeLines(lines)
	}

	var b strings.Builder
	if opts.IncludeSummary {
		b.WriteString(FormatSummary(summary, opts.Color, opts.Width))
		b.WriteString("\n\n")
	}
	b.WriteString(renderLines(lines, opts))
	return b.String()
}

// renderLines renders parsed diff lines, optionally with a line-number gutter.
func renderLines(lines []diffLine, opts RenderOptions) string {
	gutter := 0
	if opts.LineNumbers {
		maxNo := 0
		for _, l := range lines {
			if l.oldNo > maxNo {
				maxNo = l.oldNo
			}
			if l.newNo > maxNo {
				maxNo = l.newNo
			}
		}
		gutter = len(strconv.Itoa(max(maxNo, 1)))
	}

	var sb strings.Builder
	for i, l := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if l.kind == diffLineSeparator {
			styled := l.text
			if opts.Color {
				styled = lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Faint(true).Render(l.text)
			}
			prefix := "  "
			if gutter > 0 {
				prefix = strings.Repeat(" ", gutter*2+3)
			}
			sb.WriteString(prefix + styled)
			continue
		}
		if gutter > 0 {
			sb.WriteString(renderGutter(l, gutter, opts.Color))
		}
		sb.WriteString(styleDiffLine(l, opts.Color))
	}
	return sb.String()
}

func renderGutter(l diffLine, width int, color bool) string {
	oldCol, newCol := "", ""
	if l.oldNo > 0 {
		oldCol = strconv.Itoa(l.oldNo)
	}
	if l.newNo > 0 {
		newCol = strconv.Itoa(l.newNo)
	}
	gutter := fmt.Sprintf("%*s %*s │ ", width, oldCol, width, newCol)
	if color {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#636E72")).Render(gutter)
	}
	return gutter
}

func styleDiffLine(l diffLine, color bool) string {
	if !color {
		return l.text
	}

	styleAdded := lipgloss.NewStyle().Foreground(lipgloss.Color("#2ED573"))
	styleDeleted := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4757"))
	styleHunk := lipgloss.NewStyle().Foreground(lipgloss.Color("#00CEC9")).Bold(true)
	styleHunkCtx := lipgloss.NewStyle().Foreground(lipgloss.Color("#81ECEC")).Faint(true)
	styleDiffHeader := lipgloss.NewStyle().Foreground(lipgloss.Color("#70A1FF")).Bold(true)
	styleFileOld := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF7675")).Bold(true)
	styleFileNew := lipgloss.NewStyle().Foreground(lipgloss.Color("#55EFC4")).Bold(true)
	styleMeta := lipgloss.NewStyle().Foreground(lipgloss.Color("#747D8C"))
	styleContext := lipgloss.NewStyle().Foreground(lipgloss.Color("#DFE6E9"))

	switch l.kind {
	case diffLineAdd:
		return styleAdded.Render(l.text)
	case diffLineDel:
		return styleDeleted.Render(l.text)
	case diffLineHunk:
		if idx := strings.Index(l.text[2:], "@@"); idx != -1 {
			hunkRange := l.text[:2+idx+2]
			trailing := l.text[2+idx+2:]
			styled := styleHunk.Render(hunkRange)
			if strings.TrimSpace(trailing) != "" {
				styled += " " + styleHunkCtx.Render(strings.TrimSpace(trailing))
			}
			return styled
		}
		return styleHunk.Render(l.text)
	case diffLineFileHeader:
		if strings.HasPrefix(l.text, "---") {
			return styleFileOld.Render(l.text)
		}
		return styleFileNew.Render(l.text)
	case diffLineMeta:
		if strings.HasPrefix(l.text, "diff --git ") {
			return styleDiffHeader.Render(l.text)
		}
		return styleMeta.Render(l.text)
	default:
		return styleContext.Render(l.text)
	}
}
