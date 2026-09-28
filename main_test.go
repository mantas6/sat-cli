package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/mantas6/sat-cli/internal/command"
	"github.com/mantas6/sat-cli/internal/ui"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "ordinary error", err: errors.New("failed"), want: 1},
		{name: "cancellation", err: fmtWrap(context.Canceled), want: 130},
		{name: "exit value", err: command.ExitError{Code: 12, Err: errors.New("failed")}, want: 12},
		{name: "wrapped exit value", err: fmt.Errorf("context: %w", command.ExitError{Code: 13}), want: 13},
		{name: "zero exit code", err: command.ExitError{Err: errors.New("failed")}, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exitCode(test.err); got != test.want {
				t.Fatalf("exitCode() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestSilent(t *testing.T) {
	processErr := exec.Command("sh", "-c", "exit 3").Run()
	var exitErr *exec.ExitError
	if !errors.As(processErr, &exitErr) {
		t.Fatalf("test setup error = %v, want *exec.ExitError", processErr)
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "ordinary error", err: errors.New("failed"), want: false},
		{name: "exit error with message", err: command.ExitError{Code: 2, Err: errors.New("bad input")}, want: false},
		{name: "exit code only", err: command.ExitError{Code: 1}, want: true},
		{name: "child process exit", err: command.ExitError{Code: 3, Err: processErr}, want: true},
		{name: "wrapped child process exit", err: command.ExitError{Code: 3, Err: fmt.Errorf("ssh: %w", processErr)}, want: true},
		{name: "selector cancelled", err: command.ExitError{Code: 130, Err: ui.ErrCancelled}, want: true},
		{name: "bare child process exit", err: processErr, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := silent(test.err); got != test.want {
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
