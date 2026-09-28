package command

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func skipWithoutSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh and unix signals")
	}
}

// readyWriter collects child output and closes ready on the first write, so a
// test can wait until the child has installed its signal traps.
type readyWriter struct {
	mu    sync.Mutex
	buf   strings.Builder
	once  sync.Once
	ready chan struct{}
}

func newReadyWriter() *readyWriter {
	return &readyWriter{ready: make(chan struct{})}
}

func (w *readyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.once.Do(func() { close(w.ready) })
	return w.buf.Write(p)
}

func (w *readyWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// runUntilReady starts runner with an sh script, cancels ctx once the script
// has written its first output, and returns how long Run took after the
// cancellation and its error.
func runUntilReady(t *testing.T, runner ExecRunner, script string, stdout *readyWriter) (time.Duration, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	result := make(chan error, 1)
	go func() {
		result <- runner.Run(ctx, "sh", []string{"-c", script}, nil, stdout, io.Discard)
	}()

	select {
	case <-stdout.ready:
	case err := <-result:
		t.Fatalf("Run() returned %v before the child was ready", err)
	case <-time.After(5 * time.Second):
		t.Fatal("child never became ready")
	}
	cancelled := time.Now()
	cancel()

	select {
	case err := <-result:
		return time.Since(cancelled), err
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
		return 0, nil
	}
}

func TestExecRunnerAttachesStreams(t *testing.T) {
	t.Parallel()
	skipWithoutSh(t)
	var stdout, stderr strings.Builder
	err := ExecRunner{}.Run(t.Context(), "sh", []string{"-c", "cat; echo err >&2"}, strings.NewReader("in\n"), &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "in\n" || stderr.String() != "err\n" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestExecRunnerReturnsChildExitError(t *testing.T) {
	t.Parallel()
	skipWithoutSh(t)
	err := ExecRunner{}.Run(t.Context(), "sh", []string{"-c", "exit 7"}, nil, io.Discard, io.Discard)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("Run() error = %v, want *exec.ExitError with code 7", err)
	}
}

func TestExecRunnerCancelledContextSkipsStart(t *testing.T) {
	t.Parallel()
	skipWithoutSh(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout strings.Builder
	err := ExecRunner{}.Run(ctx, "sh", []string{"-c", "echo started"}, nil, &stdout, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want the child not started", stdout.String())
	}
}

func TestExecRunnerSendsSIGTERMOnCancel(t *testing.T) {
	t.Parallel()
	skipWithoutSh(t)
	// The trap exits cleanly, so Run must still report the cancellation
	// rather than the child's zero exit status. The long grace proves the
	// child stopped because of SIGTERM, not the kill.
	const script = `trap 'kill $! 2>/dev/null; echo term; exit 0' TERM; echo ready; sleep 10 & wait`
	stdout := newReadyWriter()
	elapsed, err := runUntilReady(t, ExecRunner{Grace: 10 * time.Second}, script, stdout)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if got := stdout.String(); got != "ready\nterm\n" {
		t.Fatalf("stdout = %q, want the SIGTERM trap to run", got)
	}
	if elapsed >= 5*time.Second {
		t.Fatalf("Run() took %v after cancel, want the child to exit on SIGTERM", elapsed)
	}
}

func TestExecRunnerKillsAfterGrace(t *testing.T) {
	t.Parallel()
	skipWithoutSh(t)
	// An ignored signal disposition survives exec, so sleep ignores SIGTERM
	// and only the kill after the grace period stops it.
	const script = `trap '' TERM; echo ready; exec sleep 10`
	const grace = 150 * time.Millisecond
	elapsed, err := runUntilReady(t, ExecRunner{Grace: grace}, script, newReadyWriter())

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if elapsed < grace {
		t.Fatalf("Run() returned %v after cancel, want at least the %v grace", elapsed, grace)
	}
}

func TestChildExitError(t *testing.T) {
	t.Parallel()
	skipWithoutSh(t)
	exited := exec.Command("sh", "-c", "exit 4").Run()
	signalled := exec.Command("sh", "-c", "kill -TERM $$").Run()
	plain := errors.New("start failed")

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantExit bool
	}{
		{name: "exit status", err: exited, wantCode: 4, wantExit: true},
		{name: "signal", err: signalled, wantCode: 128 + int(syscall.SIGTERM), wantExit: true},
		{name: "cancellation", err: context.Canceled},
		{name: "start error", err: plain},
		{name: "nil", err: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := childExitError(test.err)
			var exitErr ExitError
			isExit := errors.As(got, &exitErr)
			if isExit != test.wantExit {
				t.Fatalf("childExitError(%v) = %#v, want ExitError = %v", test.err, got, test.wantExit)
			}
			if !test.wantExit {
				if got != test.err {
					t.Fatalf("childExitError(%v) = %v, want the error unchanged", test.err, got)
				}
				return
			}
			if exitErr.Code != test.wantCode || !errors.Is(got, test.err) {
				t.Fatalf("childExitError() = %#v, want code %d wrapping %v", got, test.wantCode, test.err)
			}
		})
	}
}
