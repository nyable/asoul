package cli

import (
	"bytes"
	"strings"
	"testing"

	"asoul/internal/i18n"
	"asoul/internal/progress"
)

func TestRenderCLIProgressDeterminate(t *testing.T) {
	line := renderCLIProgress(1, 2, "skill-a")
	if !strings.Contains(line, "50% (1/2)") {
		t.Fatalf("expected 50%% (1/2) in progress line, got %q", line)
	}
	if !strings.Contains(line, "skill-a") {
		t.Fatalf("expected label in progress line, got %q", line)
	}
}

func TestRenderCLIProgressIndeterminate(t *testing.T) {
	line := renderCLIProgress(0, 0, "working")
	if strings.Contains(line, "%") {
		t.Fatalf("indeterminate line must not show a percentage, got %q", line)
	}
}

func TestCLIProgressLabel(t *testing.T) {
	i18n.Init("en")
	defer i18n.Init("en")

	if got := cliProgressLabel(progress.Update{Phase: progress.PhaseUnknown, Text: "raw"}); got != "raw" {
		t.Fatalf("unknown phase should keep raw text, got %q", got)
	}
	if got := cliProgressLabel(progress.Update{Phase: progress.PhaseDeploy, Text: "skill-a"}); got != "skill-a" {
		t.Fatalf("item phase should keep concrete detail, got %q", got)
	}
	if got := cliProgressLabel(progress.Update{Phase: progress.PhaseReceiving, Current: 1, Total: 2}); got == "" || strings.Contains(got, "progress.") {
		t.Fatalf("git phase should map to a localized label, got %q", got)
	}
}

func TestOutputProgressInactiveWhenNotTTY(t *testing.T) {
	o := NewOutput()
	var buf bytes.Buffer
	o.SetWriters(&buf, &buf)

	o.Progress(1, 2, "skill-a")
	o.ClearProgress()
	if buf.Len() != 0 {
		t.Fatalf("progress output must be silent without a TTY, wrote %q", buf.String())
	}
	if o.ProgressActive() {
		t.Fatal("ProgressActive must be false for a non-TTY writer")
	}
}

func TestOutputProgressInactiveInJSONMode(t *testing.T) {
	o := NewOutput()
	o.json = true
	if o.ProgressActive() {
		t.Fatal("ProgressActive must be false in JSON mode")
	}
	if o.ProgressFunc() != nil {
		t.Fatal("ProgressFunc must be nil when progress output is inactive")
	}
}
