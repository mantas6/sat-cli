package main

import (
	"context"
	"errors"
	"testing"

	"github.com/mantas6/sat-cli/internal/command"
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
		{name: "exit pointer", err: &command.ExitError{Code: 13, Err: errors.New("failed")}, want: 13},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exitCode(test.err); got != test.want {
				t.Fatalf("exitCode() = %d, want %d", got, test.want)
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
