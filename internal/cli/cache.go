package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"asoul/internal/i18n"

	"github.com/spf13/cobra"
)

func newCacheCmd() *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage Git mirror cache repositories",
	}

	cacheCmd.AddCommand(&cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all cached Git repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := appService.CacheList()
			if err != nil {
				return err
			}

			if out.json {
				return out.PrintJSON(entries)
			}

			if len(entries) == 0 {
				out.Println(i18n.T("cli.cache.none"))
				return nil
			}

			w := tabwriter.NewWriter(out.stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "KEY\tSIZE\tLAST USED\tURL")
			for _, e := range entries {
				sizeMB := float64(e.SizeBytes) / (1024 * 1024)
				lastUsed := "-"
				if !e.LastUsedAt.IsZero() {
					lastUsed = e.LastUsedAt.Format(time.RFC3339)
				}
				fmt.Fprintf(w, "%s\t%.2f MB\t%s\t%s\n", e.Key, sizeMB, lastUsed, e.URL)
			}
			return w.Flush()
		},
	})

	cacheCmd.AddCommand(&cobra.Command{
		Use:   "prune",
		Short: "Remove cache repositories not referenced in current workspace catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			count, bytesFreed, err := appService.CachePrune(cmd.Context())
			if err != nil {
				return err
			}

			mbFreed := float64(bytesFreed) / (1024 * 1024)
			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":     "pruned",
					"count":      count,
					"bytesFreed": bytesFreed,
				})
			}

			out.Successf(i18n.T("cli.cache.pruned"), count, mbFreed)
			return nil
		},
	})

	cacheCmd.AddCommand(&cobra.Command{
		Use:   "clean",
		Short: "Remove all cached Git repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			count, bytesFreed, err := appService.CacheClean()
			if err != nil {
				return err
			}

			mbFreed := float64(bytesFreed) / (1024 * 1024)
			if out.json {
				return out.PrintJSON(map[string]interface{}{
					"status":     "cleaned",
					"count":      count,
					"bytesFreed": bytesFreed,
				})
			}

			out.Successf(i18n.T("cli.cache.cleaned"), count, mbFreed)
			return nil
		},
	})

	return cacheCmd
}
