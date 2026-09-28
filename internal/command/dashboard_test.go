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

func dashboardText(text string) *fakeAPI {
	return &fakeAPI{dashboard: func(context.Context) (string, error) { return text, nil }}
}

func TestDashboardOneShotPrintsResponse(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"dashboard text", "dashboard text\n"} {
		app, stdout, _ := newTestApp(t, withAPI(dashboardText(text)))
		if err := run(t, app, "dashboard"); err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); got != "dashboard text\n" {
			t.Fatalf("output for %q = %q, want one trailing newline", text, got)
		}
	}
}

func TestDashboardOneShotErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("dashboard failed")
	client := &fakeAPI{dashboard: func(context.Context) (string, error) {
		return "", wantErr
	}}
	app, _, _ := newTestApp(t, withAPI(client))

	if err := run(t, app, "dashboard"); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestDashboardFollowIntervalsAndFetcher(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want time.Duration
	}{
		{name: "flag without value", args: []string{"dashboard", "--follow"}, want: 5 * time.Second},
		{name: "alias and short flag", args: []string{"dash", "-f"}, want: 5 * time.Second},
		{name: "positional value", args: []string{"dashboard", "--follow", "10s"}, want: 10 * time.Second},
		{name: "equals value", args: []string{"dashboard", "--follow=10s"}, want: 10 * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			apiCalls := 0
			client := &fakeAPI{dashboard: func(ctx context.Context) (string, error) {
				apiCalls++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("Dashboard() context has no request timeout")
				}
				return "latest", nil
			}}
			app, _, _ := newTestApp(t, withAPI(client), withTTY())
			app.TermSize = func() (int, int, bool) { return 120, 40, true }
			var gotOpts ui.DashboardOptions
			app.Follow = func(ctx context.Context, _ io.Reader, _ io.Writer, fetch ui.Fetcher, opts ui.DashboardOptions) error {
				gotOpts = opts
				text, err := fetch(ctx)
				if err != nil {
					return err
				}
				if text != "latest" {
					t.Errorf("fetch() = %q, want latest", text)
				}
				return nil
			}

			if err := run(t, app, test.args...); err != nil {
				t.Fatal(err)
			}
			if want := (ui.DashboardOptions{Interval: test.want, Size: ui.Size{Width: 120, Height: 40}}); gotOpts != want {
				t.Fatalf("follow options = %#v, want %#v", gotOpts, want)
			}
			if apiCalls != 1 {
				t.Fatalf("Dashboard() calls = %d, want 1", apiCalls)
			}
		})
	}
}

func TestDashboardFollowRejectsInvalidIntervals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"dashboard", "--follow", "invalid"}, `invalid dashboard follow interval "invalid"`},
		{[]string{"dashboard", "--follow=0s"}, "dashboard follow interval must be greater than zero"},
		{[]string{"dashboard", "--follow=-1s"}, "dashboard follow interval must be greater than zero"},
		{[]string{"dashboard", "10s"}, "dashboard interval requires --follow"},
		{[]string{"dashboard", "--follow", "1s", "2s"}, "accepts at most 1 arg(s), received 2"},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			t.Parallel()
			app, _, _ := newTestApp(t, withTTY())
			if err := run(t, app, test.args...); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDashboardFollowRequiresTerminal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		terminal func(app *App) any
	}{
		// newTestApp streams are not terminals and Follow fails the test if run.
		{name: "neither", terminal: func(*App) any { return nil }},
		{name: "stdout only", terminal: func(app *App) any { return app.Stdout }},
		{name: "stdin only", terminal: func(app *App) any { return app.Stdin }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			app, _, _ := newTestApp(t)
			terminal := test.terminal(app)
			app.IsTTY = func(stream any) bool { return terminal != nil && stream == terminal }

			err := run(t, app, "dashboard", "--follow")
			if !errors.Is(err, errDashboardNeedsTerminal) {
				t.Fatalf("Execute() error = %v, want terminal requirement", err)
			}
		})
	}
}
