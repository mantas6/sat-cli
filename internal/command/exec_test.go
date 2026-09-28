package command

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecRunnerAttachesStreams(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	var stdout, stderr strings.Builder
	err := ExecRunner{}.Run(t.Context(), "sh", []string{"-c", "cat; echo err >&2"}, strings.NewReader("in\n"), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "in\n" || stderr.String() != "err\n" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestExecRunnerForwardsCancellationAsSignal(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("signal forwarding is unix-specific")
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- ExecRunner{Grace: 100 * time.Millisecond}.Run(ctx, "sleep", []string{"10"}, nil, io.Discard, io.Discard)
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
