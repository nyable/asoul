package cli

import (
	"runtime"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if out.json {
				return out.PrintJSON(map[string]string{
					"version":   Version,
					"goVersion": runtime.Version(),
					"os":        runtime.GOOS,
					"arch":      runtime.GOARCH,
				})
			}
			out.Printf("asoul %s (%s/%s, %s)\n", Version, runtime.GOOS, runtime.GOARCH, runtime.Version())
			return nil
		},
	}
}
