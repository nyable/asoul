package cli

import (
	"fmt"
	"strings"

	"asoul/internal/fsx"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	var (
		flagPath          string
		flagRef           string
		flagAs            string
		flagAll           bool
		flagReplaceSource bool
	)

	cmd := &cobra.Command{
		Use:   "add <source-path-or-url>",
		Short: "Add a skill to the workspace from a local directory or Git repository",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			ctx := cmd.Context()

			isGit := strings.HasPrefix(src, "http://") ||
				strings.HasPrefix(src, "https://") ||
				strings.HasPrefix(src, "git@") ||
				strings.HasPrefix(src, "ssh://") ||
				strings.HasSuffix(src, ".git")

			if !isGit {
				// Check if it exists locally
				expanded, _ := fsx.ExpandUser(src)
				if !fsx.DirExists(expanded) && (strings.Contains(src, "github.com/") || strings.Contains(src, "gitlab.com/") || strings.Contains(src, "gitee.com/")) {
					isGit = true
				}
			}

			if !isGit {
				// Local directory source
				if err := appService.AddLocal(ctx, src, flagReplaceSource); err != nil {
					return out.EmitError("add_failed", err)
				}

				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"status": "added",
						"type":   "local",
						"source": src,
					})
				}

				out.Successf(i18n.T("cli.add.added_local"), src)
				return nil
			}

			// Git repository source
			onProgress := out.ProgressFunc()

			if flagAll && flagAs != "" {
				return fmt.Errorf("--as cannot be used with --all")
			}

			if flagPath != "" {
				// Explicit subpath specified
				if err := appService.AddGitCustom(ctx, src, flagRef, flagPath, flagAs, flagReplaceSource, false, onProgress); err != nil {
					if !out.json {
						out.ClearProgress()
					}
					return out.EmitError("add_failed", err)
				}
				if !out.json {
					out.ClearProgress()
				}

				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"status":  "added",
						"type":    "git",
						"url":     src,
						"path":    flagPath,
						"ref":     flagRef,
						"skillId": flagAs,
					})
				}

				if flagAs != "" {
					out.Successf(i18n.T("cli.add.added_git_as"), src, flagPath, flagAs)
				} else {
					out.Successf(i18n.T("cli.add.added_git"), src, flagPath)
				}
				return nil
			}

			// Path not specified: discover skills in repo
			out.Printf(i18n.T("cli.add.discovering"), src)
			discovered, err := appService.DiscoverGit(ctx, src, flagRef, onProgress)
			if !out.json {
				out.ClearProgress()
			}
			if err != nil {
				return out.EmitError("add_failed", err)
			}

			if len(discovered) == 0 {
				return out.EmitError("add_failed", fmt.Errorf("no valid skills found in repository %s", src))
			}

			if len(discovered) == 1 {
				// Exactly one skill found, add it directly (fetch=false since DiscoverGit just fetched)
				sk := discovered[0]
				var aliases map[string]string
				if flagAs != "" {
					aliases = map[string]string{sk.Path: flagAs}
				}
				res, err := appService.AddGitBatchWithOptions(ctx, src, flagRef, []string{sk.Path}, aliases, nil, flagReplaceSource, false, false, onProgress)
				if !out.json {
					out.ClearProgress()
				}
				if err != nil {
					return out.EmitError("add_failed", err)
				}
				if len(res.Failed) > 0 {
					reasons := make([]string, 0, len(res.Failed))
					for _, fErr := range res.Failed {
						reasons = append(reasons, fErr)
					}
					if out.json {
						if printErr := out.PrintJSON(map[string]interface{}{
							"status": "failed",
							"type":   "git",
							"url":    src,
							"failed": reasons,
						}); printErr != nil {
							return printErr
						}
					}
					return fmt.Errorf("failed to add skill: %s", strings.Join(reasons, ", "))
				}

				skillID := sk.ID
				if flagAs != "" {
					skillID = flagAs
				}

				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"status":  "added",
						"type":    "git",
						"url":     src,
						"skillId": skillID,
						"path":    sk.Path,
					})
				}

				out.Successf(i18n.T("cli.add.added_skill_from"), skillID, src)
				return nil
			}

			// Multiple skills discovered
			if flagAll {
				var subpaths []string
				for _, sk := range discovered {
					subpaths = append(subpaths, sk.Path)
				}
				res, err := appService.AddGitBatch(ctx, src, flagRef, subpaths, flagReplaceSource, false, false, onProgress)
				if !out.json {
					out.ClearProgress()
				}
				if err != nil {
					return out.EmitError("add_failed", err)
				}

				for _, id := range res.Added {
					out.Successf(i18n.T("cli.add.added_skill"), id)
				}
				var failed []string
				for id, reason := range res.Failed {
					out.Warnf(i18n.T("cli.add.failed"), id, reason)
					failed = append(failed, fmt.Sprintf("%s: %s", id, reason))
				}

				if out.json {
					if err := out.PrintJSON(map[string]interface{}{
						"status": "added",
						"type":   "git",
						"url":    src,
						"added":  res.Added,
						"failed": failed,
					}); err != nil {
						return err
					}
				}
				if len(failed) > 0 {
					return fmt.Errorf("failed to add %d skill(s): %s", len(failed), strings.Join(failed, ", "))
				}
				return nil
			}

			// If multiple skills found and --all not specified
			out.Println(fmt.Sprintf("Discovered %d skills in repository:", len(discovered)))
			for _, sk := range discovered {
				out.Printf(i18n.T("cli.add.item"), sk.ID, sk.Path)
			}

			return out.EmitError("ambiguous_source", fmt.Errorf("multiple skills found in %s; re-run with --path <path> or --all to choose which skill to add", src))
		},
	}

	cmd.Flags().StringVar(&flagPath, "path", "", "Path to skill subdirectory within repository")
	cmd.Flags().StringVar(&flagRef, "ref", "", "Git ref (branch, tag, or commit) to checkout")
	cmd.Flags().StringVar(&flagAs, "as", "", "Custom ID to rename the skill during import")
	cmd.Flags().BoolVar(&flagAll, "all", false, "Add all skills discovered in the repository")
	cmd.Flags().BoolVar(&flagReplaceSource, "replace-source", false, "Replace existing skill source if conflict occurs")

	return cmd
}
