package ui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// PageOptions configures the initial pager dimensions and title.
type PageOptions struct {
	Title         string
	Width, Height int
}

// RenderMarkdown converts Markdown to styled terminal text at the given width.
func RenderMarkdown(markdown string, width int) (string, error) {
	if width <= 0 {
		width = 80
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(markdown)
}

// Page shows Markdown in a scrollable full-screen viewport.
func Page(ctx context.Context, in io.Reader, out io.Writer, content string, opts PageOptions) error {
	model, err := newPagerModel(content, opts, RenderMarkdown)
	if err != nil {
		return err
	}
	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithContext(ctx),
		tea.WithAltScreen(),
	)
	final, err := program.Run()
	if err != nil {
		return err
	}
	if pager, ok := final.(*pagerModel); ok {
		return pager.renderErr
	}
	return nil
}

type markdownRenderFunc func(string, int) (string, error)

type pagerModel struct {
	title     string
	markdown  string
	width     int
	height    int
	viewport  viewport.Model
	render    markdownRenderFunc
	renderErr error
	quitting  bool
}

func newPagerModel(markdown string, opts PageOptions, render markdownRenderFunc) (*pagerModel, error) {
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	height := opts.Height
	if height <= 0 {
		height = 24
	}
	if render == nil {
		render = RenderMarkdown
	}

	rendered, err := render(markdown, width)
	if err != nil {
		return nil, err
	}
	model := &pagerModel{
		title:    opts.Title,
		markdown: markdown,
		width:    width,
		height:   height,
		render:   render,
	}
	model.viewport = viewport.New(width, model.viewportHeight())
	model.viewport.SetContent(rendered)
	return model, nil
}

func (m *pagerModel) Init() tea.Cmd {
	return nil
}

func (m *pagerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		widthChanged := message.Width > 0 && message.Width != m.width
		if message.Width > 0 {
			m.width = message.Width
			m.viewport.Width = message.Width
		}
		if message.Height > 0 {
			m.height = message.Height
			m.viewport.Height = m.viewportHeight()
		}
		if widthChanged {
			rendered, err := m.render(m.markdown, m.width)
			if err != nil {
				m.renderErr = err
				return m, tea.Quit
			}
			m.viewport.SetContent(rendered)
		}
		return m, nil
	case tea.KeyMsg:
		switch message.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "j", "down":
			m.viewport.LineDown(1)
		case "k", "up":
			m.viewport.LineUp(1)
		case "pgdown", " ":
			m.viewport.PageDown()
		case "pgup", "b":
			m.viewport.PageUp()
		case "d":
			m.viewport.HalfPageDown()
		case "u":
			m.viewport.HalfPageUp()
		case "g":
			m.viewport.GotoTop()
		case "G":
			m.viewport.GotoBottom()
		default:
			var command tea.Cmd
			m.viewport, command = m.viewport.Update(message)
			return m, command
		}
		return m, nil
	}

	var command tea.Cmd
	m.viewport, command = m.viewport.Update(message)
	return m, command
}

func (m *pagerModel) View() string {
	if m.quitting {
		return ""
	}
	parts := make([]string, 0, 3)
	if m.title != "" {
		parts = append(parts, m.title)
	}
	parts = append(parts, m.viewport.View())
	parts = append(parts, fmt.Sprintf("%3.0f%%  j/k scroll  q quit", m.viewport.ScrollPercent()*100))
	return strings.Join(parts, "\n")
}

func (m *pagerModel) viewportHeight() int {
	chrome := 1
	if m.title != "" {
		chrome++
	}
	return max(1, m.height-chrome)
}
