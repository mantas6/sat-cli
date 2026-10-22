package command

import (
	"context"
	"time"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

// followRequestTimeout bounds each API request made while following, so a
// stalled request shows the failure badge instead of freezing the view.
const followRequestTimeout = 10 * time.Second

// addFollowFlag registers -f/--follow on cmd. Without a value the flag uses
// the default refresh interval.
func addFollowFlag(cmd *cobra.Command, interval *time.Duration) {
	cmd.Flags().DurationVarP(interval, "follow", "f", 0, "refresh continuously (default interval "+ui.DefaultDashboardInterval.String()+")")
	cmd.Flags().Lookup("follow").NoOptDefVal = ui.DefaultDashboardInterval.String()
}

// runFollow shows fetch's text full-screen, refreshing every interval until
// the user quits. label names what is followed in the loading and failure
// text; empty means the dashboard.
func runFollow(ctx context.Context, app *App, interval time.Duration, label string, fetch ui.Fetcher) error {
	opts := ui.DashboardOptions{Interval: interval, Label: label}
	if width, height, ok := app.TermSize(); ok {
		opts.Width, opts.Height = width, height
	}
	timed := func(ctx context.Context) (string, error) {
		requestContext, cancel := context.WithTimeout(ctx, followRequestTimeout)
		defer cancel()
		return fetch(requestContext)
	}
	return app.Follow(ctx, app.Stdin, app.Stdout, timed, opts)
}
