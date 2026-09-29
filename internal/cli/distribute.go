package cli

import (
	"fmt"
	"strings"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newDeployCmd() *cobra.Command {
	var (
		flagGlobal  string
		flagProject string
		flagFormats []string
		flagForce   bool
		flagTo      string // legacy fallback
	)

	cmd := &cobra.Command{
		Use:   "deploy <skill-id> (--global <program> | --project <path> [--format <format>])",
		Short: "Deploy a managed skill to a global program or project directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			// If legacy --to was provided and neither --global nor --project was given
			if flagTo != "" && flagGlobal == "" && flagProject == "" {
				flagGlobal = flagTo
			}

			if flagGlobal == "" && flagProject == "" {
				return out.EmitError("invalid_arguments", fmt.Errorf("must specify either --global <program> or --project <path>"))
			}

			if flagGlobal != "" && flagProject != "" {
				return out.EmitError("invalid_arguments", fmt.Errorf("--global and --project are mutually exclusive"))
			}

			if flagGlobal != "" {
				if cmd.Flags().Changed("format") {
					return out.EmitError("invalid_arguments", fmt.Errorf("--format is only valid when using --project"))
				}

				out.Progress(0, 0, id)
				err := appService.DeployGlobal(cmd.Context(), id, flagGlobal, flagForce)
				out.ClearProgress()
				if err != nil {
					return out.EmitError("deploy_failed", err)
				}

				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"status":  "deployed",
						"skillId": id,
						"scope":   "global",
						"program": flagGlobal,
					})
				}

				out.Successf(i18n.T("cli.distribute.deployed_global"), id, flagGlobal)
				return nil
			}

			// Project deployment
			if len(flagFormats) == 0 {
				flagFormats = []string{"standard"}
			}

			out.Progress(0, 0, id)
			deployedFormats, fallback, err := appService.DeployProject(cmd.Context(), id, flagProject, flagFormats, flagForce)
			out.ClearProgress()
			if err != nil {
				return out.EmitError("deploy_failed", err)
			}

			if fallback {
				out.Infof(i18n.T("cli.distribute.no_agent_dirs"), flagProject)
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":   "deployed",
					"skillId":  id,
					"scope":    "project",
					"project":  flagProject,
					"formats":  deployedFormats,
					"fallback": fallback,
				})
			}

			out.Successf(i18n.T("cli.distribute.deployed_project"), id, flagProject, strings.Join(deployedFormats, ", "))
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagGlobal, "global", "g", "", "Target global program (e.g. standard, codex, claude, cursor, etc.)")
	cmd.Flags().StringVarP(&flagProject, "project", "p", "", "Target project directory path (e.g. . or /path/to/project)")
	cmd.Flags().StringSliceVarP(&flagFormats, "format", "m", []string{"standard"}, "Distribution format(s) for project (standard, claude, cline, auto, etc.)")
	cmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force deployment even if target was modified")
	cmd.Flags().StringVar(&flagTo, "to", "", "Legacy alias for --global")
	_ = cmd.Flags().MarkHidden("to")

	return cmd
}

func newUndeployCmd() *cobra.Command {
	var (
		flagGlobal  string
		flagProject string
		flagFormats []string
		flagForce   bool
		flagFrom    string // legacy fallback
	)

	cmd := &cobra.Command{
		Use:   "undeploy <skill-id> (--global <program> | --project <path> [--format <format>])",
		Short: "Undeploy a skill from a global program or project directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			if flagFrom != "" && flagGlobal == "" && flagProject == "" {
				flagGlobal = flagFrom
			}

			if flagGlobal == "" && flagProject == "" {
				return out.EmitError("invalid_arguments", fmt.Errorf("must specify either --global <program> or --project <path>"))
			}

			if flagGlobal != "" && flagProject != "" {
				return out.EmitError("invalid_arguments", fmt.Errorf("--global and --project are mutually exclusive"))
			}

			if flagGlobal != "" {
				if cmd.Flags().Changed("format") {
					return out.EmitError("invalid_arguments", fmt.Errorf("--format is only valid when using --project"))
				}

				out.Progress(0, 0, id)
				err := appService.UndeployGlobal(cmd.Context(), id, flagGlobal, flagForce)
				out.ClearProgress()
				if err != nil {
					return out.EmitError("undeploy_failed", err)
				}

				if out.json {
					return out.PrintJSON(map[string]interface{}{
						"status":  "undeployed",
						"skillId": id,
						"scope":   "global",
						"program": flagGlobal,
					})
				}

				out.Successf(i18n.T("cli.distribute.undeployed_global"), id, flagGlobal)
				return nil
			}

			// Project undeploy
			var formats []string
			if cmd.Flags().Changed("format") {
				formats = flagFormats
			}

			out.Progress(0, 0, id)
			undeployedFormats, err := appService.UndeployProject(cmd.Context(), id, flagProject, formats, flagForce)
			out.ClearProgress()
			if err != nil {
				return out.EmitError("undeploy_failed", err)
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":  "undeployed",
					"skillId": id,
					"scope":   "project",
					"project": flagProject,
					"formats": undeployedFormats,
				})
			}

			out.Successf(i18n.T("cli.distribute.undeployed_project"), id, flagProject, strings.Join(undeployedFormats, ", "))
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagGlobal, "global", "g", "", "Target global program (e.g. standard, codex, claude, cursor, etc.)")
	cmd.Flags().StringVarP(&flagProject, "project", "p", "", "Target project directory path (e.g. . or /path/to/project)")
	cmd.Flags().StringSliceVarP(&flagFormats, "format", "m", nil, "Distribution format(s) to undeploy (omit to undeploy all formats)")
	cmd.Flags().BoolVarP(&flagForce, "force", "f", false, "Force undeployment even if target was modified")
	cmd.Flags().StringVar(&flagFrom, "from", "", "Legacy alias for --global")
	_ = cmd.Flags().MarkHidden("from")

	return cmd
}
