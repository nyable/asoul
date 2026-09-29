package cli

import (
	"fmt"
	"text/tabwriter"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var refresh bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Display the status of managed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			statuses, err := appService.Status(cmd.Context(), refresh)
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(statuses)
			}

			if len(statuses) == 0 {
				out.Println(i18n.T("cli.status.none"))
				return nil
			}

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			if refresh {
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
			} else {
				fmt.Fprintln(w, "NAME\tSOURCE\tMANAGED\tDETAILS")
				for _, s := range statuses {
					details := s.Error
					if details == "" && s.CurrentHash != "" {
						details = s.CurrentHash[:10]
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.Source.Type, s.ManagedStatus, details)
				}
			}
			return w.Flush()
		},
	}

	cmd.Flags().BoolVar(&refresh, "refresh", false, "Fetch upstream repositories and check for updates")
	return cmd
}
