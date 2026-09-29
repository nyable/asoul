package app_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"asoul/internal/agent"
	"asoul/internal/app"
	"asoul/internal/cache"
	"asoul/internal/config"
	"asoul/internal/model"
	"asoul/internal/skill"
	"asoul/internal/state"
)

func setupTestService(t *testing.T) (*app.Service, string, func()) {
	tmpDir, err := os.MkdirTemp("", "asoul-app-test-*")
	if err != nil {
		t.Fatal(err)
	}

	wsRoot := filepath.Join(tmpDir, "ws")
	cfgFile := filepath.Join(tmpDir, "config", "config.jsonc")
	stateFile := filepath.Join(tmpDir, "state", "deployments.json")
	cacheDir := filepath.Join(tmpDir, "cache", "git")

	cfgMgr, err := config.NewManager(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	stateMgr, err := state.NewManager(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	cacheMgr, err := cache.NewManager(cacheDir)
	if err != nil {
		t.Fatal(err)
	}

	svc := app.NewService(wsRoot, cfgMgr, stateMgr, cacheMgr)

	cleanup := func() {
		_ = os.RemoveAll(tmpDir)
	}
	return svc, tmpDir, cleanup
}

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")

	// 1. Initialize Workspace
	abs, err := svc.InitWorkspace(ctx, wsRoot)
	if err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	if abs != wsRoot {
		t.Fatalf("expected workspace root %s, got %s", wsRoot, abs)
	}

	// 2. Doctor check
	docReport, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if docReport.HasError {
		t.Fatalf("Doctor report has unexpected errors: %+v", docReport.Items)
	}

	// 3. Create Managed Skill
	if err := svc.NewSkill(ctx, "managed-helper"); err != nil {
		t.Fatalf("NewSkill failed: %v", err)
	}

	statuses, err := svc.Status(ctx, false)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if len(statuses) != 1 || statuses[0].ID != "managed-helper" {
		t.Fatalf("unexpected statuses after NewSkill: %+v", statuses)
	}
	if statuses[0].ManagedStatus != model.StatusClean {
		t.Fatalf("expected status clean, got %s", statuses[0].ManagedStatus)
	}

	// 4. Test Local Modification Detection
	skillMD := filepath.Join(wsRoot, "skills", "managed-helper", "SKILL.md")
	_ = os.WriteFile(skillMD, []byte("---\nname: managed-helper\ndescription: Modified\n---\n# Modified\n"), 0644)

	statuses, err = svc.Status(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if statuses[0].ManagedStatus != model.StatusModified {
		t.Fatalf("expected status modified, got %s", statuses[0].ManagedStatus)
	}

	// 5. Local Source Add, Check, and Update
	localSourceDir := filepath.Join(tmpDir, "ext-sources", "local-tool")
	_ = os.MkdirAll(localSourceDir, 0755)
	_ = os.WriteFile(filepath.Join(localSourceDir, "SKILL.md"), skill.GenerateSkillMD("local-tool", "Original local tool"), 0644)

	if err := svc.AddLocal(ctx, localSourceDir, false); err != nil {
		t.Fatalf("AddLocal failed: %v", err)
	}

	st, _, err := svc.Show(ctx, "local-tool")
	if err != nil {
		t.Fatalf("Show local-tool failed: %v", err)
	}
	if st.ManagedStatus != model.StatusClean || st.Source.Type != model.SourceTypeLocal {
		t.Fatalf("unexpected local tool state: %+v", st)
	}

	// Modify external local source
	_ = os.WriteFile(filepath.Join(localSourceDir, "SKILL.md"), skill.GenerateSkillMD("local-tool", "Updated local tool content"), 0644)

	checkRes, err := svc.Check(ctx, []string{"local-tool"})
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	var localCheck *model.SkillStatus
	for i := range checkRes {
		if checkRes[i].ID == "local-tool" {
			localCheck = &checkRes[i]
		}
	}
	if localCheck == nil || localCheck.Upstream != model.UpstreamSourceChanged {
		t.Fatalf("expected local source changed, got %+v", localCheck)
	}

	// Update local skill
	updatedStat, err := svc.Update(ctx, "local-tool", false, false)
	if err != nil {
		t.Fatalf("Update local-tool failed: %v", err)
	}
	if updatedStat.ManagedStatus != model.StatusClean {
		t.Fatalf("expected clean after update, got %s", updatedStat.ManagedStatus)
	}

	// 6. Git Source Add, Check, Diff, Update (using purely local Git repo)
	remoteRepoDir := filepath.Join(tmpDir, "remote-git-repo")
	_ = os.MkdirAll(remoteRepoDir, 0755)
	runGit(t, remoteRepoDir, "init")
	runGit(t, remoteRepoDir, "config", "user.name", "testuser")
	runGit(t, remoteRepoDir, "config", "user.email", "test@test.local")
	// Commit 1
	skillGitPath := filepath.Join(remoteRepoDir, "skills", "git-helper")
	_ = os.MkdirAll(skillGitPath, 0755)
	_ = os.WriteFile(filepath.Join(skillGitPath, "SKILL.md"), skill.GenerateSkillMD("git-helper", "Git helper initial"), 0644)
	runGit(t, remoteRepoDir, "add", ".")
	runGit(t, remoteRepoDir, "commit", "-m", "initial commit")

	// Add git skill
	gitURL := "file://" + remoteRepoDir
	if err := svc.AddGit(ctx, gitURL, "HEAD", "skills/git-helper", false); err != nil {
		t.Fatalf("AddGit failed: %v", err)
	}

	gitStat, _, err := svc.Show(ctx, "git-helper")
	if err != nil {
		t.Fatalf("Show git-helper failed: %v", err)
	}
	if gitStat.ManagedStatus != model.StatusClean || gitStat.Source.Type != model.SourceTypeGit {
		t.Fatalf("unexpected git skill state: %+v", gitStat)
	}

	// Commit 2 on remote repo
	_ = os.WriteFile(filepath.Join(skillGitPath, "SKILL.md"), skill.GenerateSkillMD("git-helper", "Git helper updated version 2"), 0644)
	runGit(t, remoteRepoDir, "commit", "-am", "second commit")

	// Check upstream for git-helper
	checkRes, err = svc.Check(ctx, []string{"git-helper"})
	if err != nil {
		t.Fatalf("Check git-helper failed: %v", err)
	}
	var gitCheck *model.SkillStatus
	for i := range checkRes {
		if checkRes[i].ID == "git-helper" {
			gitCheck = &checkRes[i]
		}
	}
	if gitCheck == nil || gitCheck.Upstream != model.UpstreamUpdateAvailable {
		t.Fatalf("expected git update available, got %+v", gitCheck)
	}

	// Test Diff
	diffText, err := svc.Diff(ctx, "git-helper")
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}
	if !strings.Contains(diffText, "Git helper updated version 2") {
		t.Fatalf("expected diff to contain new version, got:\n%s", diffText)
	}

	// Test Update
	upGitStat, err := svc.Update(ctx, "git-helper", false, false)
	if err != nil {
		t.Fatalf("Update git-helper failed: %v", err)
	}
	if upGitStat.ManagedStatus != model.StatusClean {
		t.Fatalf("expected clean after git update, got %s", upGitStat.ManagedStatus)
	}

	// 7. Target Management and Deploy
	targetDir := filepath.Join(tmpDir, "agents", "codex", "skills")
	if err := svc.TargetAdd("codex", targetDir); err != nil {
		t.Fatalf("TargetAdd failed: %v", err)
	}

	targets, err := svc.TargetList()
	if err != nil || len(targets) != 1 {
		t.Fatalf("expected 1 target, got %v (err: %v)", targets, err)
	}

	// Deploy git-helper to codex
	if err := svc.Deploy(ctx, "git-helper", "codex", false); err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}
	deployedFile := filepath.Join(targetDir, "git-helper", "SKILL.md")
	if _, err := os.Stat(deployedFile); err != nil {
		t.Fatalf("deployed file not found at %s", deployedFile)
	}

	// Modify target file manually (simulate user edit in agent target)
	_ = os.WriteFile(deployedFile, []byte("tampered content"), 0644)

	// Deploy without force must be blocked!
	if err := svc.Deploy(ctx, "git-helper", "codex", false); err == nil {
		t.Fatalf("expected deploy to fail when target was modified without force")
	}

	// Deploy with force must succeed
	if err := svc.Deploy(ctx, "git-helper", "codex", true); err != nil {
		t.Fatalf("Deploy with force failed: %v", err)
	}

	// Undeploy
	if err := svc.Undeploy(ctx, "git-helper", "codex", false); err != nil {
		t.Fatalf("Undeploy failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "git-helper")); !os.IsNotExist(err) {
		t.Fatalf("expected target directory to be removed after undeploy")
	}

	// 8. Cache List and Prune
	cacheList, err := svc.CacheList()
	if err != nil {
		t.Fatalf("CacheList failed: %v", err)
	}
	if len(cacheList) == 0 {
		t.Fatalf("expected at least 1 cached git repo, got 0")
	}

	// Remove git-helper from workspace
	if err := svc.Remove(ctx, "git-helper", true); err != nil {
		t.Fatalf("Remove git-helper failed: %v", err)
	}

	// Cache prune should clean the now unreferenced git cache
	prunedCount, _, err := svc.CachePrune(ctx)
	if err != nil {
		t.Fatalf("CachePrune failed: %v", err)
	}
	if prunedCount == 0 {
		t.Fatalf("expected at least 1 cache pruned, got 0")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git command failed (%s): %s (err: %v)", strings.Join(args, " "), string(out), err)
	}
}

func TestDoctor_NoTargetChecks(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}

	// Add a non-existent target (e.g. codex)
	nonExistentTarget := filepath.Join(tmpDir, "agents", "codex", "skills")
	if err := svc.TargetAdd("codex", nonExistentTarget); err != nil {
		t.Fatalf("TargetAdd failed: %v", err)
	}

	rep, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	// Ensure NO items check or warn about agent targets like "Target codex"
	for _, item := range rep.Items {
		if strings.HasPrefix(item.Name, "Target") || strings.Contains(item.Name, "codex") || strings.Contains(item.Message, "codex") {
			t.Fatalf("Doctor should not check agent targets, but found item: %+v", item)
		}
	}
}

