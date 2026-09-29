package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"asoul/internal/agent"
	"asoul/internal/modelsdev"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newModelCmd() *cobra.Command {
	modelCmd := &cobra.Command{
		Use:   "model",
		Short: "Manage and enrich Agent provider and model configurations",
		Long:  "Commands for enriching, querying, and configuring agent provider models with models.dev metadata and custom rules.",
	}

	modelCmd.AddCommand(newModelEnrichCmd())
	modelCmd.AddCommand(newModelQueryCmd())
	modelCmd.AddCommand(newModelRulesCmd())
	modelCmd.AddCommand(newModelProvidersCmd())

	return modelCmd
}

func newModelEnrichCmd() *cobra.Command {
	var (
		flagAgent     string
		flagFile      string
		flagProvider  string
		flagRules     string
		flagOverride  bool
		flagRulesOnly bool
		flagDryRun    bool
		flagNoBackup  bool
		flagRefresh   bool
	)

	cmd := &cobra.Command{
		Use:   "enrich",
		Short: "Enrich Agent configuration with models.dev metadata and rule overlays",
		Example: `  # Enrich opencode config automatically
  asoul model enrich --agent opencode

  # Preview diff without writing to disk
  asoul model enrich --agent opencode --dry-run

  # Enrich a specific file and provider with force override
  asoul model enrich --file ./opencode.jsonc --provider my-gateway --override

  # Apply custom rules file
  asoul model enrich --agent opencode --rules ./rules/opencode.json

  # Apply custom rules only, without models.dev metadata
  asoul model enrich --agent opencode --rules-only --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := agent.EnrichOptions{
				FilePath:     flagFile,
				ProviderID:   flagProvider,
				RulesPath:    flagRules,
				RulesOnly:    flagRulesOnly,
				Override:     flagOverride,
				DryRun:       flagDryRun,
				NoBackup:     flagNoBackup,
				RefreshCache: flagRefresh,
			}

			out.Progress(0, 0, i18n.T("progress.working"))
			summary, err := appService.EnrichModelConfig(cmd.Context(), flagAgent, opts)
			out.ClearProgress()
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(summary)
			}

			if flagDryRun && summary.Modified {
				out.Infof(i18n.T("cli.model.dry_run"), summary.ConfigFile)
			} else if summary.Modified {
				out.Successf(i18n.T("cli.model.enriched"), summary.ConfigFile)
				if summary.BackupFile != "" {
					out.Infof(i18n.T("cli.model.backup"), summary.BackupFile)
				}
			} else {
				out.Infof(i18n.T("cli.model.no_modified"), summary.ConfigFile)
				return nil
			}

			if len(summary.Results) > 0 {
				w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
				fmt.Fprintln(w, "PROVIDER\tMODEL\tMATCHED DEV\tDATA SOURCE\tRULES\tADDED FIELDS")
				for _, r := range summary.Results {
					dev := r.MatchedDev
					if dev == "" {
						dev = "-"
					}
					source := r.MatchedProvider
					if source == "" {
						source = "-"
					}
					rulesStr := "-"
					if len(r.MatchedRules) > 0 {
						rulesStr = strings.Join(r.MatchedRules, ", ")
					}
					fieldsStr := "-"
					if len(r.FieldsAdded) > 0 {
						fieldsStr = strings.Join(r.FieldsAdded, ", ")
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", r.ProviderID, r.ModelID, dev, source, rulesStr, fieldsStr)
				}
				_ = w.Flush()
			}

			if flagDryRun && summary.DiffText != "" {
				out.Println()
				out.PrintDiff(summary.DiffText)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&flagAgent, "agent", "opencode", "Target agent identifier (e.g. opencode)")
	cmd.Flags().StringVar(&flagFile, "file", "", "Explicit path to configuration file (opencode.json or .jsonc)")
	cmd.Flags().StringVar(&flagProvider, "provider", "", "Enrich only models belonging to this provider ID")
	cmd.Flags().StringVar(&flagRules, "rules", "", "Explicit path to custom agent rules JSON file")
	cmd.Flags().BoolVar(&flagRulesOnly, "rules-only", false, "Apply custom rules only, skipping models.dev metadata")
	cmd.Flags().BoolVar(&flagOverride, "override", false, "Overwrite existing metadata with models.dev official values")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Preview modifications without writing to disk")
	cmd.Flags().BoolVar(&flagNoBackup, "no-backup", false, "Disable generating .bak backup file")
	cmd.Flags().BoolVar(&flagRefresh, "refresh", false, "Force refresh local models.dev cache")

	return cmd
}

func newModelQueryCmd() *cobra.Command {
	var flagRefresh bool

	cmd := &cobra.Command{
		Use:   "query <model-id>",
		Short: "Query model specifications, context limits, pricing, and capabilities from models.dev",
		Args:  cobra.ExactArgs(1),
		Example: `  asoul model query claude-3-7-sonnet
  asoul model query gpt-4o --refresh`,
		RunE: func(cmd *cobra.Command, args []string) error {
			modelID := args[0]
			modelData, suggestions, err := appService.QueryModel(cmd.Context(), modelID, flagRefresh)
			if err != nil {
				return err
			}

			if out.json {
				if modelData != nil {
					return out.PrintJSON(modelData)
				}
				if err := out.PrintJSON(map[string]any{
					"error":       "not found",
					"code":        "model_not_found",
					"suggestions": suggestions,
				}); err != nil {
					return err
				}
				return fmt.Errorf("model %q not found in models.dev catalog", modelID)
			}

			if modelData == nil {
				out.Warnf(i18n.T("cli.model.not_found"), modelID)
				if len(suggestions) > 0 {
					out.Println(i18n.T("cli.model.did_you_mean"))
					for _, s := range suggestions {
						out.Printf(i18n.T("cli.model.suggestion"), s.ID, s.Name)
					}
				}
				return fmt.Errorf("model %q not found in models.dev catalog", modelID)
			}

			printModelData(modelData)
			return nil
		},
	}

	cmd.Flags().BoolVar(&flagRefresh, "refresh", false, "Force refresh local models.dev cache")
	return cmd
}

func newModelRulesCmd() *cobra.Command {
	var (
		flagAgent string
		flagFile  string
	)

	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List active model rules for an agent",
		Example: `  asoul model rules --agent opencode
  asoul model rules --agent opencode --file ./rules/opencode.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rules, sourcePath, err := appService.GetModelRules(flagAgent, flagFile)
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]any{
					"agent":      flagAgent,
					"source":     sourcePath,
					"rulesCount": len(rules),
					"rules":      rules,
				})
			}

			if len(rules) == 0 {
				out.Infof(i18n.T("cli.model.no_rules"), flagAgent)
				out.Printf("%s", i18n.T("cli.model.rules_help1"))
				out.Printf(i18n.T("cli.model.rules_help2"), flagAgent)
				out.Printf(i18n.T("cli.model.rules_help3"), flagAgent)
				out.Printf("%s", i18n.T("cli.model.rules_help4"))
				return nil
			}

			out.Infof(i18n.T("cli.model.active_rules"), flagAgent, sourcePath, len(rules))

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "RULE NAME\tPATTERN\tOPERATIONS\tDESCRIPTION")
			for _, r := range rules {
				var ops []string
				if len(r.Fill) > 0 {
					ops = append(ops, fmt.Sprintf("fill(%d)", len(r.Fill)))
				}
				if len(r.Merge) > 0 {
					ops = append(ops, fmt.Sprintf("merge(%d)", len(r.Merge)))
				}
				if len(r.Override) > 0 {
					ops = append(ops, fmt.Sprintf("override(%d)", len(r.Override)))
				}
				if len(r.Remove) > 0 {
					ops = append(ops, fmt.Sprintf("remove(%d)", len(r.Remove)))
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Pattern, strings.Join(ops, ", "), r.Description)
			}
			return w.Flush()
		},
	}

	cmd.Flags().StringVar(&flagAgent, "agent", "opencode", "Target agent identifier (e.g. opencode)")
	cmd.Flags().StringVar(&flagFile, "file", "", "Path to custom rule file")
	return cmd
}

