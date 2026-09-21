package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestDashboardModelFetchesImmediatelyAndSchedulesInterval(t *testing.T) {
	fetched := false
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		fetched = true
		return "first", nil
	}, DashboardOptions{Interval: 7 * time.Second})

	message, ok := model.Init()().(fetchResultMsg)
	if !ok || !fetched || message.Text != "first" || message.Err != nil {
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
}

func TestDashboardModelUpdatesOnlyWhenVisibleStateChanges(t *testing.T) {
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		return "", nil
	}, DashboardOptions{Interval: time.Second})
	model.tick = noDashboardTick

	model.Update(fetchResultMsg{Text: "one"})
	first := model.View()
	if first != "one" {
		t.Fatalf("View() = %q, want one", first)
	}

	model.Update(fetchResultMsg{Text: "one"})
	if got := model.View(); got != first {
		t.Fatalf("unchanged response changed View() from %q to %q", first, got)
	}

	model.Update(fetchResultMsg{Text: "two"})
	if got := model.View(); got != "two" {
		t.Fatalf("changed response View() = %q, want two", got)
	}
}

func TestDashboardModelKeepsContentAcrossFailureAndClearsBadgeOnRecovery(t *testing.T) {
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		return "", nil
	}, DashboardOptions{Interval: time.Second})
	model.tick = noDashboardTick
	model.Update(fetchResultMsg{Text: "last good"})

	model.Update(fetchResultMsg{Err: errors.New("secret token")})
	failed := model.View()
	if !strings.Contains(failed, "last good") || !strings.Contains(failed, "<!>") {
		t.Fatalf("failure View() = %q, want last content and badge", failed)
	}
	if strings.Contains(failed, "secret token") {
		t.Fatalf("failure View() exposed error details: %q", failed)
	}

	model.Update(fetchResultMsg{Text: "recovered"})
	recovered := model.View()
	if recovered != "recovered" || strings.Contains(recovered, "<!>") {
		t.Fatalf("recovery View() = %q, want recovered content without badge", recovered)
	}
}

func TestDashboardModelTruncatesWidthAndHeight(t *testing.T) {
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		return "", nil
	}, DashboardOptions{Interval: time.Second, Width: 5, Height: 2})
	model.tick = noDashboardTick
	model.Update(fetchResultMsg{Text: "123456789\nabcdef\nthird"})

	lines := strings.Split(model.View(), "\n")
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
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		return "", nil
	}, DashboardOptions{Interval: time.Second, Width: 40, Height: 2})
	model.tick = noDashboardTick
	model.Update(fetchResultMsg{Text: "first\nsecond"})
	model.Update(fetchResultMsg{Err: errors.New("temporary")})

	lines := strings.Split(model.View(), "\n")
	if len(lines) != 2 || lines[0] != "first" || !strings.Contains(lines[1], "<!>") {
		t.Fatalf("View() lines = %#v, want one content line and badge", lines)
	}
}

func TestDashboardModelIgnoresZeroSizeAndQuits(t *testing.T) {
	model := newDashboardModel(context.Background(), func(context.Context) (string, error) {
		return "", nil
	}, DashboardOptions{Interval: time.Second, Width: 20, Height: 10})
	model.Update(tea.WindowSizeMsg{})
	if model.width != 20 || model.height != 10 {
		t.Fatalf("size = %dx%d, want 20x10", model.width, model.height)
	}

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
	} {
		_, command := model.Update(key)
		if command == nil {
			t.Fatalf("key %q did not return a quit command", key.String())
		}
		if _, ok := command().(tea.QuitMsg); !ok {
			t.Fatalf("key %q command returned %T, want tea.QuitMsg", key.String(), command())
		}
	}
}

func noDashboardTick(time.Duration) tea.Cmd {
	return func() tea.Msg { return dashboardTickMsg{} }
}
