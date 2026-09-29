package cli

import (
	"context"

	"asoul/internal/app"
	"asoul/internal/tui"

	"github.com/spf13/cobra"
)

func newTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive Terminal User Interface (TUI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunTUI(cmd.Context(), appService, out)
		},
	}
}

// RunTUI launches the Bubble Tea interactive application.
func RunTUI(ctx context.Context, svc *app.Service, out *Output) error {
	return tui.Run(ctx, svc)
}
