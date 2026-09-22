package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	configpkg "github.com/mantas6/sat-cli/internal/config"
	"golang.org/x/term"
)

func TestLoginPromptsForMissingURLAndToken(t *testing.T) {
	config := &memoryConfig{}
	stderr, err := executeLogin(config, "https://sat.example\nnew-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://sat.example" || config.token != "new-token" {
		t.Fatalf("config = URL %q, token %q", config.baseURL, config.token)
	}
	if !strings.Contains(stderr, "Base URL: ") || !strings.Contains(stderr, "Token: ") {
		t.Fatalf("prompts = %q", stderr)
	}
}

func TestLoginSkipsConfiguredURL(t *testing.T) {
	config := &memoryConfig{baseURL: "https://existing.example"}
	stderr, err := executeLogin(config, "new-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://existing.example" {
		t.Fatalf("base URL = %q", config.baseURL)
	}
	if strings.Contains(stderr, "Base URL: ") {
		t.Fatalf("unexpected URL prompt: %q", stderr)
	}
}

func TestLoginReplacesURLWithFlag(t *testing.T) {
	config := &memoryConfig{baseURL: "https://old.example"}
	_, err := executeLogin(config, "https://new.example\nnew-token\n", "--replace-url")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://new.example" {
		t.Fatalf("base URL = %q", config.baseURL)
	}
}

func TestLoginURLOnlyReplacesURLWithoutToken(t *testing.T) {
	config := &memoryConfig{baseURL: "https://old.example", token: "existing-token"}
	_, err := executeLogin(config, "https://new.example\n", "--url-only")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://new.example" || config.token != "existing-token" {
		t.Fatalf("config = URL %q, token %q", config.baseURL, config.token)
	}
}

func TestLoginWarnsWhenReplacingToken(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example", token: "old-token"}
	stderr, err := executeLogin(config, "new-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "Token is already defined; it will be replaced") {
		t.Fatalf("stderr = %q", stderr)
	}
	if config.token != "new-token" {
		t.Fatalf("token = %q", config.token)
	}
}

func TestLoginRejectsEmptyToken(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example"}
	if _, err := executeLogin(config, "\n"); err == nil {
		t.Fatal("expected an empty token error")
	}
}

func TestLoginRejectsInvalidURLNonInteractively(t *testing.T) {
	config := &memoryConfig{}
	if _, err := executeLogin(config, "not-a-url\ntoken\n"); err == nil {
		t.Fatal("expected an invalid URL error")
	}
	if config.baseURL != "" || config.token != "" {
		t.Fatalf("config changed to URL %q, token %q", config.baseURL, config.token)
	}
}

func TestLoginReadsNonInteractiveTokenFromStdin(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example"}
	stderr, err := executeLogin(config, "piped-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if config.token != "piped-token" {
		t.Fatalf("token = %q", config.token)
	}
	if !strings.Contains(stderr, "Token saved.") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestLoginTokenPromptCancels(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example"}

	stdin, cleanupStdin := terminalFile(t)
	defer cleanupStdin()

	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	restoreReadPassword := readPassword
	readPassword = func(int) ([]byte, error) {
		close(started)
		<-release
		return nil, io.EOF
	}
	defer func() { readPassword = restoreReadPassword }()

	restoreGetState := getTerminalState
	getTerminalState = func(int) (*term.State, error) { return &term.State{}, nil }
	defer func() { getTerminalState = restoreGetState }()

	restored := make(chan struct{}, 1)
	restoreRestore := restoreTerminal
	restoreTerminal = func(int, *term.State) error {
		restored <- struct{}{}
		return nil
	}
	defer func() { restoreTerminal = restoreRestore }()

	app := &App{
		Config:     config,
		Stdin:      stdin,
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTerminal: func(int) bool { return true },
	}
	command := newLoginCommand(app)
	command.SetArgs(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- command.ExecuteContext(ctx) }()

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
		t.Fatal("restoreTerminal was not called")
	}

	if config.token != "" || config.baseURL != "https://sat.example" {
		t.Fatalf("config changed to URL %q, token %q", config.baseURL, config.token)
	}
}

func TestLoginTokenPromptEOFReturnsMissing(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example"}

	stdin, cleanupStdin := terminalFile(t)
	defer cleanupStdin()

	restoreReadPassword := readPassword
	readPassword = func(int) ([]byte, error) { return nil, io.EOF }
	defer func() { readPassword = restoreReadPassword }()

	restoreGetState := getTerminalState
	getTerminalState = func(int) (*term.State, error) { return &term.State{}, nil }
	defer func() { getTerminalState = restoreGetState }()

	restoreRestore := restoreTerminal
	restoreTerminal = func(int, *term.State) error { return nil }
	defer func() { restoreTerminal = restoreRestore }()

	app := &App{
		Config:     config,
		Stdin:      stdin,
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTerminal: func(int) bool { return true },
	}
	command := newLoginCommand(app)
	command.SetArgs(nil)

	err := command.Execute()
	if !errors.Is(err, configpkg.ErrTokenMissing) {
		t.Fatalf("err = %v, want ErrTokenMissing", err)
	}
	if config.token != "" {
		t.Fatalf("token = %q", config.token)
	}
}

func TestLoginURLPromptCancels(t *testing.T) {
	config := &memoryConfig{}

	started := make(chan struct{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	var startedOnce bool
	reader := readerFunc(func(p []byte) (int, error) {
		if !startedOnce {
			startedOnce = true
			close(started)
		}
		<-release
		return 0, io.EOF
	})

	app := &App{
		Config:     config,
		Stdin:      reader,
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		IsTerminal: func(int) bool { return false },
	}
	command := newLoginCommand(app)
	command.SetArgs(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- command.ExecuteContext(ctx) }()

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

	if config.baseURL != "" || config.token != "" {
		t.Fatalf("config changed to URL %q, token %q", config.baseURL, config.token)
	}
}

// terminalFile returns a real *os.File (the read end of a pipe) so the hidden
// token path is taken; readPassword itself is stubbed, so nothing is read.
func terminalFile(t *testing.T) (*os.File, func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	return r, func() {
		r.Close()
		w.Close()
	}
}

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func executeLogin(config *memoryConfig, input string, args ...string) (string, error) {
	var stderr bytes.Buffer
	app := &App{
		Config:     config,
		Stdin:      strings.NewReader(input),
		Stdout:     &bytes.Buffer{},
		Stderr:     &stderr,
		IsTerminal: func(int) bool { return false },
	}
	command := newLoginCommand(app)
	command.SetArgs(args)
	err := command.Execute()
	return stderr.String(), err
}
