package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/mantas6/sat-cli/internal/command"
	"github.com/mantas6/sat-cli/internal/ui"
)

func main() {
	ctx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := command.NewDefaultApp()
	if err != nil {
		exit(err, nil)
	}
	info, ok := debug.ReadBuildInfo()
	app.Version = resolveVersion(version, info, ok)
	root := command.NewRootCommand(app)

	if err := root.ExecuteContext(ctx); err != nil {
		exit(err, context.Cause(ctx))
	}
}

// signalCause is the cancellation cause recorded by notifyContext. Like the
// cause set by signal.NotifyContext it matches context.Canceled, but it also
// keeps the signal so the exit code can reflect it.
type signalCause struct {
	signal os.Signal
}

func (c signalCause) Error() string { return c.signal.String() + " signal received" }

func (c signalCause) Is(target error) bool { return target == context.Canceled }

// notifyContext is signal.NotifyContext with a cause that exposes which
// signal cancelled the context.
func notifyContext(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	received := make(chan os.Signal, 1)
	signal.Notify(received, signals...)
	go func() {
		select {
		case sig := <-received:
			cancel(signalCause{signal: sig})
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(received)
		cancel(nil)
	}
}

// exit reports err and terminates the process. cause is the context
// cancellation cause, if any.
func exit(err, cause error) {
	if !silent(err, cause) {
		fmt.Fprintf(os.Stderr, "sat: %v\n", err)
	}
	os.Exit(exitCode(err, cause))
}

// silent reports whether err only carries an exit code: the failure was
// already reported by a child process (editor, ssh), the user cancelled a
// picker or sent a signal, or there is no underlying error at all.
func silent(err, cause error) bool {
	var signalled signalCause
	if errors.Is(err, context.Canceled) && errors.As(cause, &signalled) {
		return true
	}
	var exitErr command.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	if exitErr.Err == nil {
		return true
	}
	var processErr *exec.ExitError
	return errors.As(exitErr.Err, &processErr) || errors.Is(exitErr.Err, ui.ErrCancelled)
}

// exitCode maps err to a process exit code. A cancellation exits with
// 128+signal when cause records the signal that cancelled the context (143
// for SIGTERM) and 130, as for SIGINT, otherwise.
func exitCode(err, cause error) int {
	if errors.Is(err, context.Canceled) {
		var signalled signalCause
		if errors.As(cause, &signalled) {
			if number, ok := signalled.signal.(syscall.Signal); ok {
				return 128 + int(number)
			}
		}
		return 130
	}

	var exitErr command.ExitError
	if errors.As(err, &exitErr) && exitErr.Code != 0 {
		return exitErr.Code
	}

	return 1
}
