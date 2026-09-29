package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"asoul/internal/catalog"
	"asoul/internal/fsx"
	"asoul/internal/model"
)

// CheckStatus represents the status of a single doctor check.
type CheckStatus string

const (
	CheckOK   CheckStatus = "OK"
	CheckWarn CheckStatus = "WARN"
	CheckErr  CheckStatus = "ERROR"
)

// CheckItem represents an individual diagnostic result.
//
// Name/Message hold canonical English text (the JSON contract and the display
// fallback). NameKey/MessageKey, when set, carry the i18n key and format
// arguments that presentation layers (TUI/CLI) use to render a localized
// string. Business packages stay language-neutral and never import i18n.
type CheckItem struct {
	Status      CheckStatus `json:"status"`
	Name        string      `json:"name"`
	Message     string      `json:"message"`
	NameKey     string      `json:"-"`
	NameArgs    []any       `json:"-"`
	MessageKey  string      `json:"-"`
	MessageArgs []any       `json:"-"`
}

// DisplayName renders the localized check name via tr (typically i18n.T),
// falling back to the canonical English name.
func (c CheckItem) DisplayName(tr func(string, ...any) string) string {
	if c.NameKey != "" {
		return tr(c.NameKey, c.NameArgs...)
	}
	return c.Name
}

// DisplayMessage renders the localized check message via tr, falling back to
// the canonical English message.
func (c CheckItem) DisplayMessage(tr func(string, ...any) string) string {
	if c.MessageKey != "" {
		return tr(c.MessageKey, c.MessageArgs...)
	}
	return c.Message
}

// DoctorReport contains all findings from the doctor diagnostics.
type DoctorReport struct {
	Items    []CheckItem `json:"items"`
	HasError bool        `json:"hasError"`
	HasWarn  bool        `json:"hasWarn"`
}

