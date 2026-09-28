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
type ExecRunner struct {
	// Grace is how long Run waits after a context cancellation (which sends
	// SIGTERM) before hard-killing the child; zero selects 2 seconds.
	Grace time.Duration
}

// Run executes one child process attached to the provided streams. Instead of
// hard-killing the child when ctx is cancelled, Run forwards os.Interrupt and
// SIGTERM to the process and, on cancellation, sends SIGTERM followed by a
// Kill after a short grace period. This lets interactive children (e.g. ssh)
// tear down cleanly. The *exec.ExitError from Wait is returned unchanged.
func (r ExecRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	process := exec.Command(name, args...)
	process.Stdin = stdin
	process.Stdout = stdout
	process.Stderr = stderr

	if err := process.Start(); err != nil {
		return err
	}
	grace := r.Grace
	if grace <= 0 {
		grace = defaultTermGrace
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
				case <-time.After(grace):
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
