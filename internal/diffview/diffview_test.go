package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestParseEmpty(t *testing.T) {
	s := Parse("")
	if s.TotalFiles != 0 || s.Additions != 0 || s.Deletions != 0 {
		t.Fatalf("expected 0 files, got %+v", s)
	}

	s2 := Parse("   \n\t  ")
	if s2.TotalFiles != 0 {
		t.Fatalf("expected 0 files for whitespace diff, got %+v", s2)
	}
}

func TestParseGitDiffSingleFile(t *testing.T) {
	raw := `diff --git a/SKILL.md b/SKILL.md
index 1234567..89abcdef 100644
--- a/SKILL.md
+++ b/SKILL.md
@@ -1,3 +1,4 @@
 name: pdf
-version: 1.0.0
+version: 1.0.1
+author: asoul
 description: test
`
	s := Parse(raw)
	if s.TotalFiles != 1 {
		t.Fatalf("expected 1 file, got %d", s.TotalFiles)
	}
	if s.Additions != 2 {
		t.Fatalf("expected 2 additions, got %d", s.Additions)
	}
	if s.Deletions != 1 {
		t.Fatalf("expected 1 deletion, got %d", s.Deletions)
	}
	if len(s.Files) != 1 || s.Files[0].Path != "SKILL.md" {
		t.Fatalf("expected file path SKILL.md, got %+v", s.Files)
	}
	if s.Files[0].Additions != 2 || s.Files[0].Deletions != 1 {
		t.Fatalf("expected file additions=2, deletions=1, got %+v", s.Files[0])
	}
}

func TestParseGitDiffMultiFile(t *testing.T) {
	raw := `diff --git a/tmp/dirA/SKILL.md b/tmp/dirB/SKILL.md
index 1111111..2222222 100644
--- a/tmp/dirA/SKILL.md
+++ b/tmp/dirB/SKILL.md
@@ -1 +1 @@
-old skill
+new skill
diff --git a/tmp/dirA/scripts/run.sh b/tmp/dirB/scripts/run.sh
index 3333333..4444444 100755
--- a/tmp/dirA/scripts/run.sh
+++ b/tmp/dirB/scripts/run.sh
@@ -1,2 +1,3 @@
 echo start
-exit 1
+echo middle
+exit 0
`
	s := Parse(raw)
	if s.TotalFiles != 2 {
		t.Fatalf("expected 2 files, got %d", s.TotalFiles)
	}
	if s.Additions != 3 {
		t.Fatalf("expected 3 additions, got %d", s.Additions)
	}
	if s.Deletions != 2 {
		t.Fatalf("expected 2 deletions, got %d", s.Deletions)
	}
	if s.Files[0].Path != "SKILL.md" {
		t.Fatalf("expected first file SKILL.md, got %q", s.Files[0].Path)
	}
	if s.Files[1].Path != "scripts/run.sh" {
		t.Fatalf("expected second file scripts/run.sh, got %q", s.Files[1].Path)
	}
}

func TestParseLCSDiff(t *testing.T) {
	raw := `--- original
+++ updated
   "model": "gpt-3.5",
-  "limit": 1000,
+  "limit": 2000,
+  "extra": true
`
	s := Parse(raw)
	if s.TotalFiles != 1 {
		t.Fatalf("expected 1 file, got %d", s.TotalFiles)
	}
	if s.Additions != 2 || s.Deletions != 1 {
		t.Fatalf("expected +2 -1, got +%d -%d", s.Additions, s.Deletions)
	}
	if s.Files[0].Path != "(config)" {
		t.Fatalf("expected (config), got %q", s.Files[0].Path)
	}
}

func TestFormatBadge(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	// Zero changes without color
	badgeNoColor := FormatBadge(Summary{}, false)
	if !strings.Contains(badgeNoColor, "[ ") || !strings.Contains(badgeNoColor, " ]") {
		t.Fatalf("unexpected badge: %q", badgeNoColor)
	}

	// 1 file with color
	s1 := Summary{
		TotalFiles: 1,
		Additions:  5,
		Deletions:  2,
	}
	b1 := FormatBadge(s1, false)
	if !strings.Contains(b1, "+5") || !strings.Contains(b1, "-2") {
		t.Fatalf("expected +5 -2 in badge, got %q", b1)
	}

	b1Color := FormatBadge(s1, true)
	if !strings.Contains(b1Color, "\x1b[") {
		t.Fatalf("expected ANSI styling in colored badge, got %q", b1Color)
	}
}

