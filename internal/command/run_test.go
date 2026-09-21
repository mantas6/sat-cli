package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

type recordingSSHRunner struct {
	name   string
	args   []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	err    error
}

func (r *recordingSSHRunner) Run(_ context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	r.name = name
	r.args = append([]string(nil), args...)
	r.stdin = stdin
	r.stdout = stdout
	r.stderr = stderr
	return r.err
}

func TestSSHTargetFromEnvironmentWithoutBaseURL(t *testing.T) {
	env := map[string]string{
		"REMOTE_HOST": "ssh.example.test",
		"REMOTE_USER": "deploy",
		"REMOTE_ROOT": "/srv/satellite/current",
	}
	target, err := resolveSSHTarget(func(key string) string { return env[key] }, "")
	if err != nil {
		t.Fatal(err)
	}
	want := sshTarget{Host: "ssh.example.test", User: "deploy", Root: "/srv/satellite/current"}
	if target != want {
		t.Fatalf("resolveSSHTarget() = %#v, want %#v", target, want)
	}
}

func TestSSHTargetDerivedFromURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		wantHost string
	}{
		{name: "three labels", baseURL: "https://sat.example.com", wantHost: "example.com"},
		{name: "two labels", baseURL: "https://example.com", wantHost: "example.com"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := resolveSSHTarget(func(string) string { return "" }, test.baseURL)
			if err != nil {
				t.Fatal(err)
			}
			if target.Host != test.wantHost {
				t.Fatalf("Host = %q, want %q", target.Host, test.wantHost)
			}
			if target.Root != "/home/mantas/"+strings.TrimPrefix(test.baseURL, "https://")+"/current" {
				t.Fatalf("Root = %q", target.Root)
			}
		})
	}
}

func TestSSHRemoteRootDefaultsFromFullURLHost(t *testing.T) {
	env := map[string]string{"REMOTE_HOST": "configured-alias"}
	target, err := resolveSSHTarget(func(key string) string { return env[key] }, "https://sat.example.com:8443/path")
	if err != nil {
		t.Fatal(err)
	}
	if target.Root != "/home/mantas/sat.example.com/current" {
		t.Fatalf("Root = %q, want default based on full hostname", target.Root)
	}
}

func TestSSHCommandUsesUserPrefixAndQuotesArguments(t *testing.T) {
	runner := &recordingSSHRunner{}
	app := newSSHTestApp(runner, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_USER": "deploy",
		"REMOTE_ROOT": "/srv/sat release/current",
	})

	if err := executeRunTestCommand(app, "run", "task", "two words", "it's", "$HOME"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-o", "LogLevel=QUIET", "deploy@server",
		"cd '/srv/sat release/current' && php artisan 'task' 'two words' 'it'\\''s' '$HOME'",
	}
	if runner.name != sshBinary || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("Run() = %q %#v, want %q %#v", runner.name, runner.args, sshBinary, want)
	}
}

func TestSSHCommandPassesFlagsToArtisan(t *testing.T) {
	runner := &recordingSSHRunner{}
	app := newSSHTestApp(runner, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_ROOT": "/srv/current",
	})

	if err := executeRunTestCommand(app, "run", "migrate", "--force"); err != nil {
		t.Fatal(err)
	}
	if got := runner.args[len(runner.args)-1]; got != "cd '/srv/current' && php artisan 'migrate' '--force'" {
		t.Fatalf("remote command = %q", got)
	}
}

func TestSSHCommandOmitsArtisanArgumentsWhenEmpty(t *testing.T) {
	runner := &recordingSSHRunner{}
	app := newSSHTestApp(runner, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_ROOT": "/srv/current",
	})

	if err := executeRunTestCommand(app, "run"); err != nil {
		t.Fatal(err)
	}
	if got := runner.args[len(runner.args)-1]; got != "cd '/srv/current' && php artisan" {
		t.Fatalf("remote command = %q", got)
	}
}

func TestSSHCommandAddsTTYOnlyForTerminalFiles(t *testing.T) {
	tests := []struct {
		name   string
		stdin  io.Reader
		stdout io.Writer
		term   bool
		want   bool
	}{
		{name: "terminal files", stdin: os.Stdin, stdout: os.Stdout, term: true, want: true},
		{name: "non-terminal files", stdin: os.Stdin, stdout: os.Stdout, term: false, want: false},
		{name: "non-file streams", stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, term: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &recordingSSHRunner{}
			app := newSSHTestApp(runner, map[string]string{
				"REMOTE_HOST": "server",
				"REMOTE_ROOT": "/srv/current",
			})
			app.Stdin = test.stdin
			app.Stdout = test.stdout
			app.IsTerminal = func(int) bool { return test.term }

			if err := executeRunTestCommand(app, "run"); err != nil {
				t.Fatal(err)
			}
			got := len(runner.args) >= 3 && runner.args[2] == "-t"
			if got != test.want {
				t.Fatalf("TTY argument present = %v, want %v; args = %#v", got, test.want, runner.args)
			}
		})
	}
}

