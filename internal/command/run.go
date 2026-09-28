package command

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const sshBinary = "ssh"

// defaultRemoteDir is the release directory used when REMOTE_ROOT is unset.
// $HOME is left outside single quotes so the remote login shell expands it.
const defaultRemoteDir = `"$HOME"/Sat/current`

type sshTarget struct {
	Host string
	User string
	// Root is the remote release directory; empty selects defaultRemoteDir.
	Root string
}

// remoteDir returns the shell expression for the remote release directory. A
// user-supplied root is quoted verbatim; the default expands $HOME remotely.
func (t sshTarget) remoteDir() string {
	if t.Root == "" {
		return defaultRemoteDir
	}
	return shellQuote(t.Root)
}

func init() {
	registerCommand(newRunCommand)
}

func newRunCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:                "run [artisan arguments...]",
		Short:              "Run Artisan on the remote Satellite host",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				return command.Help()
			}

			var baseURL string
			if app.Getenv("REMOTE_HOST") == "" {
				var err error
				baseURL, err = app.Config.BaseURL()
				if err != nil {
					return err
				}
			}

			target, err := resolveSSHTarget(app.Getenv, baseURL)
			if err != nil {
				return err
			}

			remoteCommand := "cd " + target.remoteDir() + " && php artisan"
			if len(args) > 0 {
				remoteCommand += " " + shellJoin(args)
			}

			// ERROR keeps connection and authentication failures visible while
			// hiding banners and warnings.
			sshArgs := []string{"-o", "LogLevel=ERROR"}
			stdin, stdinOK := app.Stdin.(*os.File)
			stdout, stdoutOK := app.Stdout.(*os.File)
			if stdinOK && stdoutOK && app.IsTerminal(int(stdin.Fd())) && app.IsTerminal(int(stdout.Fd())) {
				sshArgs = append(sshArgs, "-t")
			}

			destination := target.Host
			if target.User != "" {
				destination = target.User + "@" + destination
			}
			sshArgs = append(sshArgs, destination, remoteCommand)

			// ExecRunner forwards signals to ssh and terminates it gracefully on
			// context cancellation rather than hard-killing the process.
			err = app.Runner.Run(command.Context(), sshBinary, sshArgs, app.Stdin, app.Stdout, app.Stderr)
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code := exitErr.ExitCode()
				if code == -1 {
					// Killed by a signal; map to the conventional 128+signal code.
					if signalCode, ok := signalExitCode(exitErr); ok {
						code = signalCode
					} else {
						code = 130
					}
				}
				return ExitError{Code: code, Err: err}
			}
			return err
		},
	}

	return command
}

func resolveSSHTarget(getenv func(string) string, baseURL string) (sshTarget, error) {
	target := sshTarget{
		Host: getenv("REMOTE_HOST"),
		User: getenv("REMOTE_USER"),
		Root: getenv("REMOTE_ROOT"),
	}
	if target.Host != "" {
		return target, nil
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return sshTarget{}, fmt.Errorf("parse base URL for SSH target: %w", err)
	}
	baseHost := strings.TrimSuffix(parsed.Hostname(), ".")
	if baseHost == "" {
		return sshTarget{}, fmt.Errorf("base URL %q has no host", baseURL)
	}

	target.Host = baseHost
	labels := strings.Split(baseHost, ".")
	if len(labels) >= 3 && net.ParseIP(baseHost) == nil {
		target.Host = strings.Join(labels[1:], ".")
	}

	return target, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shellJoin(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = shellQuote(value)
	}
	return strings.Join(quoted, " ")
}