// Doctor executes comprehensive system and workspace diagnostics.
func (s *Service) Doctor(ctx context.Context) (*DoctorReport, error) {
	report := &DoctorReport{
		Items: make([]CheckItem, 0),
	}

	addItem := func(status CheckStatus, name, nameKey string, nameArgs []any, message, messageKey string, messageArgs []any) {
		report.Items = append(report.Items, CheckItem{
			Status:      status,
			Name:        name,
			NameKey:     nameKey,
			NameArgs:    nameArgs,
			Message:     message,
			MessageKey:  messageKey,
			MessageArgs: messageArgs,
		})
		if status == CheckErr {
			report.HasError = true
		} else if status == CheckWarn {
			report.HasWarn = true
		}
	}

	// 1. Check Git
	gitVer, err := s.gitClient.CheckInstalled(ctx)
	if err != nil {
		addItem(CheckErr, "Git Tool", "doctor.check.git", nil, err.Error(), "", nil)
	} else {
		addItem(CheckOK, "Git Tool", "doctor.check.git", nil, gitVer, "", nil)
	}

	// 2. Check Workspace
	if s.workspaceRoot == "" {
		addItem(CheckWarn, "Workspace", "doctor.check.workspace", nil,
			"no active workspace configured (run 'asoul init' or use --root)", "doctor.msg.no_workspace", nil)
		return report, nil
	}

	addItem(CheckOK, "Workspace", "doctor.check.workspace", nil, s.workspaceRoot, "", nil)

	// 3. Check Catalog.json
	cat, err := catalog.Load(s.workspaceRoot)
	if err != nil {
		addItem(CheckErr, "Catalog File", "doctor.check.catalog", nil,
			fmt.Sprintf("failed to load catalog.json: %v", err), "doctor.msg.catalog_load_failed", []any{err})
	} else {
		addItem(CheckOK, "Catalog File", "doctor.check.catalog", nil,
			fmt.Sprintf("catalog.json v%d valid", cat.Version), "doctor.msg.catalog_valid", []any{cat.Version})
	}

	// 4. Check Skills Directory & Temporary Leftovers
	skillsDir := catalog.SkillsDir(s.workspaceRoot)
	if !fsx.DirExists(skillsDir) {
		addItem(CheckErr, "Skills Directory", "doctor.check.skills_dir", nil,
			fmt.Sprintf("directory %s does not exist", skillsDir), "doctor.msg.skills_dir_missing", []any{skillsDir})
	} else {
		addItem(CheckOK, "Skills Directory", "doctor.check.skills_dir", nil, skillsDir, "", nil)

		// Check for leftover temporary files (.asoul-tmp-*, .asoul-backup-*)
		entries, _ := os.ReadDir(skillsDir)
		var leftovers []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".asoul-tmp-") || strings.HasPrefix(e.Name(), ".asoul-backup-") {
				leftovers = append(leftovers, e.Name())
			}
		}
		if len(leftovers) > 0 {
			joined := strings.Join(leftovers, ", ")
			addItem(CheckWarn, "Leftover Artifacts", "doctor.check.leftovers", nil,
				fmt.Sprintf("found %d temporary leftover directories: %s", len(leftovers), joined),
				"doctor.msg.leftovers_found", []any{len(leftovers), joined})
		} else {
			addItem(CheckOK, "Leftover Artifacts", "doctor.check.leftovers", nil,
				"no leftover staging directories found", "doctor.msg.leftovers_none", nil)
		}
	}

	// 5. Skills status checks
	if cat != nil {
		statuses, statErr := s.Status(ctx, false)
		if statErr != nil {
			addItem(CheckErr, "Skills Consistency", "doctor.check.consistency", nil, statErr.Error(), "", nil)
		} else {
			var cleanCount, modCount, unmanagedCount, invalidCount, missingCount int
			for _, st := range statuses {
				switch st.ManagedStatus {
				case model.StatusClean:
					cleanCount++
				case model.StatusModified:
					modCount++
				case model.StatusUnmanaged:
					unmanagedCount++
				case model.StatusInvalid:
					invalidCount++
				case model.StatusMissing:
					missingCount++
				}
			}

			summaryArgs := []any{len(statuses), cleanCount, modCount, unmanagedCount, invalidCount, missingCount}
			summary := fmt.Sprintf(
				"%d total skills (%d clean, %d locally modified, %d unmanaged, %d invalid, %d missing)",
				len(statuses), cleanCount, modCount, unmanagedCount, invalidCount, missingCount)

			if invalidCount > 0 || missingCount > 0 {
				addItem(CheckErr, "Skills Consistency", "doctor.check.consistency", nil, summary, "doctor.msg.consistency_summary", summaryArgs)
			} else if modCount > 0 || unmanagedCount > 0 {
				addItem(CheckWarn, "Skills Consistency", "doctor.check.consistency", nil, summary, "doctor.msg.consistency_summary", summaryArgs)
			} else {
				addItem(CheckOK, "Skills Consistency", "doctor.check.consistency", nil, summary, "doctor.msg.consistency_summary", summaryArgs)
			}
		}
	}

	// 6. Cache Directory
	cacheDir := s.cacheMgr.CacheDir()
	if fsx.DirExists(cacheDir) {
		entries, _ := s.cacheMgr.List()
		addItem(CheckOK, "Git Cache", "doctor.check.cache", nil,
			fmt.Sprintf("%d cached repositories in %s", len(entries), cacheDir),
			"doctor.msg.cache_repos", []any{len(entries), cacheDir})
	} else {
		addItem(CheckOK, "Git Cache", "doctor.check.cache", nil,
			"cache directory initialized", "doctor.msg.cache_initialized", nil)
	}

	// 7. Check Cached Projects
	if s.configMgr != nil {
		projs, _ := s.ProjectList()
		if len(projs) == 0 {
			if cfg, err := s.configMgr.Load(); err == nil && cfg != nil && len(cfg.Projects) > 0 {
				projs = cfg.Projects
			}
		}
		if len(projs) > 0 {
			var validProjects, missingProjects int
			for _, p := range projs {
				trimmed := strings.TrimSpace(p)
				if trimmed == "" {
					missingProjects++
					addItem(CheckWarn, "Cached Project", "doctor.check.cached_project", nil,
						"empty project path in cache", "doctor.msg.project_empty", nil)
					continue
				}
				expanded, err := fsx.ExpandUser(trimmed)
				if err != nil {
					missingProjects++
					base := filepath.Base(trimmed)
					addItem(CheckWarn, "Project "+base, "doctor.check.project_named", []any{base},
						fmt.Sprintf("invalid project path %s: %v", trimmed, err),
						"doctor.msg.project_invalid", []any{trimmed, err})
					continue
				}
				absPath, err := filepath.Abs(expanded)
				if err != nil {
					absPath = filepath.Clean(expanded)
				}
				baseName := filepath.Base(absPath)
				if baseName == "." || baseName == "/" || baseName == string(filepath.Separator) {
					baseName = fsx.CompactUser(absPath)
				}

				fi, err := os.Stat(absPath)
				if err != nil {
					missingProjects++
					compact := fsx.CompactUser(absPath)
					if os.IsNotExist(err) {
						addItem(CheckWarn, "Project "+baseName, "doctor.check.project_named", []any{baseName},
							fmt.Sprintf("project path %s does not exist on disk", compact),
							"doctor.msg.project_missing", []any{compact})
					} else {
						addItem(CheckWarn, "Project "+baseName, "doctor.check.project_named", []any{baseName},
							fmt.Sprintf("cannot access project path %s: %v", compact, err),
							"doctor.msg.project_inaccessible", []any{compact, err})
					}
					continue
				}
				if !fi.IsDir() {
					missingProjects++
					compact := fsx.CompactUser(absPath)
					addItem(CheckWarn, "Project "+baseName, "doctor.check.project_named", []any{baseName},
						fmt.Sprintf("project path %s is not a directory", compact),
						"doctor.msg.project_not_dir", []any{compact})
					continue
				}
				validProjects++
			}
			if missingProjects == 0 {
				if validProjects == 1 {
					compact := fsx.CompactUser(projs[0])
					addItem(CheckOK, "Cached Projects", "doctor.check.projects", nil,
						fmt.Sprintf("1 cached project valid and reachable (%s)", compact),
						"doctor.msg.projects_one", []any{compact})
				} else {
					addItem(CheckOK, "Cached Projects", "doctor.check.projects", nil,
						fmt.Sprintf("%d cached projects valid and reachable", validProjects),
						"doctor.msg.projects_many", []any{validProjects})
				}
			} else if validProjects > 0 {
				addItem(CheckOK, "Cached Projects", "doctor.check.projects", nil,
					fmt.Sprintf("%d of %d cached projects valid", validProjects, len(projs)),
					"doctor.msg.projects_partial", []any{validProjects, len(projs)})
			}
		} else {
			addItem(CheckOK, "Cached Projects", "doctor.check.projects", nil,
				"no cached projects", "doctor.msg.projects_none", nil)
		}
	}

	// 8. Check Deployments State
	st, err := s.stateMgr.Load()
	if err == nil && st != nil {
		var totalDeploys int
		for _, skMap := range st.Targets {
			totalDeploys += len(skMap)
		}
		addItem(CheckOK, "Deployments State", "doctor.check.deployments", nil,
			fmt.Sprintf("%d active deployment records", totalDeploys),
			"doctor.msg.deployments", []any{totalDeploys})
	}

	return report, nil
}

// CleanLeftovers removes temporary leftovers from the skills directory.
func (s *Service) CleanLeftovers() (int, error) {
	if s.workspaceRoot == "" {
		return 0, fmt.Errorf("no active workspace")
	}
	skillsDir := catalog.SkillsDir(s.workspaceRoot)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return 0, err
	}

	var cleaned int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".asoul-tmp-") || strings.HasPrefix(e.Name(), ".asoul-backup-") {
			fullPath := filepath.Join(skillsDir, e.Name())
			if err := os.RemoveAll(fullPath); err == nil {
				cleaned++
			}
		}
	}
	return cleaned, nil
}
