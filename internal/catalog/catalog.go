package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"asoul/internal/fsx"
	"asoul/internal/model"
)

// CatalogFileName is the standard catalog file name.
const CatalogFileName = "catalog.json"

// SkillsDirName is the directory containing managed skills.
const SkillsDirName = "skills"

// FindWorkspaceRoot discovers the workspace root according to the priority:
// 1. explicit root flag
// 2. ASOUL_ROOT environment variable
// 3. directory specified in user configuration (default_root or first in workspaces)
// 4. standalone fallback (if no config is provided): upward search from current directory
// 5. default fallback ~/.config/asoul
func FindWorkspaceRoot(flagRoot string, cfg *model.Config) (string, error) {
	// 1. Explicit flag
	if flagRoot != "" {
		expanded, err := fsx.ExpandUser(flagRoot)
		if err != nil {
			return "", err
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", err
		}
		if !fsx.FileExists(filepath.Join(abs, CatalogFileName)) {
			return abs, fmt.Errorf("no %s found in specified workspace root: %s", CatalogFileName, abs)
		}
		return abs, nil
	}

	// 2. ASOUL_ROOT env
	if envRoot := os.Getenv("ASOUL_ROOT"); envRoot != "" {
		expanded, err := fsx.ExpandUser(envRoot)
		if err != nil {
			return "", err
		}
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return "", err
		}
		if fsx.FileExists(filepath.Join(abs, CatalogFileName)) {
			return abs, nil
		}
		return abs, fmt.Errorf("no %s found in ASOUL_ROOT: %s", CatalogFileName, abs)
	}

	// 3. User configuration: ALWAYS use the workspace directory specified in configuration
	if cfg != nil {
		targetWS := cfg.DefaultRoot
		if targetWS == "" && len(cfg.Workspaces) > 0 {
			targetWS = cfg.Workspaces[0]
		}
		if targetWS != "" {
			expanded, err := fsx.ExpandUser(targetWS)
			if err != nil {
				return "", err
			}
			abs, err := filepath.Abs(expanded)
			if err != nil {
				return "", err
			}
			if !fsx.FileExists(filepath.Join(abs, CatalogFileName)) {
				return abs, fmt.Errorf("asoul workspace at %s is not initialized (run 'asoul init' or 'asoul workspace init')", abs)
			}
			return abs, nil
		}
	}

	// 4. Standalone fallback (when no config is provided, e.g. standalone tests): upwards from current directory
	if cfg == nil {
		cwd, err := os.Getwd()
		if err == nil {
			curr := cwd
			for {
				if fsx.FileExists(filepath.Join(curr, CatalogFileName)) {
					return curr, nil
				}
				parent := filepath.Dir(curr)
				if parent == curr {
					break
				}
				curr = parent
			}
		}
	}

	// 5. Default fallback ~/.config/asoul
	defWS, err := fsx.ExpandUser("~/.config/asoul")
	if err == nil {
		abs, err := filepath.Abs(defWS)
		if err == nil {
			if !fsx.FileExists(filepath.Join(abs, CatalogFileName)) {
				return abs, fmt.Errorf("asoul workspace at %s is not initialized (run 'asoul init' or 'asoul workspace init')", abs)
			}
			return abs, nil
		}
	}

	return "", fmt.Errorf("no asoul workspace found (use --root, set ASOUL_ROOT, or run 'asoul init')")
}

// CatalogPath returns the full path to catalog.json in workspace.
func CatalogPath(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, CatalogFileName)
}

// SkillsDir returns the full path to skills/ in workspace.
func SkillsDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, SkillsDirName)
}

// SkillDir returns the full path to a specific skill inside skills/.
func SkillDir(workspaceRoot, id string) string {
	return filepath.Join(workspaceRoot, SkillsDirName, id)
}

// InitWorkspace initializes a new workspace directory with catalog.json and skills/.
func InitWorkspace(workspaceRoot string) error {
	expanded, err := fsx.ExpandUser(workspaceRoot)
	if err != nil {
		expanded = workspaceRoot
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return err
	}

	catFile := CatalogPath(abs)
	if fsx.FileExists(catFile) {
		return fmt.Errorf("workspace already exists at %s", abs)
	}

	skillsDir := SkillsDir(abs)
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("failed to create skills directory: %w", err)
	}

	cat := &model.Catalog{
		Version: 1,
		Skills:  make(map[string]model.SkillRecord),
	}

	if err := Save(abs, cat); err != nil {
		return err
	}

	_ = ensureGitignore(abs)
	return nil
}

func ensureGitignore(workspaceRoot string) error {
	ignorePath := filepath.Join(workspaceRoot, ".gitignore")
	defaultRules := []string{
		"# asoul concurrency locks",
		"*.lock",
		".asoul.lock",
		".asoul-deploy.lock",
		"",
		"# Temporary staging and backups",
		".asoul-tmp-*",
		".asoul-backup-*",
		".asoul-deploy-tmp-*",
		".asoul-deploy-backup-*",
		".tmp-*",
		"",
		"# Machine-local configuration overrides",
		"config.local.jsonc",
		"*.local.jsonc",
		"*.local.json",
		"",
		"# OS metadata",
		".DS_Store",
		"Thumbs.db",
	}

	if !fsx.FileExists(ignorePath) {
		content := strings.Join(defaultRules, "\n") + "\n"
		return os.WriteFile(ignorePath, []byte(content), 0644)
	}

	existingBytes, err := os.ReadFile(ignorePath)
	if err != nil {
		return nil
	}
	existingStr := string(existingBytes)
	if !strings.Contains(existingStr, ".asoul.lock") {
		toAppend := "\n# asoul workspace ignore\n" + strings.Join(defaultRules[1:], "\n") + "\n"
		f, err := os.OpenFile(ignorePath, os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			_, _ = f.WriteString(toAppend)
		}
	}
	return nil
}

// Load loads catalog.json from workspaceRoot.
func Load(workspaceRoot string) (*model.Catalog, error) {
	catFile := CatalogPath(workspaceRoot)
	if !fsx.FileExists(catFile) {
		return nil, fmt.Errorf("catalog file not found: %s", catFile)
	}

	data, err := os.ReadFile(catFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog file: %w", err)
	}

	cat := &model.Catalog{
		Skills: make(map[string]model.SkillRecord),
	}
	if err := json.Unmarshal(data, cat); err != nil {
		return nil, fmt.Errorf("failed to parse catalog.json: %w", err)
	}

	if cat.Skills == nil {
		cat.Skills = make(map[string]model.SkillRecord)
	}

	return cat, nil
}

// Save writes catalog.json atomically to workspaceRoot.
func Save(workspaceRoot string, cat *model.Catalog) error {
	if cat.Version == 0 {
		cat.Version = 1
	}
	if cat.Skills == nil {
		cat.Skills = make(map[string]model.SkillRecord)
	}

	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode catalog to json: %w", err)
	}
	data = append(data, '\n')

	catFile := CatalogPath(workspaceRoot)
	return fsx.AtomicWriteFile(catFile, data, 0644)
}
