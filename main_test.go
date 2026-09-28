package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/command"
	"github.com/mantas6/sat-cli/internal/ui"
)

func TestExitCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		err   error
		cause error
		want  int
	}{
		{name: "ordinary error", err: errors.New("failed"), want: 1},
		{name: "cancellation without cause", err: fmtWrap(context.Canceled), want: 130},
		{name: "cancellation by caller", err: context.Canceled, cause: context.Canceled, want: 130},
		{name: "interrupt", err: fmtWrap(context.Canceled), cause: signalCause{signal: syscall.SIGINT}, want: 130},
		{name: "terminate", err: context.Canceled, cause: signalCause{signal: syscall.SIGTERM}, want: 143},
		{name: "signal cause without cancellation", err: errors.New("failed"), cause: signalCause{signal: syscall.SIGTERM}, want: 1},
		{name: "exit value", err: command.ExitError{Code: 12, Err: errors.New("failed")}, want: 12},
		{name: "wrapped exit value", err: fmt.Errorf("context: %w", command.ExitError{Code: 13}), want: 13},
		{name: "zero exit code", err: command.ExitError{Err: errors.New("failed")}, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := exitCode(test.err, test.cause); got != test.want {
				t.Fatalf("exitCode() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestNotifyContextRecordsSignal(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("cannot send SIGTERM to self")
	}
	ctx, stop := notifyContext(t.Context(), syscall.SIGTERM)
	defer stop()

	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("context not cancelled by SIGTERM")
	}

	cause := context.Cause(ctx)
	if !errors.Is(cause, context.Canceled) {
		t.Fatalf("Cause() = %v, want it to match context.Canceled", cause)
	}
	if got := exitCode(ctx.Err(), cause); got != 143 {
		t.Fatalf("exitCode() = %d, want 143", got)
	}
}

func TestNotifyContextStopHasNoSignalCause(t *testing.T) {
	t.Parallel()
	// SIGINT, because TestNotifyContextRecordsSignal sends SIGTERM to the
	// whole test process.
	ctx, stop := notifyContext(t.Context(), os.Interrupt)
	stop()

	<-ctx.Done()
	if got := exitCode(ctx.Err(), context.Cause(ctx)); got != 130 {
		t.Fatalf("exitCode() = %d, want 130", got)
	}
}

func TestSilent(t *testing.T) {
	t.Parallel()
	processErr := exec.Command("sh", "-c", "exit 3").Run()
	var exitErr *exec.ExitError
	if !errors.As(processErr, &exitErr) {
		t.Fatalf("test setup error = %v, want *exec.ExitError", processErr)
	}

	tests := []struct {
		name  string
		err   error
		cause error
		want  bool
	}{
		{name: "ordinary error", err: errors.New("failed"), want: false},
		{name: "exit error with message", err: command.ExitError{Code: 2, Err: errors.New("bad input")}, want: false},
		{name: "exit code only", err: command.ExitError{Code: 1}, want: true},
		{name: "child process exit", err: command.ExitError{Code: 3, Err: processErr}, want: true},
		{name: "wrapped child process exit", err: command.ExitError{Code: 3, Err: fmt.Errorf("ssh: %w", processErr)}, want: true},
		{name: "selector cancelled", err: command.ExitError{Code: 130, Err: ui.ErrCancelled}, want: true},
		{name: "bare child process exit", err: processErr, want: false},
		{name: "cancelled by signal", err: fmtWrap(context.Canceled), cause: signalCause{signal: syscall.SIGTERM}, want: true},
		{name: "cancelled without signal", err: context.Canceled, cause: context.Canceled, want: false},
		{name: "signal cause without cancellation", err: errors.New("failed"), cause: signalCause{signal: syscall.SIGINT}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := silent(test.err, test.cause); got != test.want {
				t.Fatalf("silent(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

func fmtWrap(err error) error {
	return &wrappedError{err: err}
}

type wrappedError struct {
	err error
}

func (e *wrappedError) Error() string { return "wrapped: " + e.err.Error() }
func (e *wrappedError) Unwrap() error { return e.err }
