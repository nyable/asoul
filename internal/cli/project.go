package cli

import (
	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage cached project paths and view their deployed skills",
	}

	cmd.AddCommand(newProjectListCmd())
	cmd.AddCommand(newProjectEnableCmd())
	cmd.AddCommand(newProjectDisableCmd())
	cmd.AddCommand(newProjectRemoveCmd())

	return cmd
}

func newProjectListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cached project paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			projects, err := appService.ProjectList()
			if err != nil {
				return err
			}

			if out.json {
				type projectInfo struct {
					Path    string   `json:"path"`
					Enabled bool     `json:"enabled"`
					Skills  []string `json:"skills,omitempty"`
				}
				var list []projectInfo
				for _, p := range projects {
					info := projectInfo{
						Path:    p,
						Enabled: appService.IsProjectEnabled(p),
					}
					if st, err := appService.ProjectStatus(p); err == nil && st != nil {
						for sID := range st.Skills {
							info.Skills = append(info.Skills, sID)
						}
					}
					list = append(list, info)
				}
				return out.PrintJSON(list)
			}

			if len(projects) == 0 {
				out.Infof(i18n.T("cli.project.none"))
				return nil
			}

			out.Println()
			out.Printf(i18n.T("cli.project.title"), len(projects))
			for i, p := range projects {
				st, err := appService.ProjectStatus(p)
				skillCount := 0
				if err == nil && st != nil {
					skillCount = len(st.Skills)
				}
				enStatus := "enabled"
				if !appService.IsProjectEnabled(p) {
					enStatus = "disabled"
				}
				out.Printf(i18n.T("cli.project.item"), i+1, enStatus, p, skillCount)
			}
			out.Println()
			return nil
		},
	}
	return cmd
}

func newProjectEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <project-path>",
		Short: "Enable a project directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := args[0]
			if err := appService.SetProjectEnabled(p, true); err != nil {
				return err
			}
			if out.json {
				return out.PrintJSON(map[string]interface{}{"project": p, "enabled": true})
			}
			out.Successf(i18n.T("cli.project.enabled"), p)
			return nil
		},
	}
}

func newProjectDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <project-path>",
		Short: "Disable a project directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := args[0]
			if err := appService.SetProjectEnabled(p, false); err != nil {
				return err
			}
			if out.json {
				return out.PrintJSON(map[string]interface{}{"project": p, "enabled": false})
			}
			out.Successf(i18n.T("cli.project.disabled"), p)
			return nil
		},
	}
}

func newProjectRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <project-path>",
		Short: "Remove a project path from the cache",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := args[0]
			if err := appService.ProjectRemove(p); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":  "removed",
					"project": p,
				})
			}

			out.Successf(i18n.T("cli.project.removed"), p)
			return nil
		},
	}
	return cmd
}
