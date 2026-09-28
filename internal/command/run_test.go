package command

import (
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/mantas6/sat-cli/internal/config"
)

// newSSHTestApp returns an app whose base URL is https://sat.example.com and
// whose runner records the ssh invocation.
func newSSHTestApp(t *testing.T, env map[string]string) (*App, *fakeRunner) {
	t.Helper()
	cfg := newFakeConfig(t)
	cfg.baseURL = "https://sat.example.com"
	runner := &fakeRunner{}
	app, _, _ := newTestApp(t, withConfig(cfg), withEnv(env), withRunner(runner))
	return app, runner
}

func TestSSHTargetFromEnvironmentWithoutBaseURL(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	tests := []struct {
		name     string
		baseURL  string
		wantHost string
	}{
		{name: "three labels", baseURL: "https://sat.example.com", wantHost: "example.com"},
		{name: "two labels", baseURL: "https://example.com", wantHost: "example.com"},
		{name: "trailing dot", baseURL: "https://sat.example.com./", wantHost: "example.com"},
		{name: "IP address", baseURL: "http://192.168.1.10:8080", wantHost: "192.168.1.10"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			target, err := resolveSSHTarget(func(string) string { return "" }, test.baseURL)
			if err != nil {
				t.Fatal(err)
			}
			if target.Host != test.wantHost {
				t.Fatalf("Host = %q, want %q", target.Host, test.wantHost)
			}
			if got := target.remoteDir(); got != `"$HOME"/Sat/current` {
				t.Fatalf("remoteDir() = %q, want $HOME/Sat/current expression", got)
			}
		})
	}
}

func TestSSHTargetRejectsBaseURLWithoutHost(t *testing.T) {
	t.Parallel()
	if _, err := resolveSSHTarget(func(string) string { return "" }, "file:///tmp"); err == nil || !strings.Contains(err.Error(), "has no host") {
		t.Fatalf("resolveSSHTarget() error = %v, want no host", err)
	}
}

func TestSSHRemoteRootDefaultsToHomeSatCurrent(t *testing.T) {
	t.Parallel()
	env := map[string]string{"REMOTE_HOST": "configured-alias"}
	target, err := resolveSSHTarget(func(key string) string { return env[key] }, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "configured-alias" || target.Root != "" {
		t.Fatalf("resolveSSHTarget() = %#v, want configured host and default root", target)
	}
	if got := target.remoteDir(); got != `"$HOME"/Sat/current` {
		t.Fatalf("remoteDir() = %q, want $HOME/Sat/current expression", got)
	}
}

func TestSSHRemoteRootFromEnvironmentStaysQuotedVerbatim(t *testing.T) {
	t.Parallel()
	target := sshTarget{Host: "server", Root: `/srv/it's "sat" $HOME`}
	if got, want := target.remoteDir(), `'/srv/it'\''s "sat" $HOME'`; got != want {
		t.Fatalf("remoteDir() = %q, want %q", got, want)
	}
}

func TestSSHCommandDefaultsRemoteRoot(t *testing.T) {
	t.Parallel()
	app, runner := newSSHTestApp(t, nil)

	if err := run(t, app, "run", "about"); err != nil {
		t.Fatal(err)
	}
	want := []string{"-o", "LogLevel=ERROR", "example.com", `cd "$HOME"/Sat/current && php artisan 'about'`}
	if !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("Run() args = %#v, want %#v", runner.args, want)
	}
}

func TestSSHCommandUsesUserPrefixAndQuotesArguments(t *testing.T) {
	t.Parallel()
	app, runner := newSSHTestApp(t, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_USER": "deploy",
		"REMOTE_ROOT": "/srv/sat release/current",
	})

	if err := run(t, app, "run", "task", "two words", "it's", "$HOME"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-o", "LogLevel=ERROR", "deploy@server",
		"cd '/srv/sat release/current' && php artisan 'task' 'two words' 'it'\\''s' '$HOME'",
	}
	if runner.name != sshBinary || !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("Run() = %q %#v, want %q %#v", runner.name, runner.args, sshBinary, want)
	}
	if runner.stdin != app.Stdin || runner.stdout != app.Stdout || runner.stderr != app.Stderr {
		t.Fatal("ssh not attached to the app streams")
	}
}

func TestSSHCommandPassesFlagsToArtisan(t *testing.T) {
	t.Parallel()
	app, runner := newSSHTestApp(t, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_ROOT": "/srv/current",
	})

	if err := run(t, app, "run", "migrate", "--force"); err != nil {
		t.Fatal(err)
	}
	if got := runner.args[len(runner.args)-1]; got != "cd '/srv/current' && php artisan 'migrate' '--force'" {
		t.Fatalf("remote command = %q", got)
	}
}

