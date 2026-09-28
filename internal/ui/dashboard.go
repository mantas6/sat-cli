package ui

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// DefaultDashboardInterval is the refresh interval used when the caller does
// not pick one.
const DefaultDashboardInterval = 5 * time.Second

// ErrInvalidInterval indicates a dashboard refresh interval that is not
// positive.
var ErrInvalidInterval = errors.New("dashboard interval must be greater than zero")

const (
	dashboardLoading = "Loading dashboard…"
	dashboardFailed  = " Dashboard refresh failed; retrying."
)

// DashboardOptions configures the dashboard refresh interval and initial size.
type DashboardOptions struct {
	Size
	Interval time.Duration
}

// Fetcher returns the latest dashboard text.
type Fetcher func(ctx context.Context) (string, error)

// Follow runs the full-screen dashboard until the user quits or ctx is
// cancelled. A fetch still in flight when the dashboard closes has its
// context cancelled.
func Follow(ctx context.Context, in io.Reader, out io.Writer, fetch Fetcher, opts DashboardOptions) error {
	return follow(ctx, in, out, fetch, opts)
}

// follow is Follow with extra program options.
func follow(ctx context.Context, in io.Reader, out io.Writer, fetch Fetcher, opts DashboardOptions, programOpts ...tea.ProgramOption) error {
	if opts.Interval <= 0 {
		return ErrInvalidInterval
	}
	opts.Size = initialSize(opts.Size, out)

	fetchContext, cancel := context.WithCancel(ctx)
	defer cancel()
	model := newDashboardModel(fetchContext, fetch, opts)
	model.cancel = cancel

	_, err := runProgram(ctx, model, in, out, programOpts...)
	if errors.Is(err, errInterrupted) {
		return nil
	}
	return err
}

type fetchResultMsg struct {
	text string
	err  error
}

type dashboardTickMsg struct{}

type dashboardModel struct {
	ctx      context.Context
	cancel   context.CancelFunc
	fetch    Fetcher
	interval time.Duration
	width    int
	height   int
	text     string
	// lines is text split and truncated to width, rebuilt only when either
	// changes rather than on every frame.
	lines    []string
	loaded   bool
	hasError bool
	tick     func(time.Duration) tea.Cmd
}

func newDashboardModel(ctx context.Context, fetch Fetcher, opts DashboardOptions) *dashboardModel {
	size := opts.orDefault()
	return &dashboardModel{
		ctx:      ctx,
		cancel:   func() {},
		fetch:    fetch,
		interval: opts.Interval,
		width:    size.Width,
		height:   size.Height,
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
	case tea.KeyPressMsg:
		switch message.String() {
		case "q", "esc", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		if message.Width > 0 && message.Width != m.width {
			m.width = message.Width
			m.lines = dashboardLines(m.text, m.width)
		}
		if message.Height > 0 {
			m.height = message.Height
		}
	case dashboardTickMsg:
		return m, m.fetchDashboard()
	case fetchResultMsg:
		m.loaded = true
		if message.err != nil {
			m.hasError = true
		} else {
			m.hasError = false
			if message.text != m.text {
				m.text = message.text
				m.lines = dashboardLines(m.text, m.width)
			}
		}
		return m, m.tick(m.interval)
	}
	return m, nil
}

func (m *dashboardModel) View() tea.View {
	view := tea.NewView(m.content())
	view.AltScreen = true
	return view
}

func (m *dashboardModel) content() string {
	if !m.loaded {
		return ansi.Truncate(dashboardLoading, m.width, "")
	}

	lineLimit := m.height
	if m.hasError {
		lineLimit--
	}
	lines := m.lines[:min(len(m.lines), max(0, lineLimit))]
	if m.hasError && m.height > 0 {
		badge := errorBadgeStyle.Render(" <!> ")
		lines = append(lines[:len(lines):len(lines)], ansi.Truncate(badge+dashboardFailed, m.width, ""))
	}
	return strings.Join(lines, "\n")
}

// fetchDashboard returns a command that fetches once. It captures the
// context and fetcher so the command never reads the model concurrently
// with Update.
func (m *dashboardModel) fetchDashboard() tea.Cmd {
	ctx, fetch := m.ctx, m.fetch
	return func() tea.Msg {
		text, err := fetch(ctx)
		return fetchResultMsg{text: text, err: err}
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
