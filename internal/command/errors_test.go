package command

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
)

func TestLoginHintOnCredentialErrors(t *testing.T) {
	t.Parallel()
	notifyErr := func(err error) *fakeAPI {
		return &fakeAPI{notify: func(context.Context, string) error { return err }}
	}
	tests := []struct {
		name     string
		baseURL  string
		token    string
		client   *fakeAPI
		args     []string
		sentinel error
		wantHint bool
	}{
		{name: "missing base URL", token: "secret", args: []string{"weather"}, sentinel: config.ErrBaseURLMissing, wantHint: true},
		{name: "missing token", baseURL: "https://sat.example", args: []string{"notify", "hi"}, sentinel: config.ErrTokenMissing, wantHint: true},
		{
			name: "unauthorized", baseURL: "https://sat.example", token: "secret", args: []string{"notify", "hi"},
			client:   notifyErr(&api.HTTPError{Status: http.StatusUnauthorized, Method: http.MethodPost, Path: "/api/notify"}),
			sentinel: api.ErrUnauthorized, wantHint: true,
		},
		{
			name: "forbidden", baseURL: "https://sat.example", token: "secret", args: []string{"notify", "hi"},
			client:   notifyErr(&api.HTTPError{Status: http.StatusForbidden, Method: http.MethodPost, Path: "/api/notify"}),
			sentinel: api.ErrForbidden,
		},
		{
			name: "server error", baseURL: "https://sat.example", token: "secret", args: []string{"notify", "hi"},
			client: notifyErr(&api.HTTPError{Status: http.StatusInternalServerError, Method: http.MethodPost, Path: "/api/notify"}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := newFakeConfig(t)
			cfg.baseURL, cfg.token = test.baseURL, test.token
			client := test.client
			if client == nil {
				client = &fakeAPI{}
			}
			app, _, _ := newTestApp(t, withConfig(cfg), withAPI(client))

			err := run(t, app, test.args...)
			if err == nil {
				t.Fatal("Execute() succeeded, want error")
			}
			if test.sentinel != nil && !errors.Is(err, test.sentinel) {
				t.Fatalf("err = %v, want errors.Is %v", err, test.sentinel)
			}
			if got := strings.HasSuffix(err.Error(), "; run `sat auth login`"); got != test.wantHint {
				t.Fatalf("err = %q, hint present = %v, want %v", err, got, test.wantHint)
			}
		})
	}
}

func TestLoginEmptyTokenHasNoLoginHint(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://sat.example"}
	app, _, _ := newTestApp(t, withConfig(cfg), withStdin(strings.NewReader("\n")))

	err := run(t, app, "auth", "login")
	if !errors.Is(err, config.ErrEmptyToken) || strings.Contains(err.Error(), "sat auth login") {
		t.Fatalf("err = %v, want ErrEmptyToken without a login hint", err)
	}
}
