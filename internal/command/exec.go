package command

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"syscall"
	"time"
)

// ExecRunner runs child processes with the standard os/exec package.
type ExecRunner struct {
	// Grace is how long Run waits after a context cancellation (which sends
	// SIGTERM) before hard-killing the child; zero selects 2 seconds.
	Grace time.Duration
}

// Run executes one child process attached to the provided streams. When ctx is
// cancelled the child receives SIGTERM and is killed if it has not exited
// after the grace period, so interactive children (e.g. ssh) can tear down
// cleanly. Terminal signals need no forwarding: the tty delivers SIGINT to the
// whole foreground process group, child included.
//
// Run returns ctx.Err() whenever ctx was cancelled, regardless of how the
// child exited, so callers see one deterministic cancellation error.
// Otherwise the *exec.ExitError from Wait is returned unchanged.
func (r ExecRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	grace := r.Grace
	if grace <= 0 {
		grace = defaultTermGrace
	}

	process := exec.CommandContext(ctx, name, args...)
	process.Stdin = stdin
	process.Stdout = stdout
	process.Stderr = stderr
	// On platforms without SIGTERM, Signal fails and WaitDelay kills the
	// child once the grace period has passed.
	process.Cancel = func() error { return process.Process.Signal(syscall.SIGTERM) }
	process.WaitDelay = grace

	err := process.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

// childExitError turns a child process exit into an ExitError carrying the
// child's exit code, or the conventional 128+signal code when a signal
// terminated it. Other errors (start failures, cancellation) pass through.
func childExitError(err error) error {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return err
	}
	code, signalled := signalExitCode(exitErr)
	if !signalled {
		code = exitErr.ExitCode()
	}
	return ExitError{Code: code, Err: err}
}
