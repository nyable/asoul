package cli

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"asoul/internal/diffview"
	"asoul/internal/i18n"
	"asoul/internal/model"

	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check [skill-id]",
		Short: "Check upstream sources for available updates",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ids []string
			if len(args) > 0 {
				ids = append(ids, args[0])
			}

			statuses, err := appService.Check(cmd.Context(), ids, out.ProgressFunc())
			out.ClearProgress()
			if err != nil {
				return err
			}

			if out.json {
				if err := out.PrintJSON(statuses); err != nil {
					return err
				}
				return checkFailures(statuses)
			}

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tSOURCE\tMANAGED\tUPSTREAM\tDETAILS")
			for _, s := range statuses {
				details := s.Error
				if details == "" {
					if s.NewCommit != "" {
						details = s.NewCommit[:7]
					} else if s.NewHash != "" {
						details = s.NewHash[:10]
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Source.Type, s.ManagedStatus, s.Upstream, details)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			for _, s := range statuses {
				if s.Error != "" {
					out.Errorf(i18n.T("cli.sync.check_failed"), s.ID, s.Error)
				}
			}
			return checkFailures(statuses)
		},
	}
}

// checkFailures returns a non-nil error when any skill check reported a
// per-item failure, so callers exit non-zero instead of silently succeeding.
func checkFailures(statuses []model.SkillStatus) error {
	var errs []error
	for _, s := range statuses {
		switch {
		case s.Error != "":
			errs = append(errs, fmt.Errorf("%s: %s", s.ID, s.Error))
		case s.Upstream == model.UpstreamUnreachable, s.Upstream == model.UpstreamInvalid:
			errs = append(errs, fmt.Errorf("%s: upstream %s", s.ID, s.Upstream))
		}
	}
	return errors.Join(errs...)
}

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <skill-id>",
		Short: "Show differences between current managed skill and upstream candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			diffText, err := appService.Diff(cmd.Context(), id)
			if err != nil {
				return err
			}

			if out.json {
				summary := diffview.Parse(diffText)
				return out.PrintJSON(map[string]interface{}{
					"skillId":   id,
					"diff":      diffText,
					"files":     summary.TotalFiles,
					"additions": summary.Additions,
					"deletions": summary.Deletions,
				})
			}

			if diffText == "" {
				out.Println(i18n.T("diff.no_differences"))
				return nil
			}

			out.PrintDiff(diffText)
			return nil
		},
	}
}

func newUpdateCmd() *cobra.Command {
	var (
		flagForce         bool
		flagReplaceSource bool
	)

	cmd := &cobra.Command{
		Use:   "update [skill-id]",
		Short: "Update managed skills to their latest upstream version",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if len(args) > 0 {
				// Update single skill
				id := args[0]
				out.Progress(0, 0, id)
				stat, err := appService.Update(ctx, id, flagForce, flagReplaceSource, out.ProgressFunc())
				out.ClearProgress()
				if err != nil {
					return err
				}

				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"status":  "updated",
						"skillId": id,
						"details": stat,
					})
				}

				out.Successf(i18n.T("cli.sync.updated_skill"), id)
				return nil
			}

			// Update all skills that have updates
			out.Println(i18n.T("cli.sync.checking_all"))
			statuses, err := appService.Check(ctx, nil, out.ProgressFunc())
			out.ClearProgress()
			if err != nil {
				return err
			}

			var pending []string
			for _, st := range statuses {
				if st.Upstream == model.UpstreamUpdateAvailable || st.Upstream == model.UpstreamSourceChanged {
					pending = append(pending, st.ID)
				}
			}

			var updated []string
			var failed []string
			var updateErrors []error
			for i, id := range pending {
				out.Progress(i+1, len(pending), id)
				_, err := appService.Update(ctx, id, flagForce, flagReplaceSource)
				out.ClearProgress()
				if err != nil {
					out.Errorf(i18n.T("cli.sync.update_failed"), id, err)
					failed = append(failed, id)
					updateErrors = append(updateErrors, fmt.Errorf("%s: %w", id, err))
				} else {
					updated = append(updated, id)
					out.Successf(i18n.T("cli.sync.updated"), id)
				}
			}

			if out.json {
				status := "updated"
				if len(updated) == 0 && len(failed) == 0 {
					status = "up-to-date"
				} else if len(failed) > 0 {
					status = "partial"
				}
				if err := out.PrintJSON(map[string]interface{}{
					"status":  status,
					"updated": updated,
					"failed":  failed,
				}); err != nil {
					return err
				}
			} else if len(updated) == 0 && len(failed) == 0 {
				out.Println(i18n.T("cli.sync.all_up_to_date"))
			}
			return errors.Join(updateErrors...)
		},
	}

	cmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force update even if local modifications exist")
	cmd.Flags().BoolVar(&flagReplaceSource, "replace-source", false, "Replace source configuration if changed")

	return cmd
}
