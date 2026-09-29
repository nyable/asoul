package state

import (
	"path/filepath"
	"testing"
)

func TestProjectStateManager(t *testing.T) {
	tempDir := t.TempDir()
	stateFile := filepath.Join(tempDir, "projects.json")

	mgr, err := NewProjectStateManager(stateFile)
	if err != nil {
		t.Fatalf("failed to create ProjectStateManager: %v", err)
	}

	// 1. Initial should be empty
	projs, err := mgr.GetProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projs) != 0 {
		t.Fatalf("expected empty projects, got %v", projs)
	}

	// 2. Add projects
	p1 := filepath.Join(tempDir, "p1")
	p2 := filepath.Join(tempDir, "p2")
	if err := mgr.AddProject(p1); err != nil {
		t.Fatalf("AddProject p1 failed: %v", err)
	}
	if err := mgr.AddProject(p2); err != nil {
		t.Fatalf("AddProject p2 failed: %v", err)
	}

	projs, err = mgr.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects failed: %v", err)
	}
	if len(projs) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projs))
	}
	// p2 was added last, so it's at front
	if projs[0] != p2 || projs[1] != p1 {
		t.Fatalf("expected order [p2, p1], got %v", projs)
	}

	// 3. Enable / Disable
	enabled, err := mgr.IsProjectEnabled(p1)
	if err != nil || !enabled {
		t.Fatalf("expected p1 enabled by default, got %v, err: %v", enabled, err)
	}

	if err := mgr.SetProjectEnabled(p1, false); err != nil {
		t.Fatalf("SetProjectEnabled false failed: %v", err)
	}
	enabled, err = mgr.IsProjectEnabled(p1)
	if err != nil || enabled {
		t.Fatalf("expected p1 disabled, got %v, err: %v", enabled, err)
	}

	if err := mgr.SetProjectEnabled(p1, true); err != nil {
		t.Fatalf("SetProjectEnabled true failed: %v", err)
	}
	enabled, err = mgr.IsProjectEnabled(p1)
	if err != nil || !enabled {
		t.Fatalf("expected p1 re-enabled, got %v, err: %v", enabled, err)
	}

	// 4. Remove project
	if err := mgr.RemoveProject(p1); err != nil {
		t.Fatalf("RemoveProject failed: %v", err)
	}
	projs, err = mgr.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects failed: %v", err)
	}
	if len(projs) != 1 || projs[0] != p2 {
		t.Fatalf("expected only [p2], got %v", projs)
	}
}
