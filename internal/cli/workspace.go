package cli

import (
	"fmt"
	"text/tabwriter"

	"asoul/internal/config"
	"asoul/internal/fsx"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newWorkspaceCmd() *cobra.Command {
	wsCmd := &cobra.Command{
		Use:     "workspace",
		Aliases: []string{"ws"},
		Short:   "Manage registered skill workspaces",
	}

	wsCmd.AddCommand(newWorkspaceListCmd())
	wsCmd.AddCommand(newWorkspaceAddCmd())
	wsCmd.AddCommand(newWorkspaceRemoveCmd())
	wsCmd.AddCommand(newWorkspaceInitCmd())
	wsCmd.AddCommand(newWorkspaceSwitchCmd())

	return wsCmd
}

func newWorkspaceListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all registered workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			workspaces := appService.Workspaces()
			activeRoot := appService.WorkspaceRoot()

			if out.json {
				type wsInfo struct {
					Path        string `json:"path"`
					Active      bool   `json:"active"`
					Initialized bool   `json:"initialized"`
				}
				var list []wsInfo
				for _, ws := range workspaces {
					list = append(list, wsInfo{
						Path:        ws,
						Active:      fsx.SamePath(ws, activeRoot),
						Initialized: appService.WorkspacePathInitialized(ws),
					})
				}
				return out.PrintJSON(list)
			}

			if len(workspaces) == 0 {
				out.Infof(i18n.T("cli.workspace.none"))
				return nil
			}

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "  \tPATH\tSTATUS")
			for _, ws := range workspaces {
				marker := " "
				if fsx.SamePath(ws, activeRoot) {
					marker = "*"
				}
				status := "not initialized"
				if appService.WorkspacePathInitialized(ws) {
					status = "ready"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", marker, ws, status)
			}
			return w.Flush()
		},
	}
}

func newWorkspaceAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <path>",
		Short: "Register a new workspace directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if err := appService.AddWorkspace(path); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status": "added",
					"path":   path,
				})
			}

			out.Successf(i18n.T("cli.workspace.registered"), path)
			if !appService.WorkspacePathInitialized(path) {
				out.Infof(i18n.T("cli.workspace.not_initialized"), path)
			}
			return nil
		},
	}
}

func newWorkspaceRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "remove <path>",
		Aliases: []string{"rm"},
		Short:   "Unregister a workspace directory",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if err := appService.RemoveWorkspace(path); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status": "removed",
					"path":   path,
				})
			}

			out.Successf(i18n.T("cli.workspace.unregistered"), path)
			return nil
		},
	}
}

func newWorkspaceInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a workspace directory with catalog.json and skills/",
		Long:  "Initialize a workspace directory. If no path is given, initializes the configured default workspace.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var targetDir string
			if len(args) > 0 {
				targetDir = args[0]
			} else if appService.WorkspaceRoot() != "" {
				targetDir = appService.WorkspaceRoot()
			} else {
				// Use first configured workspace or default
				workspaces := appService.Workspaces()
				if len(workspaces) > 0 {
					targetDir = workspaces[0]
				} else {
					targetDir = config.DefaultWorkspacePathCompact()
				}
			}

			abs, err := appService.InitWorkspace(cmd.Context(), targetDir)
			if err != nil {
				return err
			}

			// Also register the workspace in config
			_ = appService.AddWorkspace(abs)

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":    "initialized",
					"workspace": abs,
				})
			}

			out.Successf(i18n.T("cli.workspace.initialized"), abs)
			return nil
		},
	}
}

func newWorkspaceSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <path>",
		Short: "Switch the active workspace to a registered path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if err := appService.SelectWorkspace(path); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status": "switched",
					"path":   path,
				})
			}

			out.Successf(i18n.T("cli.workspace.switched"), path)
			return nil
		},
	}
}
