package cli

import (
	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <directory>",
		Short: "Validate that a directory conforms to the Agent Skill format",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dirPath := args[0]
			meta, err := appService.Validate(cmd.Context(), dirPath)
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"valid":       true,
					"name":        meta.Name,
					"description": meta.Description,
					"path":        meta.Path,
				})
			}

			out.Successf(i18n.T("cli.validate.valid"), meta.Name, meta.Description)
			return nil
		},
	}
}