func TestDoctor_CachedProjectsValidation(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}

	// 1. Initially no projects cached
	rep, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	foundNoProjects := false
	for _, item := range rep.Items {
		if item.Name == "Cached Projects" && strings.Contains(item.Message, "no cached projects") {
			foundNoProjects = true
			if item.Status != app.CheckOK {
				t.Fatalf("expected CheckOK for no cached projects, got %v", item.Status)
			}
		}
	}
	if !foundNoProjects {
		t.Fatalf("expected 'Cached Projects: no cached projects' in report items")
	}

	// 2. Add valid project directory
	proj1 := filepath.Join(tmpDir, "project-one")
	if err := os.MkdirAll(proj1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectAdd(proj1); err != nil {
		t.Fatalf("ProjectAdd failed: %v", err)
	}

	rep, err = svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	foundProj1 := false
	for _, item := range rep.Items {
		if item.Name == "Cached Projects" && strings.Contains(item.Message, "1 cached project valid") {
			foundProj1 = true
			if item.Status != app.CheckOK {
				t.Fatalf("expected CheckOK, got %v", item.Status)
			}
		}
	}
	if !foundProj1 {
		t.Fatalf("expected report to show 1 valid cached project, got: %+v", rep.Items)
	}

	// 3. Add second valid project
	proj2 := filepath.Join(tmpDir, "project-two")
	if err := os.MkdirAll(proj2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProjectAdd(proj2); err != nil {
		t.Fatalf("ProjectAdd failed: %v", err)
	}

	rep, err = svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}
	foundProj2 := false
	for _, item := range rep.Items {
		if item.Name == "Cached Projects" && strings.Contains(item.Message, "2 cached projects valid") {
			foundProj2 = true
			if item.Status != app.CheckOK {
				t.Fatalf("expected CheckOK, got %v", item.Status)
			}
		}
	}
	if !foundProj2 {
		t.Fatalf("expected report to show 2 valid cached projects, got: %+v", rep.Items)
	}

	// 4. Inject an invalid (non-existent) project path and a regular file into projects state
	cfgFile := filepath.Join(tmpDir, "config", "config.jsonc")
	cfgMgr, err := config.NewManager(cfgFile)
	if err != nil {
		t.Fatal(err)
	}

	nonExistentProj := filepath.Join(tmpDir, "project-deleted")
	fileAsProj := filepath.Join(tmpDir, "some-file.txt")
	if err := os.WriteFile(fileAsProj, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := cfgMgr.AddRecentProject(nonExistentProj); err != nil {
		t.Fatal(err)
	}
	if err := cfgMgr.AddRecentProject(fileAsProj); err != nil {
		t.Fatal(err)
	}

	rep, err = svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	var foundMissingWarn, foundNotDirWarn, foundPartialOK bool
	for _, item := range rep.Items {
		if strings.Contains(item.Name, "project-deleted") {
			foundMissingWarn = true
			if item.Status != app.CheckWarn {
				t.Fatalf("expected CheckWarn for missing project, got %v", item.Status)
			}
			if !strings.Contains(item.Message, "does not exist on disk") {
				t.Fatalf("unexpected message: %s", item.Message)
			}
		}
		if strings.Contains(item.Name, "some-file.txt") {
			foundNotDirWarn = true
			if item.Status != app.CheckWarn {
				t.Fatalf("expected CheckWarn for file as project, got %v", item.Status)
			}
			if !strings.Contains(item.Message, "is not a directory") {
				t.Fatalf("unexpected message: %s", item.Message)
			}
		}
		if item.Name == "Cached Projects" && strings.Contains(item.Message, "2 of 4 cached projects valid") {
			foundPartialOK = true
		}
	}

	if !foundMissingWarn {
		t.Fatalf("expected warning for missing project directory, items: %+v", rep.Items)
	}
	if !foundNotDirWarn {
		t.Fatalf("expected warning for non-directory project, items: %+v", rep.Items)
	}
	if !foundPartialOK {
		t.Fatalf("expected '2 of 4 cached projects valid', items: %+v", rep.Items)
	}
}

func TestServiceDeployDisabledTargetEnforced(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	if err := svc.NewSkill(ctx, "sample-skill"); err != nil {
		t.Fatalf("NewSkill failed: %v", err)
	}

	targetDir := filepath.Join(tmpDir, "agents", "test-agent", "skills")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := svc.TargetAdd("test-agent", targetDir); err != nil {
		t.Fatalf("TargetAdd failed: %v", err)
	}

	// Disable the target
	if err := svc.SetTargetEnabled("test-agent", false); err != nil {
		t.Fatalf("SetTargetEnabled failed: %v", err)
	}

	// Deploy without force should fail
	err := svc.Deploy(ctx, "sample-skill", "test-agent", false)
	if err == nil {
		t.Fatalf("expected Deploy to fail for disabled target without force")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Deploy with force should succeed
	if err := svc.Deploy(ctx, "sample-skill", "test-agent", true); err != nil {
		t.Fatalf("expected Deploy with force to succeed, got %v", err)
	}

	// Undeploy should succeed
	if err := svc.Undeploy(ctx, "sample-skill", "test-agent", false); err != nil {
		t.Fatalf("expected Undeploy to succeed, got %v", err)
	}
}

func TestApp_AddGitBatch(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	wsRoot := filepath.Join(tmpDir, "ws")
	if _, err := svc.InitWorkspace(ctx, wsRoot); err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}

	remoteRepoDir := filepath.Join(tmpDir, "batch-git-repo")
	_ = os.MkdirAll(remoteRepoDir, 0755)
	runGit(t, remoteRepoDir, "init")
	runGit(t, remoteRepoDir, "config", "user.name", "testuser")
	runGit(t, remoteRepoDir, "config", "user.email", "test@test.local")

	skill1 := filepath.Join(remoteRepoDir, "skills", "batch-alpha")
	skill2 := filepath.Join(remoteRepoDir, "skills", "batch-beta")
	_ = os.MkdirAll(skill1, 0755)
	_ = os.MkdirAll(skill2, 0755)
	_ = os.WriteFile(filepath.Join(skill1, "SKILL.md"), skill.GenerateSkillMD("batch-alpha", "Alpha skill description"), 0644)
	_ = os.WriteFile(filepath.Join(skill2, "SKILL.md"), skill.GenerateSkillMD("batch-beta", "Beta skill description"), 0644)

	runGit(t, remoteRepoDir, "add", ".")
	runGit(t, remoteRepoDir, "commit", "-m", "initial batch commit")

	gitURL := "file://" + remoteRepoDir

	// Discover first
	discovered, err := svc.DiscoverGit(ctx, gitURL, "HEAD")
	if err != nil {
		t.Fatalf("DiscoverGit failed: %v", err)
	}
	if len(discovered) != 2 {
		t.Fatalf("expected 2 discovered skills, got %d", len(discovered))
	}

	// Add batch with fetch=false (reusing cache from discover)
	subpaths := []string{"skills/batch-alpha", "skills/batch-beta"}
	res, err := svc.AddGitBatch(ctx, gitURL, "HEAD", subpaths, false, false, false)
	if err != nil {
		t.Fatalf("AddGitBatch failed: %v", err)
	}
	if len(res.Added) != 2 {
		t.Fatalf("expected 2 added skills, got: %+v", res)
	}
	if len(res.Failed) != 0 {
		t.Fatalf("unexpected failures: %+v", res.Failed)
	}

	// Verify both exist in workspace
	statAlpha, _, err := svc.Show(ctx, "batch-alpha")
	if err != nil || statAlpha.ManagedStatus != model.StatusClean {
		t.Fatalf("Show batch-alpha failed: stat=%+v, err=%v", statAlpha, err)
	}
	statBeta, _, err := svc.Show(ctx, "batch-beta")
	if err != nil || statBeta.ManagedStatus != model.StatusClean {
		t.Fatalf("Show batch-beta failed: stat=%+v, err=%v", statBeta, err)
	}

	// Adding again should be idempotent (skipped)
	res2, err := svc.AddGitBatch(ctx, gitURL, "HEAD", subpaths, false, false, false)
	if err != nil {
		t.Fatalf("second AddGitBatch failed: %v", err)
	}
	if len(res2.Skipped) != 2 {
		t.Fatalf("expected 2 skipped skills on idempotent re-add, got %+v", res2)
	}
}

func TestEnrichModelConfigRulesOnlySkipsCatalog(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	cfgPath := filepath.Join(tmpDir, "ws", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"provider":{"acme":{"models":{"m1":{"name":"Old"}}}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	rulesPath := filepath.Join(tmpDir, "rules.json")
	if err := os.WriteFile(rulesPath, []byte(`{"rules":[{"name":"r1","pattern":"^m1$","override":{"name":"From Rules"}}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	summary, err := svc.EnrichModelConfig(ctx, "opencode", agent.EnrichOptions{
		FilePath:  cfgPath,
		RulesPath: rulesPath,
		RulesOnly: true,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("rules-only enrich should not require models.dev: %v", err)
	}
	if summary == nil || !summary.Modified {
		t.Fatal("expected the rule override to produce a modification")
	}
	if !strings.Contains(summary.DiffText, "From Rules") {
		t.Fatalf("expected rule output in diff, got:\n%s", summary.DiffText)
	}
}

func TestEnrichModelConfigRulesOnlyRequiresRules(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "xdg-config"))

	cfgPath := filepath.Join(tmpDir, "ws", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"provider":{"acme":{"models":{"m1":{}}}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := svc.EnrichModelConfig(ctx, "opencode", agent.EnrichOptions{
		FilePath:  cfgPath,
		RulesOnly: true,
		DryRun:    true,
	})
	if err == nil {
		t.Fatal("expected rules-only mode without a rule file to fail")
	}
}

func TestAgentConfigProvidersAndEdits(t *testing.T) {
	ctx := context.Background()
	svc, tmpDir, cleanup := setupTestService(t)
	defer cleanup()

	cfgPath := filepath.Join(tmpDir, "ws", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
  // provider comment
  "provider": {
    "acme": {
      "name": "Old Name", // inline comment
      "npm": "@ai-sdk/openai-compatible",
      "options": { "apiKey": "super-secret-key" },
      "models": { "m1": { "name": "M1" } }
    }
  }
}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	providers, _, err := svc.ListAgentProviders(ctx, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	if providers[0].Name != "Old Name" || providers[0].ModelCount != 1 || providers[0].NPM == "" {
		t.Fatalf("unexpected provider item: %+v", providers[0])
	}

	path := []string{"provider", "acme", "name"}

	// Preview must not touch the file.
	preview, err := svc.PreviewAgentConfigValue(ctx, "opencode", cfgPath, path, "New Name", false)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Modified || !strings.Contains(preview.DiffText, "New Name") {
		t.Fatalf("expected preview diff with new value, got %+v", preview)
	}
	onDisk, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(onDisk), "New Name") {
		t.Fatal("preview must not write to disk")
	}

	// Apply writes the value and keeps comments.
	applied, err := svc.ApplyAgentConfigValue(ctx, "opencode", cfgPath, path, "New Name", false)
	if err != nil {
		t.Fatal(err)
	}
	if applied.BackupFile == "" {
		t.Fatal("expected a backup file")
	}
	updated, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(updated), "New Name") {
		t.Fatalf("expected updated value, got:\n%s", updated)
	}
	if !strings.Contains(string(updated), "// provider comment") || !strings.Contains(string(updated), "// inline comment") {
		t.Fatalf("expected comments preserved, got:\n%s", updated)
	}

	// Delete a nested option.
	if _, err := svc.ApplyAgentConfigValue(ctx, "opencode", cfgPath, []string{"provider", "acme", "options", "apiKey"}, nil, true); err != nil {
		t.Fatal(err)
	}
	updated, _ = os.ReadFile(cfgPath)
	if strings.Contains(string(updated), "apiKey") {
		t.Fatalf("expected apiKey removed, got:\n%s", updated)
	}

	// Creating a missing intermediate object should work.
	if _, err := svc.ApplyAgentConfigValue(ctx, "opencode", cfgPath, []string{"provider", "acme", "options", "baseURL"}, "https://example.com", false); err != nil {
		t.Fatal(err)
	}
	updated, _ = os.ReadFile(cfgPath)
	if !strings.Contains(string(updated), "baseURL") || !strings.Contains(string(updated), "https://example.com") {
		t.Fatalf("expected baseURL written, got:\n%s", updated)
	}
}

func TestConfigModelEnrichMode(t *testing.T) {
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	cfg, err := svc.Config()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.GetModelEnrichMode(); got != model.ModelEnrichModeRemote {
		t.Fatalf("expected default enrich mode %q, got %q", model.ModelEnrichModeRemote, got)
	}

	if err := svc.ConfigSet("model_enrich_mode", model.ModelEnrichModeIncremental); err != nil {
		t.Fatal(err)
	}
	cfg, _ = svc.Config()
	if got := cfg.GetModelEnrichMode(); got != model.ModelEnrichModeIncremental {
		t.Fatalf("expected persisted enrich mode %q, got %q", model.ModelEnrichModeIncremental, got)
	}

	if err := svc.ConfigSet("model_enrich_mode", "bogus"); err == nil {
		t.Fatal("expected invalid enrich mode to be rejected")
	}
}
