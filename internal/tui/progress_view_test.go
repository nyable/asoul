package tui_test

import (
	"context"
	"strings"
	"testing"

	"asoul/internal/i18n"
	"asoul/internal/progress"
	"asoul/internal/tui"
)

func TestLoadingViewShowsDeterminateProgress(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	i18n.Init("en")
	defer i18n.Init("en")

	m := tui.NewModel(ctx, svc)
	m.BeginLoadingForTest("Deploying skills")
	model, _ := m.Update(tui.NewProgressMsgForTest(progress.PhaseDeploy, 1, 2, "skill-a"))
	view := model.(*tui.Model).View()

	if !strings.Contains(view, "50% (1/2)") {
		t.Fatalf("expected determinate progress percentage in loading view, got:\n%s", view)
	}
	if !strings.Contains(view, "skill-a") {
		t.Fatalf("expected progress detail in loading view, got:\n%s", view)
	}
}

func TestLoadingViewShowsIndeterminateBar(t *testing.T) {
	ctx := context.Background()
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	i18n.Init("en")
	defer i18n.Init("en")

	m := tui.NewModel(ctx, svc)
	m.BeginLoadingForTest("Running health checks")
	view := m.View()

	if !strings.Contains(view, "Running health checks") {
		t.Fatalf("expected loading label in view, got:\n%s", view)
	}
	if strings.Contains(view, "% (") {
		t.Fatalf("indeterminate loading view must not show a percentage, got:\n%s", view)
	}
}
