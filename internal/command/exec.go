package command

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// ExecRunner runs child processes with the standard os/exec package.
type ExecRunner struct{}

// termGracePeriod is how long ExecRunner waits after a context cancellation
// (which sends SIGTERM) before hard-killing the child. It is a package-level
// variable so tests can shorten it.
var termGracePeriod = 2 * time.Second

// Run executes one child process attached to the provided streams. Instead of
// hard-killing the child when ctx is cancelled, Run forwards os.Interrupt and
// SIGTERM to the process and, on cancellation, sends SIGTERM followed by a
// Kill after a short grace period. This lets interactive children (e.g. ssh)
// tear down cleanly. The *exec.ExitError from Wait is returned unchanged.
func (ExecRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	process := exec.Command(name, args...)
	process.Stdin = stdin
	process.Stdout = stdout
	process.Stderr = stderr

	if err := process.Start(); err != nil {
		return err
	}

	done := make(chan struct{})
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		for {
			select {
			case sig := <-sigCh:
				_ = process.Process.Signal(sig)
			case <-ctx.Done():
				_ = process.Process.Signal(syscall.SIGTERM)
				select {
				case <-done:
				case <-time.After(termGracePeriod):
					_ = process.Process.Kill()
				}
				return
			case <-done:
				return
			}
		}
	}()

	err := process.Wait()
	close(done)
	signal.Stop(sigCh)
	<-watcherDone
	return err
}
