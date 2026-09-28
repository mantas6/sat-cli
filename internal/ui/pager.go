package ui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
)

// PageOptions configures the initial pager dimensions and title.
type PageOptions struct {
	Title         string
	Width, Height int
}

// renderMarkdown converts Markdown to styled terminal text at the given width
// using the dark or light standard style. The style is always explicit so
// glamour never queries the terminal while bubbletea owns it.
func renderMarkdown(markdown string, width int, dark bool) (string, error) {
	if width <= 0 {
		width = 80
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(glamourStyle(dark)),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}
	return renderer.Render(markdown)
}

func glamourStyle(dark bool) string {
	if dark {
		return "dark"
	}
	return "light"
}

// Page shows Markdown in a scrollable full-screen viewport.
func Page(ctx context.Context, in io.Reader, out io.Writer, content string, opts PageOptions) error {
	model, err := newPagerModel(content, opts, renderMarkdown)
	if err != nil {
		return err
	}
	program := tea.NewProgram(
		model,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithContext(ctx),
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

type markdownRenderFunc func(markdown string, width int, dark bool) (string, error)

type pagerModel struct {
	title     string
	markdown  string
	width     int
	height    int
	dark      bool
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
		render = renderMarkdown
	}

	model := &pagerModel{
		title:    opts.Title,
		markdown: markdown,
		width:    width,
		height:   height,
		dark:     true,
		render:   render,
	}
	rendered, err := render(markdown, width, model.dark)
	if err != nil {
		return nil, err
	}
	model.viewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(model.viewportHeight()))
	model.viewport.SetContent(rendered)
	return model, nil
}

// Init asks the terminal for its background colour; the answer arrives as a
// tea.BackgroundColorMsg and selects the light or dark Markdown style.
func (m *pagerModel) Init() tea.Cmd {
	return tea.RequestBackgroundColor
}

func (m *pagerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.BackgroundColorMsg:
		if dark := message.IsDark(); dark != m.dark {
			m.dark = dark
			return m, m.rerender()
		}
		return m, nil
	case tea.WindowSizeMsg:
		widthChanged := message.Width > 0 && message.Width != m.width
		if message.Width > 0 {
			m.width = message.Width
			m.viewport.SetWidth(message.Width)
		}
		if message.Height > 0 {
			m.height = message.Height
			m.viewport.SetHeight(m.viewportHeight())
		}
		if widthChanged {
			return m, m.rerender()
		}
		return m, nil
	case tea.KeyPressMsg:
		switch message.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "j", "down":
			m.viewport.ScrollDown(1)
		case "k", "up":
			m.viewport.ScrollUp(1)
		case "pgdown", "space":
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

// rerender renders the Markdown for the current width and style, quitting
// with renderErr set when rendering fails.
func (m *pagerModel) rerender() tea.Cmd {
	rendered, err := m.render(m.markdown, m.width, m.dark)
	if err != nil {
		m.renderErr = err
		return tea.Quit
	}
	m.viewport.SetContent(rendered)
	return nil
}

func (m *pagerModel) View() tea.View {
	view := tea.NewView(m.content())
	view.AltScreen = true
	return view
}

func (m *pagerModel) content() string {
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