func TestSSHCommandOmitsArtisanArgumentsWhenEmpty(t *testing.T) {
	t.Parallel()
	app, runner := newSSHTestApp(t, map[string]string{
		"REMOTE_HOST": "server",
		"REMOTE_ROOT": "/srv/current",
	})

	if err := run(t, app, "run"); err != nil {
		t.Fatal(err)
	}
	if got := runner.args[len(runner.args)-1]; got != "cd '/srv/current' && php artisan" {
		t.Fatalf("remote command = %q", got)
	}
}

func TestSSHCommandAddsTTYOnlyForTerminals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		stdinTTY  bool
		stdoutTTY bool
		want      bool
	}{
		{name: "both terminals", stdinTTY: true, stdoutTTY: true, want: true},
		{name: "stdin only", stdinTTY: true, stdoutTTY: false, want: false},
		{name: "stdout only", stdinTTY: false, stdoutTTY: true, want: false},
		{name: "no terminals", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			app, runner := newSSHTestApp(t, map[string]string{
				"REMOTE_HOST": "server",
				"REMOTE_ROOT": "/srv/current",
			})
			app.IsTTY = func(stream any) bool {
				switch stream {
				case app.Stdin:
					return test.stdinTTY
				case app.Stdout:
					return test.stdoutTTY
				}
				return false
			}

			if err := run(t, app, "run"); err != nil {
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
	t.Parallel()
	execErr := exec.Command("sh", "-c", "exit 3").Run()
	var processErr *exec.ExitError
	if !errors.As(execErr, &processErr) {
		t.Fatalf("test setup error = %v, want *exec.ExitError", execErr)
	}

	app, runner := newSSHTestApp(t, map[string]string{"REMOTE_HOST": "server"})
	runner.err = execErr
	err := run(t, app, "run")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 3 {
		t.Fatalf("Execute() error = %#v, want ExitError with code 3", err)
	}
	if !errors.Is(err, execErr) {
		t.Fatalf("Execute() error does not wrap process error")
	}
}

func TestSSHCommandReturnsStartErrors(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("ssh not found")
	app, runner := newSSHTestApp(t, map[string]string{"REMOTE_HOST": "server"})
	runner.err = wantErr

	err := run(t, app, "run")
	var exitErr ExitError
	if !errors.Is(err, wantErr) || errors.As(err, &exitErr) {
		t.Fatalf("Execute() error = %#v, want the plain start error", err)
	}
}

func TestSSHCommandMapsSignalExitToConventionalCode(t *testing.T) {
	t.Parallel()
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

	app, runner := newSSHTestApp(t, map[string]string{"REMOTE_HOST": "server"})
	runner.err = execErr
	err := run(t, app, "run")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 128+int(syscall.SIGTERM) {
		t.Fatalf("Execute() error = %#v, want ExitError with code %d", err, 128+int(syscall.SIGTERM))
	}
	if !errors.Is(err, execErr) {
		t.Fatalf("Execute() error does not wrap process error")
	}
}

func TestSSHCommandHelpDoesNotRunSSH(t *testing.T) {
	t.Parallel()
	for _, helpArg := range []string{"--help", "-h"} {
		t.Run(helpArg, func(t *testing.T) {
			t.Parallel()
			// The default runner fails the test if ssh starts.
			app, stdout, _ := newTestApp(t)

			if err := run(t, app, "run", helpArg); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), "run [artisan arguments...]") {
				t.Fatalf("help output = %q", stdout.String())
			}
		})
	}
}

func TestSSHCommandSkipsFailingBaseURLWhenHostIsConfigured(t *testing.T) {
	t.Parallel()
	app, runner := newSSHTestApp(t, map[string]string{"REMOTE_HOST": "server"})
	app.Config.(*fakeConfig).baseURLErr = errors.New("base URL unavailable")

	if err := run(t, app, "run"); err != nil {
		t.Fatal(err)
	}
	if got := runner.args[len(runner.args)-1]; got != `cd "$HOME"/Sat/current && php artisan` {
		t.Fatalf("remote command = %q", got)
	}
}

func TestSSHCommandRequiresBaseURLWithoutHost(t *testing.T) {
	t.Parallel()
	cfg := newFakeConfig(t)
	cfg.baseURL = ""
	// The default runner fails the test if ssh starts.
	app, _, _ := newTestApp(t, withConfig(cfg))

	if err := run(t, app, "run"); !errors.Is(err, config.ErrBaseURLMissing) {
		t.Fatalf("Execute() error = %v, want missing base URL", err)
	}
}
