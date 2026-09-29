package cli

import (
	"fmt"

	"asoul/internal/app"
	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var flagClean bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run diagnostics on workspace, skills, git, and configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if flagClean {
				cleaned, err := appService.CleanLeftovers()
				if err != nil {
					out.Errorf(i18n.T("doctor.cli.clean_failed"), err)
				} else if cleaned > 0 {
					out.Successf(i18n.T("notice.clean_leftovers"), cleaned)
				}
			}

			out.Progress(0, 0, i18n.T("progress.working"))
			report, err := appService.Doctor(ctx)
			out.ClearProgress()
			if err != nil {
				return err
			}

			if out.json {
				if err := out.PrintJSON(report); err != nil {
					return err
				}
				if report.HasError {
					return fmt.Errorf("%s", i18n.T("doctor.cli.error"))
				}
				return nil
			}

			out.Println(i18n.T("doctor.cli.title"))
			out.Println("--------------------------------")
			for _, item := range report.Items {
				var icon string
				switch item.Status {
				case app.CheckOK:
					icon = "✓"
				case app.CheckWarn:
					icon = "!"
				case app.CheckErr:
					icon = "✗"
				}
				out.Printf("%s %-20s: %s\n", icon, item.DisplayName(i18n.T), item.DisplayMessage(i18n.T))
			}
			out.Println("--------------------------------")

			if report.HasError {
				return fmt.Errorf("%s", i18n.T("doctor.cli.error"))
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&flagClean, "clean", false, "Clean temporary leftover staging directories")
	return cmd
}
