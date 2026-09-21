package command

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/ui"
)

type dashboardAPI struct {
	*stubAPI
	dashboard func(context.Context) (string, error)
}

func (d *dashboardAPI) Dashboard(ctx context.Context) (string, error) {
	if d.dashboard == nil {
		return "", nil
	}
	return d.dashboard(ctx)
}

func TestDashboardOneShotPrintsResponse(t *testing.T) {
	client := &dashboardAPI{stubAPI: &stubAPI{}, dashboard: func(context.Context) (string, error) {
		return "dashboard text", nil
	}}
	app, output := newWeatherNotifyTestApp(client)

	if err := executeWeatherNotifyTestCommand(app, "dashboard"); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "dashboard text\n" {
		t.Fatalf("output = %q, want %q", got, "dashboard text\n")
	}
}

func TestDashboardOneShotPreservesTrailingNewline(t *testing.T) {
	client := &dashboardAPI{stubAPI: &stubAPI{}, dashboard: func(context.Context) (string, error) {
		return "dashboard text\n", nil
	}}
	app, output := newWeatherNotifyTestApp(client)

	if err := executeWeatherNotifyTestCommand(app, "dashboard"); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "dashboard text\n" {
		t.Fatalf("output = %q, want one trailing newline", got)
	}
}

func TestDashboardOneShotErrorPropagates(t *testing.T) {
	wantErr := errors.New("dashboard failed")
	client := &dashboardAPI{stubAPI: &stubAPI{}, dashboard: func(context.Context) (string, error) {
		return "", wantErr
	}}
	app, _ := newWeatherNotifyTestApp(client)

	err := executeWeatherNotifyTestCommand(app, "dashboard")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestDashboardFollowIntervalsAndFetcher(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want time.Duration
	}{
		{name: "flag without value", args: []string{"dashboard", "--follow"}, want: 5 * time.Second},
		{name: "positional value", args: []string{"dashboard", "--follow", "10s"}, want: 10 * time.Second},
		{name: "equals value", args: []string{"dashboard", "--follow=10s"}, want: 10 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previous := runDashboard
			t.Cleanup(func() { runDashboard = previous })

			apiCalls := 0
			client := &dashboardAPI{stubAPI: &stubAPI{}, dashboard: func(context.Context) (string, error) {
				apiCalls++
				return "latest", nil
			}}
			var gotInterval time.Duration
			runDashboard = func(_ context.Context, _ io.Reader, _ io.Writer, fetch ui.Fetcher, opts ui.DashboardOptions) error {
				gotInterval = opts.Interval
				text, err := fetch(context.Background())
				if err != nil {
					return err
				}
				if text != "latest" {
					t.Fatalf("fetch() = %q, want latest", text)
				}
				return nil
			}
			app, _ := newWeatherNotifyTestApp(client)

			if err := executeWeatherNotifyTestCommand(app, test.args...); err != nil {
				t.Fatal(err)
			}
			if gotInterval != test.want {
				t.Fatalf("follow interval = %v, want %v", gotInterval, test.want)
			}
			if apiCalls != 1 {
				t.Fatalf("Dashboard() calls = %d, want 1", apiCalls)
			}
		})
	}
}

func TestDashboardFollowRejectsInvalidIntervals(t *testing.T) {
	tests := [][]string{
		{"dashboard", "--follow", "invalid"},
		{"dashboard", "--follow=0s"},
		{"dashboard", "--follow=-1s"},
		{"dashboard", "10s"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			app, _ := newWeatherNotifyTestApp(&dashboardAPI{stubAPI: &stubAPI{}})
			if err := executeWeatherNotifyTestCommand(app, args...); err == nil {
				t.Fatal("Execute() error = nil, want invalid interval error")
			}
		})
	}
}

func TestDashboardFollowRequiresTerminalWithoutStub(t *testing.T) {
	previous := runDashboard
	t.Cleanup(func() { runDashboard = previous })
	runDashboard = ui.Follow
	app, _ := newWeatherNotifyTestApp(&dashboardAPI{stubAPI: &stubAPI{}})

	err := executeWeatherNotifyTestCommand(app, "dashboard", "--follow")
	if !errors.Is(err, errDashboardNeedsTerminal) {
		t.Fatalf("Execute() error = %v, want terminal requirement", err)
	}
}
