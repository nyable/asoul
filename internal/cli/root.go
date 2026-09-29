package cli

import (
	"context"
	"fmt"
	"os"

	"asoul/internal/app"
	"asoul/internal/cache"
	"asoul/internal/catalog"
	"asoul/internal/config"
	"asoul/internal/i18n"
	"asoul/internal/state"

	"github.com/spf13/cobra"
)

var (
	flagRoot           string
	flagConfig         string
	flagJSON           bool
	flagQuiet          bool
	flagNoColor        bool
	flagNonInteractive bool
	flagYes            bool
	flagVerbose        bool

	appService *app.Service
	out        *Output
)

// Version string populated during build.
var Version = "0.1.0-dev"

// NewRootCmd creates the base Cobra command.
func NewRootCmd() *cobra.Command {
	out = NewOutput()

	rootCmd := &cobra.Command{
		Use:   "asoul",
		Short: "asoul is a local management and distribution tool for Agent Skills",
		Long: `asoul manages Agent Skills with a single unified store, tracks Git/Local upstream sources,
and distributes skills safely to agent targets (Codex, OpenCode, projects).`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Update output writers and settings from global flags
			out.SetWriters(cmd.OutOrStdout(), cmd.ErrOrStderr())
			out.json = flagJSON
			out.quiet = flagQuiet
			out.noColor = flagNoColor
			out.verbose = flagVerbose
			out.nonInteractive = flagNonInteractive
			out.yes = flagYes

			// In JSON mode, keep stdout as a single valid JSON document: never
			// dump usage text onto stdout when a run-time error occurs.
			cmd.SilenceUsage = flagJSON

			// Don't initialize service for init, version, completion, help
			name := cmd.Name()
			if name == "version" || name == "completion" || name == "help" {
				return nil
			}

			// Initialize config manager
			cfgMgr, err := config.NewManager(flagConfig)
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			// Initialize state manager
			stateMgr, err := state.NewManager("")
			if err != nil {
				return fmt.Errorf("state error: %w", err)
			}

			// Initialize cache manager
			cacheMgr, err := cache.NewManager("")
			if err != nil {
				return fmt.Errorf("cache error: %w", err)
			}

			// First-run configuration check: create config directory and run interactive wizard
			isConfigInit := cmd.Parent() != nil && cmd.Parent().Name() == "config" && cmd.Name() == "init"
			if !cfgMgr.Exists() && name != "init" && !isConfigInit {
				if out.IsTTY() && !flagNonInteractive && !flagJSON {
					_, err := config.RunInteractiveWizard(cfgMgr)
					if err != nil {
						out.Warnf(i18n.T("cli.root.wizard_skipped"), err)
					}
				} else {
					_ = config.CreateDefaultConfig(cfgMgr)
				}
			}

			// Find workspace root from flag, env, or configuration
			cfg, _ := cfgMgr.Load()
			if cfg != nil {
				i18n.Init(cfg.Language)
			} else {
				i18n.Init("")
			}
			root, err := catalog.FindWorkspaceRoot(flagRoot, cfg)
			if err != nil {
				// Allow doctor, config, target, cache, validate, model, workspace, tui, and init to run even if workspace root is not yet initialized
				requiresWorkspace := true
				switch cmd.Name() {
				case "asoul", "tui", "doctor", "config", "target", "cache", "validate", "model", "workspace", "init":
					requiresWorkspace = false
				}
				for curr := cmd.Parent(); curr != nil; curr = curr.Parent() {
					switch curr.Name() {
					case "config", "target", "cache", "workspace", "model":
						requiresWorkspace = false
					}
				}
				if requiresWorkspace {
					return err
				}
			}

			appService = app.NewService(root, cfgMgr, stateMgr, cacheMgr)
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no subcommands specified, launch TUI if in terminal and interactive
			if out.IsTTY() && !flagNonInteractive && !flagJSON {
				return runTUI(cmd.Context())
			}
			return cmd.Help()
		},
	}

	// Persistent global flags
	rootCmd.PersistentFlags().StringVar(&flagRoot, "root", "", "Path to workspace root directory containing catalog.json")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Path to custom asoul config file")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "Output results in JSON format to stdout")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress non-essential messages")
	rootCmd.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&flagNonInteractive, "non-interactive", false, "Disable interactive prompts and fail on ambiguity")
	rootCmd.PersistentFlags().BoolVarP(&flagYes, "yes", "y", false, "Automatically answer yes to prompts")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "Enable verbose logging to stderr")

	// Register subcommands
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newNewCmd())
	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newAdoptCmd())
	rootCmd.AddCommand(newRemoveCmd())
	rootCmd.AddCommand(newRenameCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newShowCmd())
	rootCmd.AddCommand(newStatusCmd())
	rootCmd.AddCommand(newValidateCmd())
	rootCmd.AddCommand(newCheckCmd())
	rootCmd.AddCommand(newDiffCmd())
	rootCmd.AddCommand(newUpdateCmd())
	rootCmd.AddCommand(newUpstreamCmd())
	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(newUndeployCmd())
	rootCmd.AddCommand(newTargetCmd())
	rootCmd.AddCommand(newProjectCmd())
	rootCmd.AddCommand(newGroupCmd())
	rootCmd.AddCommand(newCacheCmd())
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newWorkspaceCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newModelCmd())
	rootCmd.AddCommand(newTUICmd())
	rootCmd.AddCommand(newCompletionCmd(rootCmd))

	return rootCmd
}

// Execute runs the root CLI command.
func Execute() {
	rootCmd := NewRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// GetService returns the initialized application service (useful for tests and TUI).
func GetService() *app.Service {
	return appService
}

func runTUI(ctx context.Context) error {
	return RunTUI(ctx, appService, out)
}
