package cli

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"asoul/internal/model"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newUpstreamCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "upstream",
		Aliases: []string{"source", "sources"},
		Short:   "Manage upstream skill sources and repositories",
		Long: `Inspect and manage upstream sources (Git repositories and local directories) tracked by managed skills.
List active upstreams, check for available updates, discover skills in upstream repositories, and pull/re-pull skills into workspace.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpstreamList(cmd, args)
		},
	}

	cmd.AddCommand(newUpstreamListCmd())
	cmd.AddCommand(newUpstreamAddCmd())
	cmd.AddCommand(newUpstreamRemoveCmd())
	cmd.AddCommand(newUpstreamCheckCmd())
	cmd.AddCommand(newUpstreamSkillsCmd())
	cmd.AddCommand(newUpstreamPullCmd())

	return cmd
}

func newUpstreamListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all tracked upstream sources and their update status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpstreamList(cmd, args)
		},
	}
}

func runUpstreamList(cmd *cobra.Command, args []string) error {
	upstreams, err := appService.UpstreamList(cmd.Context())
	if err != nil {
		return err
	}

	if out.json {
		return out.PrintJSON(upstreams)
	}

	if len(upstreams) == 0 {
		out.Println(i18n.T("cli.upstream.none"))
		return nil
	}

	w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TYPE\tSTATUS\tSKILLS\tCOMMIT\tURL")
	for _, u := range upstreams {
		commit := u.Commit
		if len(commit) > 7 {
			commit = commit[:7]
		}
		if commit == "" {
			commit = "-"
		}
		skillsStr := fmt.Sprintf("%d skills", len(u.Skills))
		if len(u.Skills) > 0 {
			if len(u.Skills) <= 3 {
				skillsStr = fmt.Sprintf("%d (%s)", len(u.Skills), strings.Join(u.Skills, ", "))
			} else {
				skillsStr = fmt.Sprintf("%d (%s...)", len(u.Skills), strings.Join(u.Skills[:3], ", "))
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", u.Type, u.Status, skillsStr, commit, u.URL)
	}
	return w.Flush()
}

func newUpstreamCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check [upstream-url]",
		Short: "Fetch and check upstream repositories for updates",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var targetURL string
			if len(args) > 0 {
				targetURL = args[0]
			}

			if !out.json {
				if targetURL != "" {
					out.Printf(i18n.T("cli.upstream.checking_one"), targetURL)
				} else {
					out.Println(i18n.T("cli.upstream.checking_all"))
				}
			}

			upstreams, err := appService.UpstreamCheck(cmd.Context(), targetURL, out.ProgressFunc())
			out.ClearProgress()
			if err != nil {
				return err
			}

			if out.json {
				if err := out.PrintJSON(upstreams); err != nil {
					return err
				}
				return upstreamCheckFailures(upstreams)
			}

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "TYPE\tSTATUS\tSKILLS\tNEW COMMIT\tURL")
			for _, u := range upstreams {
				newCommit := u.NewCommit
				if len(newCommit) > 7 {
					newCommit = newCommit[:7]
				} else if u.NewHash != "" {
					if len(u.NewHash) > 10 {
						newCommit = u.NewHash[:10]
					} else {
						newCommit = u.NewHash
					}
				}
				if newCommit == "" {
					newCommit = "-"
				}
				skillsStr := fmt.Sprintf("%d skills", len(u.Skills))
				if len(u.Skills) > 0 {
					if len(u.Skills) <= 3 {
						skillsStr = fmt.Sprintf("%d (%s)", len(u.Skills), strings.Join(u.Skills, ", "))
					} else {
						skillsStr = fmt.Sprintf("%d (%s...)", len(u.Skills), strings.Join(u.Skills[:3], ", "))
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", u.Type, u.Status, skillsStr, newCommit, u.URL)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			for _, u := range upstreams {
				if u.Status == model.UpstreamUnreachable || u.Status == model.UpstreamInvalid {
					out.Errorf(i18n.T("cli.upstream.check_failed"), u.URL, u.Status)
				}
			}
			return upstreamCheckFailures(upstreams)
		},
	}
}

// upstreamCheckFailures returns a non-nil error when any upstream could not be
// checked, so callers exit non-zero instead of silently succeeding.
func upstreamCheckFailures(upstreams []model.UpstreamInfo) error {
	var errs []error
	for _, u := range upstreams {
		if u.Status == model.UpstreamUnreachable || u.Status == model.UpstreamInvalid || u.Error != "" {
			msg := u.Error
			if msg == "" {
				msg = string(u.Status)
			}
			errs = append(errs, fmt.Errorf("%s: %s", u.URL, msg))
		}
	}
	return errors.Join(errs...)
}

func newUpstreamSkillsCmd() *cobra.Command {
	var flagRef string

	cmd := &cobra.Command{
		Use:     "skills <upstream-url>",
		Aliases: []string{"show", "ls-skills"},
		Short:   "Discover and list all skills available in an upstream repository or directory",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			ctx := cmd.Context()

			onProgress := out.ProgressFunc()

			discovered, err := appService.UpstreamDiscover(ctx, url, flagRef, onProgress)
			if !out.json {
				out.ClearProgress()
			}
			if err != nil {
				return err
			}

			cat, catErr := appService.Status(ctx, false)
			installedSet := make(map[string]bool)
			if catErr == nil {
				for _, st := range cat {
					installedSet[st.ID] = true
				}
			}

			type skillItem struct {
				ID          string `json:"id"`
				Path        string `json:"path"`
				Description string `json:"description"`
				Installed   bool   `json:"installed"`
			}

			var items []skillItem
			for _, sk := range discovered {
				items = append(items, skillItem{
					ID:          sk.ID,
					Path:        sk.Path,
					Description: sk.Description,
					Installed:   installedSet[sk.ID],
				})
			}

			if out.json {
				return out.PrintJSON(items)
			}

			if len(items) == 0 {
				out.Printf(i18n.T("cli.upstream.no_skills"), url)
				return nil
			}

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "SKILL ID\tSTATUS\tPATH\tDESCRIPTION")
			for _, it := range items {
				st := "new"
				if it.Installed {
					st = "installed"
				}
				desc := it.Description
				if len(desc) > 40 {
					desc = desc[:37] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", it.ID, st, it.Path, desc)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&flagRef, "ref", "", "Git branch, tag or commit ref to inspect")
	return cmd
}

func newUpstreamPullCmd() *cobra.Command {
	var (
		flagRef   string
		flagAll   bool
		flagForce bool
	)

	cmd := &cobra.Command{
		Use:   "pull <upstream-url> [skill-id...]",
		Short: "Pull or update skills from an upstream source into the workspace",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			requestedSkills := args[1:]
			ctx := cmd.Context()

			onProgress := out.ProgressFunc()

			if !out.json {
				out.Printf(i18n.T("cli.upstream.discovering"), url)
			}
			discovered, err := appService.UpstreamDiscover(ctx, url, flagRef, onProgress)
			if !out.json {
				out.ClearProgress()
			}
			if err != nil {
				return err
			}

			if len(discovered) == 0 {
				return fmt.Errorf("no skills found in upstream %s", url)
			}

			skillMap := make(map[string]string)
			for _, sk := range discovered {
				skillMap[sk.ID] = sk.Path
			}

			var toPullPaths []string
			if flagAll || len(requestedSkills) == 0 {
				for _, sk := range discovered {
					toPullPaths = append(toPullPaths, sk.Path)
				}
			} else {
				for _, id := range requestedSkills {
					if path, ok := skillMap[id]; ok {
						toPullPaths = append(toPullPaths, path)
					} else {
						return fmt.Errorf("skill %q not found in upstream repository %s", id, url)
					}
				}
			}

			res, err := appService.UpstreamPull(ctx, url, flagRef, toPullPaths, nil, nil, flagForce, onProgress)
			if !out.json {
				out.ClearProgress()
			}
			if err != nil {
				return err
			}

			if out.json {
				if printErr := out.PrintJSON(res); printErr != nil {
					return printErr
				}
				if len(res.Failed) > 0 {
					return fmt.Errorf("failed to pull %d skill(s)", len(res.Failed))
				}
				return nil
			}

			if len(res.Added) > 0 {
				out.Successf(i18n.T("cli.upstream.pulled"), len(res.Added), url)
				for _, id := range res.Added {
					out.Printf("  ✓ %s\n", id)
				}
			}
			if len(res.Skipped) > 0 {
				out.Printf(i18n.T("cli.upstream.skipped"), len(res.Skipped))
			}
			if len(res.Failed) > 0 {
				for id, fErr := range res.Failed {
					out.Errorf(i18n.T("cli.upstream.item_failed"), id, fErr)
				}
				return fmt.Errorf("failed to pull %d skill(s)", len(res.Failed))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&flagRef, "ref", "", "Git branch, tag or commit ref")
	cmd.Flags().BoolVar(&flagAll, "all", false, "Pull all skills available in the upstream repository")
	cmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force pull and overwrite local modifications")
	return cmd
}

func newUpstreamAddCmd() *cobra.Command {
	var (
		flagRef  string
		flagName string
	)
	cmd := &cobra.Command{
		Use:   "add <url-or-path>",
		Short: "Add or register a new upstream repository or directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			if err := appService.UpstreamAdd(cmd.Context(), url, flagRef, flagName); err != nil {
				return err
			}
			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status": "added",
					"url":    url,
					"ref":    flagRef,
					"name":   flagName,
				})
			}
			out.Successf(i18n.T("cli.upstream.added"), url)
			return nil
		},
	}
	cmd.Flags().StringVar(&flagRef, "ref", "", "Git branch, tag or commit to track")
	cmd.Flags().StringVar(&flagName, "name", "", "Optional alias / display name for the upstream source")
	return cmd
}

func newUpstreamRemoveCmd() *cobra.Command {
	var flagRemoveSkills bool
	cmd := &cobra.Command{
		Use:     "remove <url-or-path>",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove an upstream source from configuration",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			if err := appService.UpstreamRemove(cmd.Context(), url, flagRemoveSkills); err != nil {
				return err
			}
			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":       "removed",
					"url":          url,
					"removeSkills": flagRemoveSkills,
				})
			}
			out.Successf(i18n.T("cli.upstream.removed"), url)
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagRemoveSkills, "remove-skills", false, "Also remove all skills downloaded from this upstream")
	return cmd
}
