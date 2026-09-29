package opencode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"asoul/internal/agent"
	"asoul/internal/agentmodel"
	"asoul/internal/fsx"
	"asoul/internal/modelrules"
	"asoul/internal/modelsdev"
)

func init() {
	agent.Register(agent.ChannelInfo{
		Name:         "opencode",
		Label:        "OpenCode",
		Description:  "OpenCode (~/.config/opencode/opencode.json|jsonc)",
		ConfigFormat: "jsonc",
	}, func() agent.AgentAdapter { return NewAdapter() })
}

// Adapter implements agent.AgentAdapter for OpenCode.
type Adapter struct{}

// NewAdapter creates an OpenCode adapter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// AgentName returns "opencode".
func (a *Adapter) AgentName() string {
	return "opencode"
}

// DetectConfigFiles finds candidate opencode.json or opencode.jsonc files.
// It prioritizes customConfigDirs (e.g. from TargetConfig.ResolveConfigDir()), then workspaceRoot, then user global directory.
func (a *Adapter) DetectConfigFiles(workspaceRoot string, customConfigDirs ...string) ([]string, error) {
	var candidates []string
	seen := make(map[string]bool)

	addCandidate := func(p string) {
		exp, err := fsx.ExpandUser(p)
		if err != nil {
			exp = p
		}
		abs, err := filepath.Abs(exp)
		if err == nil {
			exp = abs
		}
		if fsx.FileExists(exp) && !seen[exp] {
			seen[exp] = true
			candidates = append(candidates, exp)
		}
	}

	// 1. Custom config directories (e.g. reused from TargetConfig)
	for _, dir := range customConfigDirs {
		if dir == "" {
			continue
		}
		for _, name := range []string{"opencode.json", "opencode.jsonc"} {
			addCandidate(filepath.Join(dir, name))
		}
	}

	// 2. Current workspace locations
	if workspaceRoot != "" {
		for _, name := range []string{
			"opencode.json", "opencode.jsonc",
			filepath.Join(".opencode", "opencode.json"),
			filepath.Join(".opencode", "opencode.jsonc"),
		} {
			addCandidate(filepath.Join(workspaceRoot, name))
		}
	}

	// 3. User global config locations (~/.config/opencode/opencode.json / jsonc)
	userConfigDir, err := os.UserConfigDir()
	if err == nil {
		for _, name := range []string{"opencode.json", "opencode.jsonc"} {
			addCandidate(filepath.Join(userConfigDir, "opencode", name))
		}
	}

	return candidates, nil
}

// Enrich enriches opencode configuration file with models.dev data and custom rules.
func (a *Adapter) Enrich(ctx context.Context, matcher *modelsdev.Matcher, rules []modelrules.Rule, opts agent.EnrichOptions) (*agent.EnrichSummary, error) {
	targetFile := opts.FilePath
	if targetFile == "" {
		detected, err := a.DetectConfigFiles(opts.Workspace, opts.ConfigDir)
		if err != nil {
			return nil, err
		}
		if len(detected) == 0 {
			return nil, fmt.Errorf("no opencode configuration file found in target config_dir, workspace, or ~/.config/opencode")
		}
		targetFile = detected[0]
	} else {
		expanded, err := fsx.ExpandUser(targetFile)
		if err != nil {
			return nil, err
		}
		targetFile = expanded
		if !fsx.FileExists(targetFile) {
			return nil, fmt.Errorf("specified config file not found: %s", targetFile)
		}
	}

	doc, err := agent.LoadConfigDoc(targetFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", targetFile, err)
	}
	rawBytes := doc.Raw()

	root, err := doc.RootMap()
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON/JSONC in %s: %w", targetFile, err)
	}

	rawProviders, ok := root["provider"].(map[string]any)
	if !ok || len(rawProviders) == 0 {
		return &agent.EnrichSummary{
			ConfigFile: targetFile,
			Modified:   false,
		}, nil
	}

	var results []agent.ModelEnrichResult

	// Iterate custom providers
	for pID, pVal := range rawProviders {
		if opts.ProviderID != "" && pID != opts.ProviderID {
			continue
		}

		pMap, ok := pVal.(map[string]any)
		if !ok {
			continue
		}

		modelsMap, ok := pMap["models"].(map[string]any)
		if !ok || len(modelsMap) == 0 {
			continue
		}

		for mID, mVal := range modelsMap {
			currentModelMap, _ := mVal.(map[string]any)
			if currentModelMap == nil {
				currentModelMap = make(map[string]any)
			}

			origKeys := getKeys(currentModelMap)
			var matchedDevName string
			var matchedDevProvider string
			var devMap map[string]any

			// 1. Fetch metadata from models.dev
			if matcher != nil {
				if devModel, found := matcher.FindModel(mID); found {
					matchedDevName = devModel.ID
					matchedDevProvider = devModel.ProviderID
					devMap = devModelToMap(devModel)
				}
			}

			// Apply models.dev data
			if len(devMap) > 0 {
				if opts.Override {
					currentModelMap = agentmodel.DeepMerge(currentModelMap, devMap)
				} else {
					currentModelMap = agentmodel.FillMissing(currentModelMap, devMap)
				}
			}

			// 2. Apply Custom Rules
			applyCtx := modelrules.Context{
				ModelID:    mID,
				ProviderID: pID,
				ConfigDir:  opts.ConfigDir,
				Workspace:  opts.Workspace,
			}
			updatedModelMap, matchedRules, err := modelrules.ApplyRules(mID, rules, currentModelMap, applyCtx)
			if err != nil {
				return nil, err
			}
			if len(matchedRules) == 0 && matchedDevName != "" && matchedDevName != mID {
				// Try matching with normalized devModel ID
				applyCtx.ModelID = matchedDevName
				updatedModelMap, matchedRules, err = modelrules.ApplyRules(matchedDevName, rules, currentModelMap, applyCtx)
				if err != nil {
					return nil, err
				}
			}
			currentModelMap = updatedModelMap

			// Check newly added fields
			newKeys := getKeys(currentModelMap)
			addedFields := diffKeys(origKeys, newKeys)

			// 3. Persist onto the comment-preserving syntax tree
			if err := doc.Apply(currentModelMap, "provider", pID, "models", mID); err != nil {
				return nil, fmt.Errorf("failed to update model %s/%s: %w", pID, mID, err)
			}

			results = append(results, agent.ModelEnrichResult{
				ProviderID:      pID,
				ModelID:         mID,
				MatchedDev:      matchedDevName,
				MatchedProvider: matchedDevProvider,
				MatchedRules:    matchedRules,
				FieldsAdded:     addedFields,
			})
		}
	}

	updatedBytes := doc.Bytes()
	modified := string(rawBytes) != string(updatedBytes)
	if modified && len(updatedBytes) > 0 && updatedBytes[len(updatedBytes)-1] != '\n' {
		updatedBytes = append(updatedBytes, '\n')
	}

	diffText := agent.UnifiedDiff(string(rawBytes), string(updatedBytes))

	summary := &agent.EnrichSummary{
		ConfigFile: targetFile,
		Modified:   modified,
		Results:    results,
		DiffText:   diffText,
	}

	if opts.DryRun || !modified {
		return summary, nil
	}

	// Perform backup if enabled
	if !opts.NoBackup {
		backupPath, err := agent.CreateConfigFileBackup(targetFile, rawBytes, opts.BackupMaxVersions)
		if err != nil {
			return nil, fmt.Errorf("failed to create backup file: %w", err)
		}
		summary.BackupFile = backupPath
	}

	// Write updated content atomically
	if err := fsx.AtomicWriteFile(targetFile, updatedBytes, doc.Perm()); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", targetFile, err)
	}

	return summary, nil
}

