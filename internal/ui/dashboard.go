package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	defaultDashboardWidth  = 80
	defaultDashboardHeight = 24
)

// DashboardOptions configures the dashboard refresh interval and initial size.
type DashboardOptions struct {
	Interval      time.Duration
	Width, Height int
}

// Fetcher returns the latest dashboard text.
type Fetcher func(ctx context.Context) (string, error)

// Follow runs the full-screen dashboard until the user quits or ctx is
// cancelled.
func Follow(ctx context.Context, in io.Reader, out io.Writer, fetch Fetcher, opts DashboardOptions) error {
	model := newDashboardModel(ctx, fetch, opts)
	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithContext(ctx),
		tea.WithAltScreen(),
	)
	_, err := program.Run()
	if errors.Is(err, tea.ErrProgramKilled) && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

type fetchResultMsg struct {
	Text string
	Err  error
}

type dashboardTickMsg struct{}

type dashboardModel struct {
	ctx      context.Context
	fetch    Fetcher
	interval time.Duration
	width    int
	height   int
	text     string
	hasError bool
	tick     func(time.Duration) tea.Cmd
}

func newDashboardModel(ctx context.Context, fetch Fetcher, opts DashboardOptions) *dashboardModel {
	width := opts.Width
	if width <= 0 {
		width = defaultDashboardWidth
	}
	height := opts.Height
	if height <= 0 {
		height = defaultDashboardHeight
	}

	return &dashboardModel{
		ctx:      ctx,
		fetch:    fetch,
		interval: opts.Interval,
		width:    width,
		height:   height,
		tick: func(interval time.Duration) tea.Cmd {
			return tea.Tick(interval, func(time.Time) tea.Msg {
				return dashboardTickMsg{}
			})
		},
	}
}

func (m *dashboardModel) Init() tea.Cmd {
	return m.fetchDashboard()
}

func (m *dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		if message.Width > 0 {
			m.width = message.Width
		}
		if message.Height > 0 {
			m.height = message.Height
		}
	case dashboardTickMsg:
		return m, m.fetchDashboard()
	case fetchResultMsg:
		if message.Err != nil {
			m.hasError = true
		} else {
			m.text = message.Text
			m.hasError = false
		}
		return m, m.tick(m.interval)
	}
	return m, nil
}

func (m *dashboardModel) View() string {
	lineLimit := m.height
	if m.hasError {
		lineLimit--
	}
	lineLimit = max(0, lineLimit)

	lines := dashboardLines(m.text, m.width)
	if len(lines) > lineLimit {
		lines = lines[:lineLimit]
	}
	if m.hasError && m.height > 0 {
		badge := lipgloss.NewStyle().
			Background(lipgloss.Color("1")).
			Foreground(lipgloss.Color("15")).
			Render(" <!> ")
		lines = append(lines, ansi.Truncate(badge+" Dashboard refresh failed; retrying.", m.width, ""))
	}
	return strings.Join(lines, "\n")
}

func (m *dashboardModel) fetchDashboard() tea.Cmd {
	return func() tea.Msg {
		text, err := m.fetch(m.ctx)
		return fetchResultMsg{Text: text, Err: err}
	}
}

func dashboardLines(text string, width int) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = ansi.Truncate(strings.TrimSuffix(line, "\r"), width, "")
	}
	return lines
}
