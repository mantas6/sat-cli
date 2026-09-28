package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func newTestDashboard(opts DashboardOptions) *dashboardModel {
	if opts.Interval == 0 {
		opts.Interval = time.Second
	}
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		return "", nil
	}, opts)
	model.tick = noDashboardTick
	return model
}

func noDashboardTick(time.Duration) tea.Cmd {
	return func() tea.Msg { return dashboardTickMsg{} }
}

func TestDashboardModelFetchesImmediatelyAndSchedulesInterval(t *testing.T) {
	t.Parallel()
	fetched := false
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		fetched = true
		return "first", nil
	}, DashboardOptions{Interval: 7 * time.Second})

	message, ok := model.Init()().(fetchResultMsg)
	if !ok || !fetched || message.text != "first" || message.err != nil {
		t.Fatalf("Init() message = %#v, fetched = %v", message, fetched)
	}

	var gotInterval time.Duration
	model.tick = func(interval time.Duration) tea.Cmd {
		gotInterval = interval
		return func() tea.Msg { return dashboardTickMsg{} }
	}
	_, command := model.Update(message)
	if gotInterval != 7*time.Second {
		t.Fatalf("scheduled interval = %v, want 7s", gotInterval)
	}
	if _, ok := command().(dashboardTickMsg); !ok {
		t.Fatalf("scheduled command returned %T, want dashboardTickMsg", command())
	}
	if _, command := model.Update(dashboardTickMsg{}); command == nil {
		t.Fatal("tick did not schedule a fetch")
	} else if _, ok := command().(fetchResultMsg); !ok {
		t.Fatalf("tick command returned %T, want fetchResultMsg", command())
	}
}

func TestDashboardModelShowsLoadingLineBeforeFirstFetch(t *testing.T) {
	t.Parallel()
	model := newTestDashboard(DashboardOptions{Size: Size{Width: 40, Height: 5}})
	if got := model.View().Content; got != dashboardLoading {
		t.Fatalf("View() before first fetch = %q, want %q", got, dashboardLoading)
	}
	if !model.View().AltScreen {
		t.Fatal("dashboard view must use the alternate screen")
	}

	model.Update(fetchResultMsg{text: "ready"})
	if got := model.View().Content; got != "ready" {
		t.Fatalf("View() after fetch = %q, want ready", got)
	}
}

func TestDashboardModelRebuildsLinesOnlyWhenTextOrWidthChanges(t *testing.T) {
	t.Parallel()
	model := newTestDashboard(DashboardOptions{Size: Size{Width: 40, Height: 5}})

	model.Update(fetchResultMsg{text: "one\ntwo"})
	first := model.lines
	model.Update(fetchResultMsg{text: "one\ntwo"})
	model.Update(tea.WindowSizeMsg{Width: 40, Height: 3})
	if &model.lines[0] != &first[0] {
		t.Fatal("unchanged text and width rebuilt the cached lines")
	}

	model.Update(tea.WindowSizeMsg{Width: 2, Height: 3})
	if got := model.View().Content; got != "on\ntw" {
		t.Fatalf("View() after narrowing = %q, want lines re-truncated to 2 cells", got)
	}
	model.Update(fetchResultMsg{text: "three"})
	if got := model.View().Content; got != "th" {
		t.Fatalf("View() after new text = %q, want th", got)
	}
}

func TestDashboardModelKeepsContentAcrossFailureAndClearsBadgeOnRecovery(t *testing.T) {
	t.Parallel()
	model := newTestDashboard(DashboardOptions{})
	model.Update(fetchResultMsg{text: "last good"})

	model.Update(fetchResultMsg{err: errors.New("secret token")})
	failed := model.View().Content
	if !strings.Contains(failed, "last good") || !strings.Contains(failed, "<!>") {
		t.Fatalf("failure View() = %q, want last content and badge", failed)
	}
	if strings.Contains(failed, "secret token") {
		t.Fatalf("failure View() exposed error details: %q", failed)
	}

	model.Update(fetchResultMsg{text: "recovered"})
	recovered := model.View().Content
	if recovered != "recovered" || strings.Contains(recovered, "<!>") {
		t.Fatalf("recovery View() = %q, want recovered content without badge", recovered)
	}
}

func TestDashboardModelFailureBeforeFirstSuccessShowsBadge(t *testing.T) {
	t.Parallel()
	model := newTestDashboard(DashboardOptions{Size: Size{Width: 60, Height: 3}})
	model.Update(fetchResultMsg{err: errors.New("down")})

	if got := ansi.Strip(model.View().Content); got != " <!> "+dashboardFailed {
		t.Fatalf("View() = %q, want only the failure badge", got)
	}
}

func TestDashboardModelTruncatesWidthAndHeight(t *testing.T) {
	t.Parallel()
	model := newTestDashboard(DashboardOptions{Size: Size{Width: 5, Height: 2}})
	model.Update(fetchResultMsg{text: "123456789\nabcdef\nthird"})

	lines := strings.Split(model.View().Content, "\n")
	if len(lines) != 2 || lines[0] != "12345" || lines[1] != "abcde" {
		t.Fatalf("View() lines = %#v, want two width-5 lines", lines)
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 5 {
			t.Fatalf("line width = %d, want <= 5: %q", lipgloss.Width(line), line)
		}
	}
}

func TestDashboardModelFailureBadgeTakesLastLine(t *testing.T) {
	t.Parallel()
	model := newTestDashboard(DashboardOptions{Size: Size{Width: 40, Height: 2}})
	model.Update(fetchResultMsg{text: "first\nsecond"})
	model.Update(fetchResultMsg{err: errors.New("temporary")})

	lines := strings.Split(model.View().Content, "\n")
	if len(lines) != 2 || lines[0] != "first" || !strings.Contains(lines[1], "<!>") {
		t.Fatalf("View() lines = %#v, want one content line and badge", lines)
	}
	// The badge must not overwrite the cached content.
	model.Update(fetchResultMsg{text: "first\nsecond"})
	if got := model.View().Content; got != "first\nsecond" {
		t.Fatalf("View() after recovery = %q, want both content lines", got)
	}
}

func TestDashboardModelIgnoresZeroSizeAndQuitsCancellingFetches(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"q", "esc", "ctrl+c"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			model := newTestDashboard(DashboardOptions{Size: Size{Width: 20, Height: 10}})
			cancelled := false
			model.cancel = func() { cancelled = true }

			model.Update(tea.WindowSizeMsg{})
			if model.width != 20 || model.height != 10 {
				t.Fatalf("size = %dx%d, want 20x10", model.width, model.height)
			}

			_, command := model.Update(keyMsg(name))
			if command == nil {
				t.Fatalf("key %q did not return a quit command", name)
			}
			if _, ok := command().(tea.QuitMsg); !ok {
				t.Fatalf("key %q command returned %T, want tea.QuitMsg", name, command())
			}
			if !cancelled {
				t.Fatalf("key %q did not cancel in-flight fetches", name)
			}
		})
	}
}

func TestFollowRejectsNonPositiveInterval(t *testing.T) {
	t.Parallel()
	fetch := func(context.Context) (string, error) {
		t.Error("fetch called for an invalid interval")
		return "", nil
	}
	for _, interval := range []time.Duration{0, -time.Second} {
		err := Follow(t.Context(), strings.NewReader(""), nil, fetch, DashboardOptions{Interval: interval})
		if !errors.Is(err, ErrInvalidInterval) {
			t.Fatalf("Follow(interval %v) error = %v, want ErrInvalidInterval", interval, err)
		}
	}
}