func TestRenderNoColorHasNoANSI(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	raw := `diff --git a/test.txt b/test.txt
--- a/test.txt
+++ b/test.txt
@@ -1 +1 @@
-hello
+world
`
	rendered := Render(raw, RenderOptions{
		Color:          false,
		IncludeSummary: true,
		Width:          80,
	})

	if strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected no ANSI escape sequences when Color=false, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "+world") || !strings.Contains(rendered, "-hello") {
		t.Fatalf("rendered output missing diff lines:\n%s", rendered)
	}
	if !strings.Contains(rendered, "+1") || !strings.Contains(rendered, "-1") {
		t.Fatalf("rendered output missing summary numbers:\n%s", rendered)
	}
}

func TestRenderColorHasDistinctStyles(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	raw := `diff --git a/test.txt b/test.txt
--- a/test.txt
+++ b/test.txt
@@ -1 +1 @@
-hello
+world
`
	rendered := Render(raw, RenderOptions{
		Color:          true,
		IncludeSummary: true,
		Width:          80,
	})

	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected ANSI escape sequences when Color=true, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "test.txt") {
		t.Fatalf("expected file name in summary, got:\n%s", rendered)
	}
}

func TestRenderEmptyDiff(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)

	rendered := Render("", RenderOptions{
		Color:          false,
		IncludeSummary: true,
		Width:          80,
	})
	if !strings.Contains(rendered, "未检测到内容差异") && !strings.Contains(rendered, "No differences") {
		t.Fatalf("expected no diff indicator, got:\n%s", rendered)
	}
}

func TestFilterChanges(t *testing.T) {
	raw := strings.Join([]string{
		"diff --git a/opencode.json b/opencode.json",
		"index 111..222 100644",
		"--- a/opencode.json",
		"+++ b/opencode.json",
		"@@ -1,3 +1,4 @@",
		` "provider": {`,
		`-  "old": 1,`,
		`+  "new": 2,`,
		`+  "extra": 3,`,
		` }`,
	}, "\n")

	got := FilterChanges(raw)
	if strings.Contains(got, `"provider": {`) || strings.Contains(got, "index ") {
		t.Fatalf("expected context and metadata removed, got:\n%s", got)
	}
	for _, want := range []string{"diff --git", "@@", `-  "old": 1,`, `+  "new": 2,`, `+  "extra": 3,`} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in filtered diff, got:\n%s", want, got)
		}
	}
}

func TestRenderChangesOnly(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)

	raw := "--- original\n+++ updated\n@@ -1,3 +1,3 @@\n keep-me\n-drop-me\n+add-me\n"

	full := Render(raw, RenderOptions{Color: false})
	if !strings.Contains(full, "keep-me") {
		t.Fatalf("full render should include the context line, got:\n%s", full)
	}

	only := Render(raw, RenderOptions{Color: false, ChangesOnly: true})
	if strings.Contains(only, "keep-me") {
		t.Fatalf("changes-only render should drop the context line, got:\n%s", only)
	}
	if !strings.Contains(only, "add-me") || !strings.Contains(only, "drop-me") {
		t.Fatalf("changes-only render should keep changed lines, got:\n%s", only)
	}
}

func TestRenderLineNumbersSimple(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)

	raw := "--- original\n+++ updated\n line1\n-old\n+new\n"
	out := Render(raw, RenderOptions{Color: false, LineNumbers: true})

	for _, want := range []string{"1 1 │", "│ -old", "│ +new"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in numbered diff, got:\n%s", want, out)
		}
	}
}

func TestRenderLineNumbersGitHunks(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)

	raw := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -10,2 +10,3 @@\n ctx\n-old\n+new\n+extra\n"
	out := Render(raw, RenderOptions{Color: false, LineNumbers: true})

	for _, want := range []string{"10 10 │", "-old", "+new", "+extra"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in git numbered diff, got:\n%s", want, out)
		}
	}
}

func TestRenderChangesOnlySeparatesFragments(t *testing.T) {
	lipgloss.SetColorProfile(termenv.Ascii)

	raw := "--- original\n+++ updated\n ctx0\n-a\n+b\n ctx1\n ctx2\n-c\n+d\n"

	full := Render(raw, RenderOptions{Color: false})
	if strings.Contains(full, "⋯") {
		t.Fatalf("full diff should not contain a fragment separator, got:\n%s", full)
	}

	only := Render(raw, RenderOptions{Color: false, ChangesOnly: true})
	if !strings.Contains(only, "⋯") {
		t.Fatalf("changes-only should separate skipped context, got:\n%s", only)
	}
	for _, want := range []string{"-a", "+b", "-c", "+d"} {
		if !strings.Contains(only, want) {
			t.Fatalf("expected %q in changes-only, got:\n%s", want, only)
		}
	}
	if strings.Contains(only, "ctx1") {
		t.Fatalf("context lines should be dropped, got:\n%s", only)
	}
}
