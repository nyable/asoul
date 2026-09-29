package tui

import (
	"strings"
	"testing"
	"time"

	"asoul/internal/progress"
)

func TestRenderProgressBarDeterminate(t *testing.T) {
	rendered := renderProgressBar(1, 2, 20, 0)
	if !strings.Contains(rendered, "50% (1/2)") {
		t.Fatalf("expected 50%% (1/2) in determininate bar, got %q", rendered)
	}
	if !strings.Contains(rendered, "█") {
		t.Fatalf("expected filled segment in bar, got %q", rendered)
	}
}

func TestRenderProgressBarIndeterminate(t *testing.T) {
	rendered := renderProgressBar(0, 0, 20, 3)
	if strings.Contains(rendered, "%") {
		t.Fatalf("indeterminate bar must not show a percentage, got %q", rendered)
	}
	if !strings.Contains(rendered, "█") {
		t.Fatalf("expected animated segment in indeterminate bar, got %q", rendered)
	}
}

func TestProgressTickAdvancesOnlyWhileLoading(t *testing.T) {
	m := &Model{width: 80, height: 24, loading: true}
	model, cmd := m.Update(progressTickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("tick must reschedule itself")
	}
	if got := model.(*Model).progressFrame; got != 1 {
		t.Fatalf("expected frame 1 while loading, got %d", got)
	}

	idle := &Model{width: 80, height: 24}
	model, _ = idle.Update(progressTickMsg(time.Now()))
	if got := model.(*Model).progressFrame; got != 0 {
		t.Fatalf("frame must not advance while idle, got %d", got)
	}
}

func TestProgressDetailText(t *testing.T) {
	if got := progressDetailText(progress.PhaseReceiving, "Receiving objects: 45% (100/200)"); got == "Receiving objects: 45% (100/200)" {
		t.Fatalf("git counter phase should collapse to a localized label, got %q", got)
	}
	if got := progressDetailText(progress.PhaseDeploy, "skill-a"); got != "skill-a" {
		t.Fatalf("item phase should keep its concrete detail, got %q", got)
	}
	if got := progressDetailText(progress.PhaseUnknown, "raw output"); got != "raw output" {
		t.Fatalf("unknown phase should keep raw text, got %q", got)
	}
}

func TestProgressBlockDeterminate(t *testing.T) {
	m := &Model{
		width:           80,
		height:          24,
		loading:         true,
		notice:          "Deploying skills",
		progressPhase:   progress.PhaseDeploy,
		progressCurrent: 2,
		progressTotal:   4,
	}
	block := m.progressBlock()
	if !strings.Contains(block, "50% (2/4)") {
		t.Fatalf("expected determinate percentage in block, got:\n%s", block)
	}
	if !strings.Contains(block, "Deploying skills") {
		t.Fatalf("expected label in block, got:\n%s", block)
	}
}