func printModelData(m *modelsdev.ModelData) {
	out.Successf(i18n.T("cli.model.details"), m.Name, m.ID)
	if m.Family != "" {
		out.Printf(i18n.T("cli.model.family"), m.Family)
	}
	if m.ReleaseDate != "" {
		out.Printf(i18n.T("cli.model.release_date"), m.ReleaseDate)
	}

	out.Println(i18n.T("cli.model.capabilities"))
	out.Printf(i18n.T("cli.model.reasoning"), m.Reasoning)
	if len(m.ReasoningOptions) > 0 {
		var opts []string
		for _, o := range m.ReasoningOptions {
			if len(o.Values) > 0 {
				opts = append(opts, fmt.Sprintf("%s (%s)", o.Type, strings.Join(o.Values, ", ")))
			} else {
				opts = append(opts, o.Type)
			}
		}
		out.Printf(i18n.T("cli.model.reasoning_options"), strings.Join(opts, "; "))
	}
	out.Printf(i18n.T("cli.model.tool_call"), m.ToolCall)
	out.Printf(i18n.T("cli.model.attachment"), m.Attachment)
	out.Printf(i18n.T("cli.model.temperature"), m.Temperature)

	if m.Limit != nil {
		out.Println(i18n.T("cli.model.token_limits"))
		out.Printf(i18n.T("cli.model.context_window"), m.Limit.Context)
		out.Printf(i18n.T("cli.model.max_output"), m.Limit.Output)
	}

	if m.Cost != nil {
		out.Println(i18n.T("cli.model.pricing"))
		out.Printf(i18n.T("cli.model.price_input"), m.Cost.Input)
		out.Printf(i18n.T("cli.model.price_output"), m.Cost.Output)
		if m.Cost.CacheRead > 0 {
			out.Printf(i18n.T("cli.model.price_cache_read"), m.Cost.CacheRead)
		}
		if m.Cost.CacheWrite > 0 {
			out.Printf(i18n.T("cli.model.price_cache_write"), m.Cost.CacheWrite)
		}
	}
}

