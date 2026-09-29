package cli

import (
	"fmt"
	"text/tabwriter"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all managed skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			skills, err := appService.List(cmd.Context())
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(skills)
			}

			if len(skills) == 0 {
				out.Println(i18n.T("cli.list.none"))
				return nil
			}

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "ID\tSOURCE\tSTATUS\tPATH/URL")
			for _, s := range skills {
				sourceLoc := s.Source.Path
				if s.Source.URL != "" {
					sourceLoc = s.Source.URL
					if s.Source.Path != "" {
						sourceLoc += "#" + s.Source.Path
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.Source.Type, s.ManagedStatus, sourceLoc)
			}
			return w.Flush()
		},
	}
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <skill-id>",
		Short: "Display detailed information and SKILL.md for a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			status, body, err := appService.Show(cmd.Context(), id)
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":  status,
					"skillMD": body,
				})
			}

			out.Printf(i18n.T("cli.list.skill_id"), status.ID)
			out.Printf(i18n.T("cli.list.source_type"), i18n.T("source.type."+string(status.Source.Type)))
			if status.Source.URL != "" {
				out.Printf(i18n.T("cli.list.git_url"), status.Source.URL)
			}
			if status.Source.Ref != "" {
				out.Printf(i18n.T("cli.list.git_ref"), status.Source.Ref)
			}
			if status.Source.Path != "" {
				out.Printf(i18n.T("cli.list.source_path"), status.Source.Path)
			}
			out.Printf(i18n.T("cli.list.managed_status"), i18n.T("status.managed."+string(status.ManagedStatus)))
			out.Printf(i18n.T("cli.list.content_hash"), status.CurrentHash)

			if body != "" {
				out.Println(i18n.T("cli.list.skill_md"))
				out.Println(body)
			}
			return nil
		},
	}
}
