package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// These tests run the real bubbletea programs on scripted input, a fixed
// window size and discarded output, so they need no terminal.

// testProgram returns program options for a hermetic run.
func testProgram() []tea.ProgramOption {
	return []tea.ProgramOption{tea.WithWindowSize(80, 24), tea.WithoutSignals()}
}

const (
	keyUp    = "\x1b[A"
	keyEnter = "\r"
	keyCtrlC = "\x03"
)

func TestRunSelectorReturnsChosenItem(t *testing.T) {
	t.Parallel()
	model := newSelectorModel(letterItems(3), SelectOptions{Title: "Letters"})

	item, err := runSelector(t.Context(), strings.NewReader(keyUp+keyEnter), io.Discard, model, testProgram()...)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "b" {
		t.Fatalf("runSelector() = %q, want b", item.ID)
	}
}

func TestRunSelectorFiltersTypedQuery(t *testing.T) {
	t.Parallel()
	model := newSelectorModel(letterItems(3), SelectOptions{})

	item, err := runSelector(t.Context(), strings.NewReader("c"+keyEnter), io.Discard, model, testProgram()...)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "c" {
		t.Fatalf("runSelector() = %q, want c", item.ID)
	}
}

func TestRunSelectorCtrlCCancels(t *testing.T) {
	t.Parallel()
	model := newSelectorModel(letterItems(3), SelectOptions{})

	_, err := runSelector(t.Context(), strings.NewReader(keyCtrlC), io.Discard, model, testProgram()...)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("runSelector() error = %v, want ErrCancelled", err)
	}
}

func TestRunSelectorReturnsContextError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	model := newSelectorModel(letterItems(3), SelectOptions{})

	_, err := runSelector(ctx, strings.NewReader(""), io.Discard, model, testProgram()...)
	if !errors.Is(err, context.Canceled) || errors.Is(err, tea.ErrProgramKilled) {
		t.Fatalf("runSelector() error = %v, want plain context.Canceled", err)
	}
}

func TestRunPagerQuits(t *testing.T) {
	t.Parallel()
	render := func(markdown string, _ int, _ bool) (string, error) { return markdown, nil }

	err := runPager(t.Context(), strings.NewReader("jjq"), io.Discard, "content", PageOptions{Title: "T"}, render, testProgram()...)
	if err != nil {
		t.Fatalf("runPager() error = %v, want nil", err)
	}
}

func TestRunPagerReturnsRenderError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("render failed")
	render := func(string, int, bool) (string, error) { return "", wantErr }

	err := runPager(t.Context(), strings.NewReader("q"), io.Discard, "content", PageOptions{}, render, testProgram()...)
	if !errors.Is(err, wantErr) {
		t.Fatalf("runPager() error = %v, want %v", err, wantErr)
	}
}

func TestRunPagerReturnsContextError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	render := func(markdown string, _ int, _ bool) (string, error) { return markdown, nil }

	err := runPager(ctx, strings.NewReader(""), io.Discard, "content", PageOptions{}, render, testProgram()...)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runPager() error = %v, want context.Canceled", err)
	}
}

func TestFollowQuitCancelsInFlightFetch(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	stopped := make(chan error, 1)
	fetch := func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		stopped <- ctx.Err()
		return "", ctx.Err()
	}
	in, write := io.Pipe()
	t.Cleanup(func() { _ = write.Close() })
	go func() {
		<-started
		_, _ = write.Write([]byte("q"))
	}()

	err := follow(t.Context(), in, io.Discard, fetch, DashboardOptions{Interval: time.Hour}, testProgram()...)
	if err != nil {
		t.Fatalf("follow() error = %v, want nil after q", err)
	}
	select {
	case err := <-stopped:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("fetch context error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight fetch was not cancelled when the dashboard quit")
	}
}

func TestFollowReturnsContextErrorWhenCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	fetches := 0
	fetch := func(context.Context) (string, error) {
		fetches++
		cancel() // e.g. SIGINT arrives while the dashboard is up
		return "dashboard", nil
	}

	err := follow(ctx, strings.NewReader(""), io.Discard, fetch, DashboardOptions{Interval: time.Hour}, testProgram()...)
	if !errors.Is(err, context.Canceled) || errors.Is(err, tea.ErrProgramKilled) {
		t.Fatalf("follow() error = %v, want plain context.Canceled", err)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}
}

// interruptModel interrupts the program as soon as it starts, as bubbletea
// does when it catches SIGINT on a non-terminal stdin.
type interruptModel struct{}

func (interruptModel) Init() tea.Cmd                         { return tea.Interrupt }
func (m interruptModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (interruptModel) View() tea.View                        { return tea.NewView("") }

func TestRunProgramMapsInterrupt(t *testing.T) {
	t.Parallel()
	_, err := runProgram(t.Context(), interruptModel{}, strings.NewReader(""), io.Discard, testProgram()...)
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("runProgram() error = %v, want errInterrupted", err)
	}
}
