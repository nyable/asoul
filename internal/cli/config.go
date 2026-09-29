package cli

import (
	"fmt"

	"asoul/internal/config"
	"asoul/internal/i18n"
	"asoul/internal/model"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage asoul user configuration",
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the path to the configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgMgr, err := config.NewManager(flagConfig)
			if err != nil {
				return err
			}
			if out.json {
				return out.PrintJSON(map[string]string{
					"path": cfgMgr.Path(),
				})
			}
			out.Println(cfgMgr.Path())
			return nil
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "get <key>",
		Short: "Get configuration value (e.g. default_root)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			cfgMgr, err := config.NewManager(flagConfig)
			if err != nil {
				return err
			}
			cfg, err := cfgMgr.Load()
			if err != nil {
				return err
			}

			var val interface{}
			switch key {
			case "default_root":
				val = cfg.DefaultRoot
			case "version":
				val = cfg.Version
			case "language":
				val = cfg.Language
			default:
				return fmt.Errorf("unknown config key %q", key)
			}

			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"key":   key,
					"value": val,
				})
			}
			out.Println(val)
			return nil
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set configuration value (e.g. default_root <path>)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, val := args[0], args[1]
			cfgMgr, err := config.NewManager(flagConfig)
			if err != nil {
				return err
			}

			if err := cfgMgr.Update(func(cfg *model.Config) error {
				switch key {
				case "default_root":
					cfg.DefaultRoot = val
				case "language":
					cfg.Language = val
				default:
					return fmt.Errorf("cannot set unsupported config key %q", key)
				}
				return nil
			}); err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]string{
					"status": "updated",
					"key":    key,
					"value":  val,
				})
			}

			out.Successf(i18n.T("cli.config.set"), key, val)
			return nil
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Run the interactive configuration wizard to generate config.jsonc",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgMgr, err := config.NewManager(flagConfig)
			if err != nil {
				return err
			}
			_, err = config.RunInteractiveWizard(cfgMgr)
			return err
		},
	})

	return configCmd
}