func TestSSHCommandPreservesExitCode(t *testing.T) {
	execErr := exec.Command("sh", "-c", "exit 3").Run()
	var processErr *exec.ExitError
	if !errors.As(execErr, &processErr) {
		t.Fatalf("test setup error = %v, want *exec.ExitError", execErr)
	}

	runner := &recordingSSHRunner{err: execErr}
	app := newSSHTestApp(runner, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_ROOT": "/srv/current",
	})
	err := executeRunTestCommand(app, "run")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("Execute() error = %#v, want ExitError with code 3", err)
	}
	if !errors.Is(err, execErr) {
		t.Fatalf("Execute() error does not wrap process error")
	}
}

func TestSSHCommandMapsSignalExitToConventionalCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal termination semantics are unix-specific")
	}

	// A process that kills itself with SIGTERM exits via a signal, so
	// ExitCode reports -1 and the command must map it to 128+SIGTERM.
	execErr := exec.Command("sh", "-c", "kill -TERM $$").Run()
	var processErr *exec.ExitError
	if !errors.As(execErr, &processErr) {
		t.Fatalf("test setup error = %v, want *exec.ExitError", execErr)
	}
	if processErr.ExitCode() != -1 {
		t.Fatalf("ExitCode() = %d, want -1 (signalled)", processErr.ExitCode())
	}

	runner := &recordingSSHRunner{err: execErr}
	app := newSSHTestApp(runner, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_ROOT": "/srv/current",
	})
	err := executeRunTestCommand(app, "run")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 128+int(syscall.SIGTERM) {
		t.Fatalf("Execute() error = %#v, want ExitError with code %d", err, 128+int(syscall.SIGTERM))
	}
	if !errors.Is(err, execErr) {
		t.Fatalf("Execute() error does not wrap process error")
	}
}

func TestExecRunnerForwardsCancellationAsSignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal forwarding is unix-specific")
	}

	original := termGracePeriod
	termGracePeriod = 100 * time.Millisecond
	t.Cleanup(func() { termGracePeriod = original })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- ExecRunner{}.Run(ctx, "sleep", []string{"10"}, nil, io.Discard, io.Discard)
	}()

	time.AfterFunc(50*time.Millisecond, cancel)

	select {
	case err := <-result:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Run() error = %v, want *exec.ExitError", err)
		}
		code, ok := signalExitCode(exitErr)
		if !ok {
			t.Fatalf("signalExitCode() reported no signal for %v", exitErr)
		}
		if code != 128+int(syscall.SIGTERM) {
			t.Fatalf("derived exit code = %d, want %d", code, 128+int(syscall.SIGTERM))
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestSSHCommandHelpDoesNotRunSSH(t *testing.T) {
	for _, helpArg := range []string{"--help", "-h"} {
		t.Run(helpArg, func(t *testing.T) {
			var output bytes.Buffer
			runner := &recordingSSHRunner{}
			app := newSSHTestApp(runner, nil)
			app.Stdout = &output

			if err := executeRunTestCommand(app, "run", helpArg); err != nil {
				t.Fatal(err)
			}
			if runner.name != "" {
				t.Fatalf("runner called with %q", runner.name)
			}
			if !strings.Contains(output.String(), "run [artisan arguments...]") {
				t.Fatalf("help output = %q", output.String())
			}
		})
	}
}

func TestSSHCommandSkipsFailingBaseURLWhenTargetIsConfigured(t *testing.T) {
	runner := &recordingSSHRunner{}
	app := newSSHTestApp(runner, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_ROOT": "/srv/current",
	})
	app.Config = &stubConfig{baseErr: errors.New("base URL unavailable")}

	if err := executeRunTestCommand(app, "run"); err != nil {
		t.Fatal(err)
	}
}

func newSSHTestApp(runner ProcessRunner, env map[string]string) *App {
	output := &bytes.Buffer{}
	return &App{
		Config: &stubConfig{baseURL: "https://sat.example.com"},
		Stdin:  &bytes.Buffer{},
		Stdout: output,
		Stderr: output,
		Getenv: func(key string) string {
			return env[key]
		},
		Runner: runner,
	}
}

func executeRunTestCommand(app *App, args ...string) error {
	command := NewRootCommand(app)
	command.SetArgs(args)
	return command.Execute()
}