func devModelToMap(m *modelsdev.ModelData) map[string]any {
	res := make(map[string]any)
	if m.Name != "" {
		res["name"] = m.Name
	}
	if m.Family != "" {
		res["family"] = m.Family
	}
	if m.Attachment {
		res["attachment"] = m.Attachment
	}
	if m.Reasoning {
		res["reasoning"] = m.Reasoning
	}
	if m.Temperature {
		res["temperature"] = m.Temperature
	}
	if m.StructuredOutput {
		res["structured_output"] = m.StructuredOutput
	}
	if m.ToolCall {
		res["tool_call"] = m.ToolCall
	}
	if m.ReleaseDate != "" {
		res["release_date"] = m.ReleaseDate
	}
	if m.Status != "" {
		res["status"] = m.Status
	}

	if m.Limit != nil {
		limit := make(map[string]any)
		if m.Limit.Context > 0 {
			limit["context"] = m.Limit.Context
		}
		if m.Limit.Output > 0 {
			limit["output"] = m.Limit.Output
		}
		if m.Limit.Input > 0 {
			limit["input"] = m.Limit.Input
		}
		if len(limit) > 0 {
			res["limit"] = limit
		}
	}

	if m.Cost != nil {
		cost := make(map[string]any)
		cost["input"] = m.Cost.Input
		cost["output"] = m.Cost.Output
		if m.Cost.CacheRead > 0 {
			cost["cache_read"] = m.Cost.CacheRead
		}
		if m.Cost.CacheWrite > 0 {
			cost["cache_write"] = m.Cost.CacheWrite
		}
		if m.Cost.ContextOver200k != nil {
			co := make(map[string]any)
			co["input"] = m.Cost.ContextOver200k.Input
			co["output"] = m.Cost.ContextOver200k.Output
			if m.Cost.ContextOver200k.CacheRead > 0 {
				co["cache_read"] = m.Cost.ContextOver200k.CacheRead
			}
			if m.Cost.ContextOver200k.CacheWrite > 0 {
				co["cache_write"] = m.Cost.ContextOver200k.CacheWrite
			}
			cost["context_over_200k"] = co
		}
		res["cost"] = cost
	}

	if m.Modalities != nil {
		mod := make(map[string]any)
		if len(m.Modalities.Input) > 0 {
			mod["input"] = m.Modalities.Input
		}
		if len(m.Modalities.Output) > 0 {
			mod["output"] = m.Modalities.Output
		}
		if len(mod) > 0 {
			res["modalities"] = mod
		}
	}

	return res
}

func getKeys(m map[string]any) map[string]bool {
	res := make(map[string]bool)
	for k := range m {
		res[k] = true
	}
	return res
}

func diffKeys(orig, updated map[string]bool) []string {
	var added []string
	for k := range updated {
		if !orig[k] {
			added = append(added, k)
		}
	}
	return added
}

// BuildDiff computes a unified diff between oldText and newText without line truncation.
func BuildDiff(oldText, newText string) string {
	return agent.UnifiedDiff(oldText, newText)
}

// buildDiff computes a unified diff; retained for internal callers/tests.
func buildDiff(oldText, newText string) string {
	return agent.UnifiedDiff(oldText, newText)
}
