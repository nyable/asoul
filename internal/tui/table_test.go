package tui

import (
	"testing"

	"asoul/internal/model"

	"github.com/charmbracelet/lipgloss"
)

func TestPadCellAlignment(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected int
	}{
		{
			name:     "pure ascii",
			input:    "my-skill",
			width:    20,
			expected: 20,
		},
		{
			name:     "pure chinese header",
			input:    "技能名称",
			width:    20,
			expected: 20,
		},
		{
			name:     "mixed chinese and english",
			input:    "技能 test-123",
			width:    20,
			expected: 20,
		},
		{
			name:     "ansi colored string",
			input:    formatStatusBadge(model.StatusClean, false),
			width:    14,
			expected: 14,
		},
		{
			name:     "ansi colored string selected",
			input:    formatStatusBadge(model.StatusModified, true),
			width:    14,
			expected: 14,
		},
		{
			name:     "truncate long ascii",
			input:    "very-long-skill-name-that-exceeds-width",
			width:    15,
			expected: 15,
		},
		{
			name:     "truncate long chinese",
			input:    "超长技能名称超过指定列宽应当安全截断",
			width:    15,
			expected: 15,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			padded := padCell(tc.input, tc.width)
			actualWidth := lipgloss.Width(padded)
			if actualWidth != tc.expected {
				t.Fatalf("expected width %d, got %d (raw string: %q)", tc.expected, actualWidth, padded)
			}
		})
	}
}

func TestComputeSkillColWidths(t *testing.T) {
	nameW80, statW80, locHashW80, srcW80, verW80 := computeSkillColWidths(80)
	total80 := 2 + nameW80 + 1 + statW80 + 1 + locHashW80 + 1 + srcW80 + 1 + verW80
	if total80 > 80 {
		t.Fatalf("total width for 80 cols terminal exceeded 80: %d", total80)
	}

	nameW120, statW120, locHashW120, srcW120, verW120 := computeSkillColWidths(120)
	total120 := 2 + nameW120 + 1 + statW120 + 1 + locHashW120 + 1 + srcW120 + 1 + verW120
	if total120 > 120 {
		t.Fatalf("total width for 120 cols terminal exceeded 120: %d", total120)
	}
	if nameW120 <= nameW80 {
		t.Fatalf("expected wider name column on wider terminal, got %d <= %d", nameW120, nameW80)
	}
	if srcW120 <= srcW80 {
		t.Fatalf("expected wider source column on wider terminal, got %d <= %d", srcW120, srcW80)
	}
}

func TestTableColumnAlignmentMatches(t *testing.T) {
	nameW, statW, locHashW, srcW, verW := computeSkillColWidths(80)

	// Chinese header
	hName := padCell("技能名称", nameW)
	hStat := padCell("本地状态", statW)
	hLocHash := padCell("本地Hash", locHashW)
	hSrc := padCell("来源", srcW)
	hVer := padCell("来源版本/状态", verW)

	testSkill := model.SkillStatus{
		ID:            "my-awesome-skill",
		ManagedStatus: model.StatusClean,
		CurrentHash:   "h1:KZ1teG9EAHB",
		Source: model.SourceSpec{
			Type: model.SourceTypeGit,
			URL:  "https://github.com/anthropics/skills.git",
		},
		RecordedCommit: "34040c9c568585f6929bedeaad110ad08f079624",
		Upstream:       model.UpstreamUpToDate,
	}

	// Data row with English skill name and ANSI colored badge
	dName := padCell(testSkill.ID, nameW)
	dStat := padCell(formatStatusBadge(testSkill.ManagedStatus, false), statW)
	dLocHash := padCell(formatLocalHash(testSkill), locHashW)
	dSrc := padCell(formatSourceCombined(testSkill.Source, false), srcW)
	dVer := padCell(formatSourceVersion(testSkill, false), verW)

	cols := []struct {
		name        string
		headerWidth int
		dataWidth   int
		expected    int
	}{
		{"Name", lipgloss.Width(hName), lipgloss.Width(dName), nameW},
		{"Status", lipgloss.Width(hStat), lipgloss.Width(dStat), statW},
		{"LocalHash", lipgloss.Width(hLocHash), lipgloss.Width(dLocHash), locHashW},
		{"Source", lipgloss.Width(hSrc), lipgloss.Width(dSrc), srcW},
		{"SourceVersion", lipgloss.Width(hVer), lipgloss.Width(dVer), verW},
	}

	for _, c := range cols {
		if c.headerWidth != c.expected {
			t.Errorf("col %s header width = %d, want %d", c.name, c.headerWidth, c.expected)
		}
		if c.dataWidth != c.expected {
			t.Errorf("col %s data width = %d, want %d", c.name, c.dataWidth, c.expected)
		}
		if c.headerWidth != c.dataWidth {
			t.Errorf("col %s mismatch: header width %d != data width %d", c.name, c.headerWidth, c.dataWidth)
		}
	}
}

