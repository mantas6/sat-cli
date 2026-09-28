package command

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/config"
	"golang.org/x/term"
)

// executeLogin runs `sat auth login` with args against cfg, reading input
// from a non-terminal stdin, and returns stderr.
func executeLogin(t *testing.T, cfg *fakeConfig, input string, args ...string) (string, error) {
	t.Helper()
	app, _, stderr := newTestApp(t, withConfig(cfg), withStdin(strings.NewReader(input)))
	err := run(t, app, append([]string{"auth", "login"}, args...)...)
	return stderr.String(), err
}

func TestLoginPromptsForMissingURLAndToken(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{}
	stderr, err := executeLogin(t, cfg, "https://sat.example\nnew-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.baseURL != "https://sat.example" || cfg.token != "new-token" {
		t.Fatalf("config = URL %q, token %q", cfg.baseURL, cfg.token)
	}
	if want := "Base URL: URL saved.\nToken: Token saved.\n"; stderr != want {
		t.Fatalf("stderr = %q, want %q", stderr, want)
	}
}

func TestLoginSkipsConfiguredURL(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://existing.example"}
	stderr, err := executeLogin(t, cfg, "new-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.baseURL != "https://existing.example" || cfg.token != "new-token" {
		t.Fatalf("config = URL %q, token %q", cfg.baseURL, cfg.token)
	}
	if strings.Contains(stderr, "Base URL: ") {
		t.Fatalf("unexpected URL prompt: %q", stderr)
	}
}

func TestLoginReplacesURLWithFlag(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://old.example"}
	if _, err := executeLogin(t, cfg, "https://new.example\nnew-token\n", "--replace-url"); err != nil {
		t.Fatal(err)
	}
	if cfg.baseURL != "https://new.example" || cfg.token != "new-token" {
		t.Fatalf("config = URL %q, token %q", cfg.baseURL, cfg.token)
	}
}

func TestLoginURLOnlyReplacesURLWithoutToken(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://old.example", token: "existing-token"}
	stderr, err := executeLogin(t, cfg, "https://new.example\n", "--url-only")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.baseURL != "https://new.example" || cfg.token != "existing-token" {
		t.Fatalf("config = URL %q, token %q", cfg.baseURL, cfg.token)
	}
	if strings.Contains(stderr, "Token") {
		t.Fatalf("stderr = %q, want no token prompt", stderr)
	}
}

func TestLoginWarnsWhenReplacingToken(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://sat.example", token: "old-token"}
	stderr, err := executeLogin(t, cfg, "new-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "Token is already defined; it will be replaced\n") {
		t.Fatalf("stderr = %q", stderr)
	}
	if cfg.token != "new-token" {
		t.Fatalf("token = %q", cfg.token)
	}
}

func TestLoginRejectsEmptyToken(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://sat.example", token: "old-token"}
	if _, err := executeLogin(t, cfg, "\n"); !errors.Is(err, config.ErrEmptyToken) {
		t.Fatalf("err = %v, want ErrEmptyToken", err)
	}
	if cfg.token != "old-token" {
		t.Fatalf("token = %q, want the old token kept", cfg.token)
	}
}

func TestLoginRejectsInvalidURLNonInteractively(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{}
	stderr, err := executeLogin(t, cfg, "not-a-url\ntoken\n")
	if err == nil || !strings.Contains(err.Error(), `invalid base URL "not-a-url": use an absolute http or https URL`) {
		t.Fatalf("err = %v, want invalid base URL error", err)
	}
	if strings.Contains(stderr, "Invalid base URL") {
		t.Fatalf("stderr = %q, want no retry prompt without a terminal", stderr)
	}
	if cfg.baseURL != "" || cfg.token != "" {
		t.Fatalf("config changed to URL %q, token %q", cfg.baseURL, cfg.token)
	}
}

func TestLoginRetriesInvalidURLInteractively(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{}
	app, _, stderr := newTestApp(t, withConfig(cfg), withTTY(),
		withStdin(strings.NewReader("bad\nhttps://sat.example\n")))
	if err := run(t, app, "auth", "login", "--url-only"); err != nil {
		t.Fatal(err)
	}
	if cfg.baseURL != "https://sat.example" {
		t.Fatalf("base URL = %q", cfg.baseURL)
	}
	if strings.Count(stderr.String(), "Base URL: ") != 2 || !strings.Contains(stderr.String(), "Invalid base URL: ") {
		t.Fatalf("stderr = %q, want a retry after the invalid URL", stderr.String())
	}
}

func TestLoginReadsNonInteractiveTokenFromStdin(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://sat.example"}
	stderr, err := executeLogin(t, cfg, "piped-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.token != "piped-token" {
		t.Fatalf("token = %q", cfg.token)
	}
	if !strings.Contains(stderr, "Token saved.\n") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestLoginTokenPromptCancels(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://sat.example"}
	// A real *os.File selects the hidden token path; ReadPassword is stubbed,
	// so nothing is read from it.
	stdin, _ := pipe(t)

	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	restored := make(chan struct{}, 1)

	app, _, _ := newTestApp(t, withConfig(cfg), withStdin(stdin), withTTY())
	app.ReadPassword = func(int) ([]byte, error) {
		close(started)
		<-release
		return nil, io.EOF
	}
	app.TerminalState = func(int) (*term.State, error) { return &term.State{}, nil }
	app.RestoreTerminal = func(int, *term.State) error {
		restored <- struct{}{}
		return nil
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- runContext(ctx, app, "auth", "login") }()

	<-started
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login did not cancel in time")
	}

	select {
	case <-restored:
	default:
		t.Fatal("RestoreTerminal was not called")
	}

	if cfg.token != "" || cfg.baseURL != "https://sat.example" {
		t.Fatalf("config changed to URL %q, token %q", cfg.baseURL, cfg.token)
	}
}

func TestLoginTokenPromptEOFReturnsEmptyToken(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{baseURL: "https://sat.example"}
	stdin, _ := pipe(t)

	app, _, stderr := newTestApp(t, withConfig(cfg), withStdin(stdin), withTTY())
	app.ReadPassword = func(int) ([]byte, error) { return nil, io.EOF }
	app.TerminalState = func(int) (*term.State, error) { return &term.State{}, nil }

	err := run(t, app, "auth", "login")
	if !errors.Is(err, config.ErrEmptyToken) {
		t.Fatalf("err = %v, want ErrEmptyToken", err)
	}
	if cfg.token != "" {
		t.Fatalf("token = %q", cfg.token)
	}
	if stderr.String() != "Token: \n" {
		t.Fatalf("stderr = %q, want the prompt line terminated", stderr.String())
	}
}

func TestLoginURLPromptCancels(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{}

	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	var startedOnce bool
	reader := readerFunc(func([]byte) (int, error) {
		if !startedOnce {
			startedOnce = true
			close(started)
		}
		<-release
		return 0, io.EOF
	})
	app, _, _ := newTestApp(t, withConfig(cfg), withStdin(reader))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- runContext(ctx, app, "auth", "login") }()

	<-started
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login did not cancel in time")
	}

	if cfg.baseURL != "" || cfg.token != "" {
		t.Fatalf("config changed to URL %q, token %q", cfg.baseURL, cfg.token)
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
