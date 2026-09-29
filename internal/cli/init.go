package cli

import (
	"os"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Initialize a new asoul workspace with catalog.json and skills/",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			} else if flagRoot != "" {
				targetDir = flagRoot
			}

			abs, err := appService.InitWorkspace(cmd.Context(), targetDir)
			if err != nil {
				return err
			}

			// Register the newly initialized workspace in config
			_ = appService.AddWorkspace(abs)

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":    "initialized",
					"workspace": abs,
				})
			}

			out.Successf(i18n.T("cli.init.initialized"), abs)
			cwd, _ := os.Getwd()
			if abs != cwd {
				out.Printf(i18n.T("cli.init.next_steps"), abs, abs)
			}
			return nil
		},
	}
}
