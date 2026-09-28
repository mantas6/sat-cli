package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

const dashboardRequestTimeout = 10 * time.Second

var errDashboardNeedsTerminal = errors.New("dashboard follow mode requires a terminal on stdin and stdout")

func newDashboardCommand(app *App) *cobra.Command {
	var follow time.Duration
	command := &cobra.Command{
		Use:     "dashboard [interval]",
		Aliases: []string{"dash"},
		Short:   "Show the dashboard",
		Long: "Show the dashboard once, or refresh it in a full-screen terminal.\n\n" +
			"With --follow, an optional positional duration may be used, for example `sat dashboard --follow 10s`.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			following := cmd.Flags().Changed("follow")
			if len(args) > 0 && !following {
				return errors.New("dashboard interval requires --follow")
			}
			if len(args) == 1 {
				interval, err := time.ParseDuration(args[0])
				if err != nil {
					return fmt.Errorf("invalid dashboard follow interval %q: %w", args[0], err)
				}
				follow = interval
			}

			if !following {
				return printDashboard(cmd.Context(), app)
			}
			if follow <= 0 {
				return errors.New("dashboard follow interval must be greater than zero")
			}
			// The dashboard reads its quit keys from stdin.
			if !app.interactive() {
				return errDashboardNeedsTerminal
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			fetch := func(ctx context.Context) (string, error) {
				requestContext, cancel := context.WithTimeout(ctx, dashboardRequestTimeout)
				defer cancel()
				return client.Dashboard(requestContext)
			}
			opts := ui.DashboardOptions{Interval: follow}
			if width, height, ok := app.TermSize(); ok {
				opts.Width, opts.Height = width, height
			}
			return app.Follow(cmd.Context(), app.Stdin, app.Stdout, fetch, opts)
		},
	}
	command.Flags().DurationVarP(&follow, "follow", "f", 0, "refresh continuously (default interval "+ui.DefaultDashboardInterval.String()+")")
	command.Flags().Lookup("follow").NoOptDefVal = ui.DefaultDashboardInterval.String()
	return command
}

func printDashboard(ctx context.Context, app *App) error {
	client, err := apiClient(app)
	if err != nil {
		return err
	}
	text, err := client.Dashboard(ctx)
	if err != nil {
		return err
	}
	return writeLine(app.Stdout, text)
}