func newModelProvidersCmd() *cobra.Command {
	var (
		flagInit  bool
		flagReset bool
	)

	cmd := &cobra.Command{
		Use:   "providers",
		Short: "List or initialize official model provider mappings in config.jsonc",
		Example: `  # List active official providers and regex patterns
  asoul model providers

  # Initialize / persist default official providers into config.jsonc
  asoul model providers --init`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagInit || flagReset {
				if err := appService.InitOfficialProviders(); err != nil {
					return err
				}
				if !out.json {
					out.Successf(i18n.T("cli.model.providers_initialized"))
				}
			}

			providers, configPath, isCustom, err := appService.GetOfficialProviders()
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(map[string]any{
					"configPath": configPath,
					"isCustom":   isCustom,
					"providers":  providers,
				})
			}

			sourceDesc := "Built-in defaults (run with --init to persist to config.jsonc)"
			if isCustom {
				sourceDesc = fmt.Sprintf("Configured in %s", configPath)
			}

			out.Infof(i18n.T("cli.model.providers_title"), sourceDesc)

			var sortedKeys []string
			for k := range providers {
				sortedKeys = append(sortedKeys, k)
			}
			sort.Strings(sortedKeys)

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "PROVIDER\tPATTERNS (REGEX)")
			for _, k := range sortedKeys {
				fmt.Fprintf(w, "%s\t%s\n", k, strings.Join(providers[k], ", "))
			}
			return w.Flush()
		},
	}

	cmd.Flags().BoolVar(&flagInit, "init", false, "Initialize default official providers into config.jsonc")
	cmd.Flags().BoolVar(&flagReset, "reset", false, "Reset official providers in config.jsonc to defaults")
	return cmd
}
