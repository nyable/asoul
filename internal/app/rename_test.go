package app_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/catalog"
	"asoul/internal/hash"
	"asoul/internal/skill"
)

func TestRenameSkill(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatal(err)
	}

	// 1. Create a skill
	origID := "original-skill"
	if err := svc.NewSkill(ctx, origID); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	// Verify original exists
	origDir := catalog.SkillDir(wsRoot, origID)
	if _, err := os.Stat(origDir); err != nil {
		t.Fatalf("expected origDir to exist: %v", err)
	}

	// 2. Rename to invalid ID
	if err := svc.RenameSkill(ctx, origID, "Invalid_ID!"); err == nil {
		t.Fatalf("expected error for invalid skill ID, got nil")
	}

	// 3. Rename non-existent skill
	if err := svc.RenameSkill(ctx, "non-existent", "valid-target"); err == nil {
		t.Fatalf("expected error for non-existent skill, got nil")
	}

	// 4. Create another skill and attempt collision
	otherID := "other-skill"
	if err := svc.NewSkill(ctx, otherID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RenameSkill(ctx, origID, otherID); err == nil {
		t.Fatalf("expected collision error when renaming to existing skill ID, got nil")
	}

	// 5. Successful rename
	newID := "renamed-skill"
	if err := svc.RenameSkill(ctx, origID, newID); err != nil {
		t.Fatalf("RenameSkill failed: %v", err)
	}

	// Old dir should no longer exist
	if _, err := os.Stat(origDir); !os.IsNotExist(err) {
		t.Fatalf("expected oldDir to be removed, but stat returned: %v", err)
	}

	// New dir must exist
	newDir := catalog.SkillDir(wsRoot, newID)
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("expected newDir to exist: %v", err)
	}

	// SKILL.md frontmatter name must match newID
	parsed, _, err := skill.ParseSkillMD(filepath.Join(newDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to parse renamed skill dir: %v", err)
	}
	if parsed.Name != newID {
		t.Fatalf("expected SKILL.md name to be %q, got %q", newID, parsed.Name)
	}

	// Catalog must be updated
	cat, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := cat.Skills[origID]; exists {
		t.Fatalf("expected old ID %q to be removed from catalog", origID)
	}
	rec, exists := cat.Skills[newID]
	if !exists {
		t.Fatalf("expected new ID %q in catalog", newID)
	}
	expectedHash, _ := hash.HashDir(newDir)
	if rec.Resolved.ContentHash != expectedHash {
		t.Fatalf("content hash mismatch: expected %s, got %s", expectedHash, rec.Resolved.ContentHash)
	}
}

func TestAddGitWithAliasAndPreserveOnUpdate(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatal(err)
	}

	// Setup local git repo
	gitRepo := filepath.Join(tmpDir, "remote.git")
	_ = os.MkdirAll(gitRepo, 0755)

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = gitRepo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.name", "testuser")
	runGit("config", "user.email", "test@test.local")

	skillSubpath := "tools/calc"
	fullSkillDir := filepath.Join(gitRepo, skillSubpath)
	_ = os.MkdirAll(fullSkillDir, 0755)
	_ = os.WriteFile(filepath.Join(fullSkillDir, "SKILL.md"), skill.GenerateSkillMD("calc", "Calculator tool"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "init calc skill")

	// 1. Add skill with custom alias "custom-calc"
	aliasID := "custom-calc"
	repoURL := "file://" + gitRepo
	if err := svc.AddGitCustom(ctx, repoURL, "", skillSubpath, aliasID, false, false); err != nil {
		t.Fatalf("AddGitCustom failed: %v", err)
	}

	// Verify alias directory exists and has rewritten frontmatter
	aliasDir := catalog.SkillDir(wsRoot, aliasID)
	meta, _, err := skill.ParseSkillMD(filepath.Join(aliasDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to parse aliased skill: %v", err)
	}
	if meta.Name != aliasID {
		t.Fatalf("expected parsed name %q, got %q", aliasID, meta.Name)
	}

	// Verify catalog entry
	cat, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := cat.Skills[aliasID]
	if !ok {
		t.Fatalf("expected %q in catalog", aliasID)
	}
	if rec.Source.Path != skillSubpath {
		t.Fatalf("expected upstream source path %q, got %q", skillSubpath, rec.Source.Path)
	}

	// 2. Commit an update upstream
	_ = os.WriteFile(filepath.Join(fullSkillDir, "SKILL.md"), []byte("---\nname: calc\ndescription: Updated Calculator\n---\n# Calculator v2\n"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "v2 update")

	// 3. Diff should succeed and compare against the aliased skill
	diff, err := svc.Diff(ctx, aliasID)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diff, "Updated Calculator") {
		t.Fatalf("expected diff to show upstream change, got:\n%s", diff)
	}

	// 4. Update should succeed and retain the alias "custom-calc"
	if _, err := svc.Update(ctx, aliasID, false, false); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify still named "custom-calc" after update
	metaUpdated, _, err := skill.ParseSkillMD(filepath.Join(aliasDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("failed to parse updated skill: %v", err)
	}
	if metaUpdated.Name != aliasID {
		t.Fatalf("expected skill name to remain %q after update, got %q", aliasID, metaUpdated.Name)
	}
	if !strings.Contains(metaUpdated.Description, "Updated Calculator") {
		t.Fatalf("expected description to be updated, got %q", metaUpdated.Description)
	}
}

func TestBatchIntraCollisionAndResolution(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatal(err)
	}

	gitRepo := filepath.Join(tmpDir, "dups.git")
	_ = os.MkdirAll(gitRepo, 0755)

	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = gitRepo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
		}
	}

	runGit("init")
	runGit("config", "user.name", "testuser")
	runGit("config", "user.email", "test@test.local")

	// Two skills with the same ID "worker" in different folders
	_ = os.MkdirAll(filepath.Join(gitRepo, "plugins", "worker"), 0755)
	_ = os.MkdirAll(filepath.Join(gitRepo, "skills", "worker"), 0755)
	_ = os.WriteFile(filepath.Join(gitRepo, "plugins", "worker", "SKILL.md"), skill.GenerateSkillMD("worker", "Plugin Worker"), 0644)
	_ = os.WriteFile(filepath.Join(gitRepo, "skills", "worker", "SKILL.md"), skill.GenerateSkillMD("worker", "Core Worker"), 0644)

	runGit("add", ".")
	runGit("commit", "-m", "add duplicate skills")

	repoURL := "file://" + gitRepo
	paths := []string{"plugins/worker", "skills/worker"}

	// 1. Without alias: second duplicate should fail cleanly
	res, err := svc.AddGitBatch(ctx, repoURL, "", paths, false, false, true)
	if err != nil {
		t.Fatalf("AddGitBatch unexpected fatal error: %v", err)
	}
	if len(res.Added) != 1 {
		t.Fatalf("expected 1 skill added, got %d", len(res.Added))
	}
	if len(res.Failed) != 1 {
		t.Fatalf("expected 1 skill failure due to duplicate ID, got %d", len(res.Failed))
	}

	// 2. Clean workspace
	if err := svc.Remove(ctx, "worker", true); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// 3. With alias resolving collision
	aliases := map[string]string{
		"plugins/worker": "plugin-worker",
		"skills/worker":  "core-worker",
	}
	resAliased, err := svc.AddGitBatchWithOptions(ctx, repoURL, "", paths, aliases, nil, false, false, true)
	if err != nil {
		t.Fatalf("AddGitBatchWithOptions unexpected error: %v", err)
	}
	if len(resAliased.Added) != 2 {
		t.Fatalf("expected both skills added with aliases, got: %v (failed: %v)", resAliased.Added, resAliased.Failed)
	}

	// Verify both exist
	if _, err := os.Stat(catalog.SkillDir(wsRoot, "plugin-worker")); err != nil {
		t.Errorf("plugin-worker dir not found: %v", err)
	}
	if _, err := os.Stat(catalog.SkillDir(wsRoot, "core-worker")); err != nil {
		t.Errorf("core-worker dir not found: %v", err)
	}
}

