package agent

import "strings"

// UnifiedDiff builds a unified diff (without line truncation) between two texts.
// It produces "--- original" / "+++ updated" headers followed by body lines
// prefixed with ' ', '+' or '-'.
func UnifiedDiff(oldText, newText string) string {
	oldLines := splitDiffLines(oldText)
	newLines := splitDiffLines(newText)

	m, n := len(oldLines), len(newLines)
	if m == 0 && n == 0 {
		return ""
	}

	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			if oldLines[i] == newLines[j] {
				dp[i+1][j+1] = dp[i][j] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i+1][j+1] = dp[i][j]
			} else {
				dp[i+1][j+1] = dp[i][j+1]
			}
		}
	}

	type edit struct {
		op   byte
		line string
	}
	var edits []edit
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			edits = append(edits, edit{op: ' ', line: oldLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			edits = append(edits, edit{op: '+', line: newLines[j-1]})
			j--
		} else if i > 0 && (j == 0 || dp[i][j-1] < dp[i-1][j]) {
			edits = append(edits, edit{op: '-', line: oldLines[i-1]})
			i--
		}
	}

	for l, r := 0, len(edits)-1; l < r; l, r = l+1, r-1 {
		edits[l], edits[r] = edits[r], edits[l]
	}

	hasDiff := false
	for _, e := range edits {
		if e.op != ' ' {
			hasDiff = true
			break
		}
	}
	if !hasDiff {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("--- original\n+++ updated\n")
	for _, e := range edits {
		sb.WriteByte(e.op)
		sb.WriteString(" ")
		sb.WriteString(e.line)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func splitDiffLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}
