package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	defaultDashboardInterval = 5 * time.Second
	dashboardRequestTimeout  = 10 * time.Second
)

type dashboardFunc func(context.Context, io.Reader, io.Writer, ui.Fetcher, ui.DashboardOptions) error

var runDashboard dashboardFunc = ui.Follow

var errDashboardNeedsTerminal = errors.New("dashboard follow mode requires a terminal on stdout")

func init() {
	registerCommand(newDashboardCommand)
}

func newDashboardCommand(app *App) *cobra.Command {
	var follow time.Duration
	command := &cobra.Command{
		Use:   "dashboard [interval]",
		Short: "Show the dashboard",
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
			if !dashboardTerminal(app) {
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
			fillDashboardTerminalSize(app, &opts)
			return runDashboard(cmd.Context(), app.Stdin, app.Stdout, fetch, opts)
		},
	}
	command.Flags().DurationVarP(&follow, "follow", "f", 0, "refresh continuously (default interval 5s)")
	command.Flags().Lookup("follow").NoOptDefVal = defaultDashboardInterval.String()
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
	if _, err := fmt.Fprint(app.Stdout, text); err != nil {
		return err
	}
	if !strings.HasSuffix(text, "\n") {
		_, err = fmt.Fprintln(app.Stdout)
	}
	return err
}

func dashboardTerminal(app *App) bool {
	output, ok := app.Stdout.(*os.File)
	if !ok {
		return dashboardStubbed()
	}
	return app.IsTerminal(int(output.Fd()))
}

func fillDashboardTerminalSize(app *App, opts *ui.DashboardOptions) {
	output, ok := app.Stdout.(*os.File)
	if !ok {
		return
	}
	width, height, err := app.TerminalSize(int(output.Fd()))
	if err != nil {
		return
	}
	opts.Width = width
	opts.Height = height
}

func dashboardStubbed() bool {
	return fmt.Sprintf("%p", runDashboard) != fmt.Sprintf("%p", dashboardFunc(ui.Follow))
}
