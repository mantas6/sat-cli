package command

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/config"
)

func TestNotifyForwardsMessage(t *testing.T) {
	t.Parallel()
	var gotMessage string
	client := &fakeAPI{notify: func(_ context.Context, message string) error {
		gotMessage = message
		return nil
	}}
	app, stdout, stderr := newTestApp(t, withAPI(client))

	if err := run(t, app, "notify", "deploy complete"); err != nil {
		t.Fatal(err)
	}
	if gotMessage != "deploy complete" {
		t.Fatalf("Notify() = %q, want %q", gotMessage, "deploy complete")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q; want no output", stdout.String(), stderr.String())
	}
}

func TestNotifyRejectsInvalidArguments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", args: []string{"notify"}, want: "accepts 1 arg(s), received 0"},
		{name: "too many", args: []string{"notify", "one", "two"}, want: "accepts 1 arg(s), received 2"},
		{name: "empty", args: []string{"notify", ""}, want: "message must not be empty"},
		{name: "blank", args: []string{"notify", "  "}, want: "message must not be empty"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeAPI{notify: func(context.Context, string) error {
				t.Error("Notify() called with invalid arguments")
				return nil
			}}
			app, _, _ := newTestApp(t, withAPI(client))
			if err := run(t, app, test.args...); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestNotifyAPIErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("notification failed")
	client := &fakeAPI{notify: func(context.Context, string) error {
		return wantErr
	}}
	app, _, _ := newTestApp(t, withAPI(client))

	if err := run(t, app, "notify", "message"); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestAuthenticatedCommandsPassConfiguredCredentials(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)
	var gotURL, gotToken string
	app.NewAPIClient = func(baseURL, token string) (APIClient, error) {
		gotURL, gotToken = baseURL, token
		return &fakeAPI{}, nil
	}

	if err := run(t, app, "notify", "message"); err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://satellite.test" || gotToken != "secret" {
		t.Fatalf("NewAPIClient(%q, %q), want configured URL and token", gotURL, gotToken)
	}
}

func TestAuthenticatedCommandsRequireToken(t *testing.T) {
	t.Parallel()
	cfg := newFakeConfig(t)
	cfg.token = ""
	app, _, _ := newTestApp(t, withConfig(cfg))
	app.NewAPIClient = func(string, string) (APIClient, error) {
		t.Error("API client built without a token")
		return &fakeAPI{}, nil
	}

	if err := run(t, app, "notify", "message"); !errors.Is(err, config.ErrTokenMissing) {
		t.Fatalf("Execute() error = %v, want missing token", err)
	}
}