func TestTargetedPerSkillOverwrite(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatal(err)
	}

	// Helper to make git repo
	makeRepo := func(name string, skills map[string]string) string {
		repoDir := filepath.Join(tmpDir, name)
		_ = os.MkdirAll(repoDir, 0755)
		run := func(args ...string) {
			cmd := exec.Command("git", args...)
			cmd.Dir = repoDir
			_ = cmd.Run()
		}
		run("init")
		run("config", "user.name", "testuser")
		run("config", "user.email", "test@test.local")
		for id, desc := range skills {
			dir := filepath.Join(repoDir, "skills", id)
			_ = os.MkdirAll(dir, 0755)
			_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), skill.GenerateSkillMD(id, desc), 0644)
		}
		run("add", ".")
		run("commit", "-m", "init")
		return repoDir
	}

	repoA := makeRepo("repoA.git", map[string]string{
		"skill-alpha": "Alpha from Repo A",
		"skill-beta":  "Beta from Repo A",
	})
	repoB := makeRepo("repoB.git", map[string]string{
		"skill-alpha": "Alpha from Repo B",
		"skill-beta":  "Beta from Repo B",
	})

	// 1. Install both from Repo A
	resA, err := svc.AddGitBatch(ctx, "file://"+repoA, "", []string{"skills/skill-alpha", "skills/skill-beta"}, false, false, true)
	if err != nil || len(resA.Added) != 2 {
		t.Fatalf("failed to add from repo A: %v", err)
	}

	// 2. Import from Repo B, but ONLY mark skill-alpha for overwrite!
	replaceSkills := map[string]bool{
		"skills/skill-alpha": true,
	}
	resB, err := svc.AddGitBatchWithOptions(ctx, "file://"+repoB, "", []string{"skills/skill-alpha", "skills/skill-beta"}, nil, replaceSkills, false, false, true)
	if err != nil {
		t.Fatalf("AddGitBatchWithOptions failed: %v", err)
	}

	// skill-alpha should be added (overwritten)
	if len(resB.Added) != 1 || resB.Added[0] != "skill-alpha" {
		t.Fatalf("expected only skill-alpha added/overwritten, got: %v", resB.Added)
	}
	// skill-beta should be in Failed because it was NOT marked for overwrite!
	if len(resB.Failed) != 1 {
		t.Fatalf("expected skill-beta to fail with conflict, got failures: %v", resB.Failed)
	}

	// Verify catalog: skill-alpha points to repoB, skill-beta still points to repoA!
	cat, err := catalog.Load(wsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cat.Skills["skill-alpha"].Source.URL, "repoB.git") {
		t.Errorf("skill-alpha expected source repoB.git, got: %s", cat.Skills["skill-alpha"].Source.URL)
	}
	if !strings.Contains(cat.Skills["skill-beta"].Source.URL, "repoA.git") {
		t.Errorf("skill-beta expected source repoA.git, got: %s", cat.Skills["skill-beta"].Source.URL)
	}
}
