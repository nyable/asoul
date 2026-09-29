package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"asoul/internal/model"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newTargetCmd() *cobra.Command {
	targetCmd := &cobra.Command{
		Use:   "target",
		Short: "Manage deployment targets configured in user config",
	}

	var (
		flagType    string
		flagChannel string
		flagSkills  string
	)

	addCmd := &cobra.Command{
		Use:   "add <name> <config-dir>",
		Short: "Add or update a deployment target",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, inputPath := args[0], args[1]
			configDir := strings.TrimSuffix(strings.TrimSuffix(inputPath, "/"), "\\")
			if strings.HasSuffix(configDir, "/skills") {
				configDir = strings.TrimSuffix(configDir, "/skills")
			} else if strings.HasSuffix(configDir, "\\skills") {
				configDir = strings.TrimSuffix(configDir, "\\skills")
			}
			tType := model.TargetTypeAgent
			if flagType == "project" {
				tType = model.TargetTypeProject
			}

			channel := flagChannel
			if channel == "" && tType == model.TargetTypeAgent {
				channel = name
			}

			skillsPath := flagSkills
			if skillsPath == "" {
				if tType == model.TargetTypeProject {
					skillsPath = "${config_dir}/.agents/skills"
				} else {
					skillsPath = "${config_dir}/skills"
				}
			}

			tgt := model.TargetConfig{
				Type:      tType,
				Channel:   channel,
				ConfigDir: configDir,
				Paths: map[string]string{
					"skills": skillsPath,
				},
			}
			if channel == "opencode" {
				tgt.Paths["rules"] = "${config_dir}/rules"
			}

			if err := appService.TargetAddConfig(name, tgt); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":    "added",
					"name":      name,
					"target":    tgt,
					"skillsDir": tgt.SkillsDir(),
				})
			}

			out.Successf(i18n.T("cli.target.added"),
				name, tgt.Type, tgt.Channel, tgt.ConfigDir, tgt.SkillsDir())
			return nil
		},
	}
	addCmd.Flags().StringVar(&flagType, "type", "agent", "Target type: 'agent' (AI tool channel) or 'project' (codebase repo)")
	addCmd.Flags().StringVar(&flagChannel, "channel", "", "Agent channel identifier (e.g. opencode, claude, codex)")
	addCmd.Flags().StringVar(&flagSkills, "skills", "", "Skills destination path (supports ${config_dir} placeholder)")

	targetCmd.AddCommand(addCmd)

	targetCmd.AddCommand(&cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all configured targets",
		RunE: func(cmd *cobra.Command, args []string) error {
			targets, err := appService.TargetList()
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(targets)
			}

			if len(targets) == 0 {
				out.Println(i18n.T("cli.target.none"))
				return nil
			}

			var names []string
			for name := range targets {
				names = append(names, name)
			}
			sort.Strings(names)

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTATUS\tTYPE\tCONFIG_DIR\tFEATURES\tSKILLS")
			for _, name := range names {
				t := targets[name]
				tType := string(t.Type)
				if tType == "" {
					tType = "agent"
				}
				status := "enabled"
				if !t.IsEnabled() {
					status = "disabled"
				}
				deployed := appService.TargetDeployedSkills(name)
				features := t.FeaturesString()
				if features == "" {
					features = "skills"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\n", name, status, tType, t.ConfigDir, features, len(deployed))
			}
			return w.Flush()
		},
	})

	targetCmd.AddCommand(&cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a deployment target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := appService.SetTargetEnabled(name, true); err != nil {
				return err
			}
			if out.json {
				return out.PrintJSON(map[string]interface{}{"name": name, "enabled": true})
			}
			out.Successf(i18n.T("cli.target.enabled"), name)
			return nil
		},
	})

	targetCmd.AddCommand(&cobra.Command{
		Use:   "disable <name>",
		Short: "Disable a deployment target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := appService.SetTargetEnabled(name, false); err != nil {
				return err
			}
			if out.json {
				return out.PrintJSON(map[string]interface{}{"name": name, "enabled": false})
			}
			out.Successf(i18n.T("cli.target.disabled"), name)
			return nil
		},
	})

	targetCmd.AddCommand(&cobra.Command{
		Use:   "show <name>",
		Short: "Show details for a specific target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			t, expanded, err := appService.TargetShow(name)
			if err != nil {
				return err
			}

			deployed := appService.TargetDeployedSkills(name)

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"name":           name,
					"type":           t.Type,
					"channel":        t.Channel,
					"enabled":        t.IsEnabled(),
					"configDir":      t.ConfigDir,
					"resolvedDir":    t.ResolveConfigDir(),
					"skillsDir":      expanded,
					"features":       t.FeatureKeys(),
					"paths":          t.Paths,
					"deployedSkills": deployed,
				})
			}

			out.Printf(i18n.T("cli.target.name"), name)
			tType := string(t.Type)
			if tType == "" {
				tType = "agent"
			}
			out.Printf(i18n.T("cli.target.type"), tType)
			statusLabel := i18n.T("cli.target.status_enabled")
			if !t.IsEnabled() {
				statusLabel = i18n.T("cli.target.status_disabled")
			}
			out.Printf(i18n.T("cli.target.status"), statusLabel)
			if t.Channel != "" {
				out.Printf(i18n.T("cli.target.channel"), t.Channel)
			}
			out.Printf(i18n.T("cli.target.config_dir"), t.ConfigDir, t.ResolveConfigDir())
			out.Printf(i18n.T("cli.target.features"), t.FeaturesString())
			out.Printf(i18n.T("cli.target.skills_path"), expanded)
			if t.Paths != nil {
				for k, v := range t.Paths {
					out.Printf(i18n.T("cli.target.mapping"), k, v, t.ResolvePath(k))
				}
			}
			if len(deployed) > 0 {
				out.Printf(i18n.T("cli.target.deployed_list"), len(deployed), strings.Join(deployed, ", "))
			} else {
				out.Printf("%s", i18n.T("cli.target.deployed_none"))
			}
			return nil
		},
	})

	targetCmd.AddCommand(&cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a target from configuration",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := appService.TargetRemove(name); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status": "removed",
					"name":   name,
				})
			}

			out.Successf(i18n.T("cli.target.removed"), name)
			return nil
		},
	})

	return targetCmd
}
