package cli

import (
	"bufio"
	"fmt"
	"strings"

	"asoul/internal/app"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <skill-id>",
		Short: "Create a new managed skill in the workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if err := appService.NewSkill(cmd.Context(), id); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status":  "created",
					"skillId": id,
				})
			}

			out.Successf(i18n.T("cli.manage.created"), id, id)
			return nil
		},
	}
}

func newAdoptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt <skill-id>",
		Short: "Adopt an unmanaged skill directory inside skills/ into catalog.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if err := appService.Adopt(cmd.Context(), id); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status":  "adopted",
					"skillId": id,
				})
			}

			out.Successf(i18n.T("cli.manage.adopted"), id)
			return nil
		},
	}
}

func newRemoveCmd() *cobra.Command {
	var (
		force      bool
		pattern    string
		regexAlias string
		source     string
		sourceType string
		dryRun     bool
		yes        bool
	)

	cmd := &cobra.Command{
		Use:     "remove [skill-id...]",
		Aliases: []string{"rm"},
		Short:   "Remove skills from catalog and managed store (supports batch, regex pattern, and source filters)",
		Long: `Remove managed skills from the workspace. Supports specifying multiple skill IDs,
filtering by regular expression (--pattern), source URL/path (--source), or source type (--source-type).
Includes safety warnings if any skills are actively deployed to agent or project targets.`,
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if pattern == "" && regexAlias != "" {
				pattern = regexAlias
			}

			if len(args) == 0 && pattern == "" && source == "" && sourceType == "" {
				return fmt.Errorf("please specify at least one skill ID or a filter flag (--pattern, --source, --source-type)")
			}

			// 1. Filter skills
			matchedSkills, err := appService.FilterSkills(cmd.Context(), app.SkillFilterOptions{
				IDs:        args,
				Pattern:    pattern,
				Source:     source,
				SourceType: sourceType,
			})
			if err != nil {
				return err
			}

			// If specific IDs were requested, check for IDs not found in workspace
			var notFound []string
			if len(args) > 0 && pattern == "" && source == "" && sourceType == "" {
				foundMap := make(map[string]bool)
				for _, sk := range matchedSkills {
					foundMap[sk.ID] = true
				}
				for _, reqID := range args {
					if !foundMap[reqID] {
						notFound = append(notFound, reqID)
					}
				}
			}

			if len(matchedSkills) == 0 && len(notFound) == 0 {
				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"matched": []string{},
						"removed": []string{},
					})
				}
				out.Infof(i18n.T("cli.manage.none_matched"))
				return nil
			}

			matchedIDs := make([]string, 0, len(matchedSkills))
			for _, sk := range matchedSkills {
				matchedIDs = append(matchedIDs, sk.ID)
			}

			// Check deployments
			allDeployments := appService.AllSkillDeployments()
			deployedMap := make(map[string][]string)
			for _, id := range matchedIDs {
				if deps := allDeployments[id]; len(deps) > 0 {
					deployedMap[id] = deps
				}
			}

			// 2. Handle dry-run
			if dryRun {
				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"dryRun":      true,
						"matched":     matchedIDs,
						"notFound":    notFound,
						"deployments": deployedMap,
					})
				}

				out.Infof(i18n.T("cli.manage.dry_run_head"), len(matchedIDs))
				for _, sk := range matchedSkills {
					depInfo := ""
					if deps, ok := deployedMap[sk.ID]; ok {
						depInfo = fmt.Sprintf(" [deployed: %s]", strings.Join(deps, ", "))
					}
					out.Printf("  • %-20s (%s)%s\n", sk.ID, i18n.T("source.type."+string(sk.Source.Type)), depInfo)
				}
				if len(notFound) > 0 {
					out.Warnf(i18n.T("cli.manage.not_found"), strings.Join(notFound, ", "))
				}
				out.Infof(i18n.T("cli.manage.dry_run_done"))
				return nil
			}

			// 3. Deployed target warnings
			if len(deployedMap) > 0 && !out.json {
				out.Warnf(i18n.T("cli.manage.deployed_warn"), len(deployedMap))
				for id, deps := range deployedMap {
					out.Printf("  • %s -> %s\n", id, strings.Join(deps, ", "))
				}
			}

			// 4. Confirmation prompt
			needConfirm := len(matchedIDs) > 1 || len(deployedMap) > 0 || pattern != "" || source != "" || sourceType != ""
			confirmed := out.yes || yes
			prompted := false

			if needConfirm && !confirmed {
				if !out.nonInteractive && out.IsTTY() {
					prompted = true
					fmt.Fprintf(cmd.OutOrStdout(), "Are you sure you want to remove %d skill(s)? [y/N]: ", len(matchedIDs))
					reader := bufio.NewReader(cmd.InOrStdin())
					ans, _ := reader.ReadString('\n')
					ans = strings.TrimSpace(strings.ToLower(ans))
					if ans == "y" || ans == "yes" {
						confirmed = true
					}
				} else if len(deployedMap) > 0 && !force {
					if out.json {
						if printErr := out.PrintJSON(map[string]interface{}{
							"error":       "skills are deployed to active targets",
							"deployments": deployedMap,
						}); printErr != nil {
							return printErr
						}
					}
					return fmt.Errorf("skill(s) are deployed to active targets; use --yes or --force to confirm deletion")
				}
			} else if !needConfirm {
				confirmed = true
			}

			if !confirmed {
				if prompted {
					if out.json {
						_ = out.PrintJSON(map[string]interface{}{
							"status":  "cancelled",
							"matched": matchedIDs,
						})
					} else {
						out.Infof(i18n.T("cli.manage.cancelled"))
					}
					return nil
				}
				if out.json {
					if printErr := out.PrintJSON(map[string]interface{}{
						"error":   "confirmation required",
						"matched": matchedIDs,
					}); printErr != nil {
						return printErr
					}
				}
				return fmt.Errorf("confirmation required; re-run with --yes to remove %d skill(s)", len(matchedIDs))
			}

			// 5. Execute batch removal
			res, err := appService.RemoveBatch(cmd.Context(), matchedIDs, force, out.ProgressFunc())
			out.ClearProgress()
			if err != nil {
				return out.EmitError("remove_failed", err)
			}
			for _, missing := range notFound {
				res.Failed[missing] = fmt.Sprintf("skill %q not found in catalog", missing)
			}

			if out.json {
				if printErr := out.PrintJSON(res); printErr != nil {
					return printErr
				}
				if len(res.Failed) > 0 {
					return fmt.Errorf("failed to remove %d skill(s)", len(res.Failed))
				}
				return nil
			}

			if len(args) == 1 && pattern == "" && source == "" && sourceType == "" && len(res.Removed) == 1 {
				out.Successf(i18n.T("cli.manage.removed_one"), res.Removed[0])
			} else if len(res.Removed) > 0 {
				out.Successf(i18n.T("cli.manage.removed_many"), len(res.Removed))
				for _, id := range res.Removed {
					out.Printf("  ✓ %s\n", id)
				}
			}

			if len(res.Failed) > 0 {
				for id, fErr := range res.Failed {
					out.Errorf(i18n.T("cli.manage.remove_failed"), id, fErr)
				}
				return fmt.Errorf("failed to remove %d skill(s)", len(res.Failed))
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force removal even if locally modified")
	cmd.Flags().StringVarP(&pattern, "pattern", "p", "", "Regular expression pattern to match skill IDs or names")
	cmd.Flags().StringVar(&regexAlias, "regex", "", "Alias for --pattern")
	_ = cmd.Flags().MarkHidden("regex")
	cmd.Flags().StringVar(&source, "source", "", "Filter skills by upstream source URL or path")
	cmd.Flags().StringVar(&sourceType, "source-type", "", "Filter skills by source type (git, local, managed)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview skills that would be removed without deleting")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Automatic yes to confirmation prompts")
	return cmd
}

func newRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old-id> <new-id>",
		Short: "Rename an existing managed skill and update its directory and metadata",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldID := args[0]
			newID := args[1]
			if err := appService.RenameSkill(cmd.Context(), oldID, newID); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status": "renamed",
					"oldId":  oldID,
					"newId":  newID,
				})
			}

			out.Successf(i18n.T("cli.manage.renamed"), oldID, newID)
			return nil
		},
	}
}