func statusW(w int) int { return w }

func TestComputeSettingsColWidths(t *testing.T) {
	itemW80 := computeSettingsColWidths(80)
	total80 := 2 + itemW80
	if total80 > 80 {
		t.Fatalf("settings col width for 80 cols exceeded 80: %d", total80)
	}

	itemW120 := computeSettingsColWidths(120)
	total120 := 2 + itemW120
	if total120 > 120 {
		t.Fatalf("settings col width for 120 cols exceeded 120: %d", total120)
	}
	if itemW120 <= itemW80 {
		t.Fatalf("expected wider item column on 120 cols terminal, got %d <= %d", itemW120, itemW80)
	}
}

func TestSettingsTableColumnAlignment(t *testing.T) {
	itemW := computeSettingsColWidths(80)

	// Chinese header
	hKey := padCell("配置项", itemW)

	// Row data
	dKey := padCell("界面显示语言", itemW)

	if lipgloss.Width(hKey) != itemW || lipgloss.Width(dKey) != itemW {
		t.Fatalf("item column alignment mismatch: %d vs %d (want %d)", lipgloss.Width(hKey), lipgloss.Width(dKey), itemW)
	}
}

func TestComputeModelColWidths(t *testing.T) {
	modelW80, ctxW80, priceW80, capsW80, varW80, statusW80 := computeModelColWidths(80)
	total80 := 4 + modelW80 + 1 + ctxW80 + 1 + priceW80 + 1 + capsW80 + 1 + varW80 + 1 + statusW80 + 2
	if total80 > 80 {
		t.Fatalf("total width for 80 cols terminal exceeded 80: %d", total80)
	}
	if statusW80 < 18 {
		t.Fatalf("expected statusW >= 18 to fit provider badges, got %d", statusW80)
	}

	modelW120, ctxW120, priceW120, capsW120, varW120, statusW120 := computeModelColWidths(120)
	total120 := 4 + modelW120 + 1 + ctxW120 + 1 + priceW120 + 1 + capsW120 + 1 + varW120 + 1 + statusW120 + 2
	if total120 > 120 {
		t.Fatalf("total width for 120 cols terminal exceeded 120: %d", total120)
	}
	if statusW120 < 20 {
		t.Fatalf("expected statusW >= 20 on wide terminal, got %d", statusW120)
	}
}

func TestModelTableColumnAlignment(t *testing.T) {
	modelW, ctxW, priceW, capsW, varW, statusW := computeModelColWidths(100)
	_ = modelW
	_ = ctxW
	_ = priceW
	_ = capsW
	_ = varW

	// Chinese header
	hStatus := padCell("状态", statusW)

	// Data rows with provider badge
	dStatusEnriched := padCell("[已补全: deepseek]", statusW)
	dStatusPending := padCell("[待补全: deepseek]", statusW)
	dStatusLocal := padCell("[已补全: 本地]", statusW)

	if lipgloss.Width(hStatus) != statusW {
		t.Fatalf("header status alignment mismatch: got %d want %d", lipgloss.Width(hStatus), statusW)
	}
	if lipgloss.Width(dStatusEnriched) != statusW {
		t.Fatalf("dStatusEnriched alignment mismatch: got %d want %d", lipgloss.Width(dStatusEnriched), statusW)
	}
	if lipgloss.Width(dStatusPending) != statusW {
		t.Fatalf("dStatusPending alignment mismatch: got %d want %d", lipgloss.Width(dStatusPending), statusW)
	}
	if lipgloss.Width(dStatusLocal) != statusW {
		t.Fatalf("dStatusLocal alignment mismatch: got %d want %d", lipgloss.Width(dStatusLocal), statusW)
	}
}
