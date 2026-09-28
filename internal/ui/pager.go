package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
)

// resizeDebounce is how long the pager waits for a width change to settle
// before re-rendering the Markdown, so dragging a window edge renders once.
const resizeDebounce = 60 * time.Millisecond

// PageOptions configures the initial pager dimensions and title.
type PageOptions struct {
	Size
	Title string
}

// renderMarkdown converts Markdown to styled terminal text at the given width
// using the dark or light standard style. The style is always explicit so
// glamour never queries the terminal while bubbletea owns it.
func renderMarkdown(markdown string, width int, dark bool) (string, error) {
	if width <= 0 {
		width = defaultWidth
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
	return runPager(ctx, in, out, content, opts, renderMarkdown)
}

// runPager is Page with an injectable renderer and extra program options.
func runPager(ctx context.Context, in io.Reader, out io.Writer, content string, opts PageOptions, render markdownRenderFunc, programOpts ...tea.ProgramOption) error {
	opts.Size = initialSize(opts.Size, out)
	model, err := newPagerModel(content, opts, render)
	if err != nil {
		return err
	}
	final, err := runProgram(ctx, model, in, out, programOpts...)
	if errors.Is(err, errInterrupted) {
		return nil
	}
	if err != nil {
		return err
	}
	if pager, ok := final.(*pagerModel); ok {
		return pager.renderErr
	}
	return nil
}

type markdownRenderFunc func(markdown string, width int, dark bool) (string, error)

// renderKey identifies one rendering of the pager's (fixed) Markdown.
type renderKey struct {
	width int
	dark  bool
}

// pagerKeyMap holds the pager's own bindings. Scrolling keys belong to the
// viewport's KeyMap and are handled by viewport.Update.
type pagerKeyMap struct {
	Quit   key.Binding
	Top    key.Binding
	Bottom key.Binding

	viewport *viewport.KeyMap
}

func newPagerKeyMap(scroll *viewport.KeyMap) pagerKeyMap {
	return pagerKeyMap{
		Quit:     key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		viewport: scroll,
	}
}

// ShortHelp implements help.KeyMap.
func (k pagerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.viewport.Down, k.viewport.Up, k.viewport.PageDown, k.Top, k.Bottom, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k pagerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.viewport.Down, k.viewport.Up, k.viewport.PageDown, k.viewport.PageUp},
		{k.viewport.HalfPageDown, k.viewport.HalfPageUp, k.Top, k.Bottom},
		{k.Quit},
	}
}

// pagerRenderMsg asks the pager to re-render for the width it had when the
// message was scheduled; stale generations are dropped.
type pagerRenderMsg struct {
	generation int
}

type pagerModel struct {
	title      string
	markdown   string
	width      int
	height     int
	dark       bool
	viewport   viewport.Model
	keys       pagerKeyMap
	help       help.Model
	render     markdownRenderFunc
	rendered   map[renderKey]string
	generation int
	renderErr  error
	quitting   bool
}

func newPagerModel(markdown string, opts PageOptions, render markdownRenderFunc) (*pagerModel, error) {
	size := opts.orDefault()
	if render == nil {
		render = renderMarkdown
	}

	model := &pagerModel{
		title:    opts.Title,
		markdown: markdown,
		width:    size.Width,
		height:   size.Height,
		dark:     true,
		help:     help.New(),
		render:   render,
		rendered: make(map[renderKey]string),
	}
	model.viewport = viewport.New(viewport.WithWidth(model.width), viewport.WithHeight(model.viewportHeight()))
	model.keys = newPagerKeyMap(&model.viewport.KeyMap)
	model.help.SetWidth(max(1, model.width-footerPercentWidth))
	if err := model.rerender(); err != nil {
		return nil, err
	}
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
			m.help.Styles = help.DefaultStyles(dark)
			return m, m.rerenderOrQuit()
		}
		return m, nil
	case tea.WindowSizeMsg:
		// A zero-sized report carries no information; keep the old size.
		widthChanged := message.Width > 0 && message.Width != m.width
		if message.Width > 0 {
			m.width = message.Width
			m.viewport.SetWidth(message.Width)
			m.help.SetWidth(max(1, m.width-footerPercentWidth))
		}
		if message.Height > 0 {
			m.height = message.Height
			m.viewport.SetHeight(m.viewportHeight())
		}
		// A taller viewport lowers the maximum offset; SetYOffset clamps.
		m.viewport.SetYOffset(m.viewport.YOffset())
		if !widthChanged {
			return m, nil
		}
		m.generation++
		generation := m.generation
		return m, tea.Tick(resizeDebounce, func(time.Time) tea.Msg {
			return pagerRenderMsg{generation: generation}
		})
	case pagerRenderMsg:
		if message.generation != m.generation {
			return m, nil
		}
		return m, m.rerenderOrQuit()
	case tea.KeyPressMsg:
		switch {
		case key.Matches(message, m.keys.Quit):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(message, m.keys.Top):
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(message, m.keys.Bottom):
			m.viewport.GotoBottom()
			return m, nil
		}
	}

	var command tea.Cmd
	m.viewport, command = m.viewport.Update(message)
	return m, command
}

// rerender shows the Markdown for the current width and style, rendering it
// only the first time that combination is needed. The relative scroll
// position survives the change in line count.
func (m *pagerModel) rerender() error {
	current := renderKey{width: m.width, dark: m.dark}
	rendered, ok := m.rendered[current]
	if !ok {
		var err error
		rendered, err = m.render(m.markdown, m.width, m.dark)
		if err != nil {
			return err
		}
		m.rendered[current] = rendered
	}

	fraction := 0.0
	if !m.viewport.AtTop() {
		fraction = m.viewport.ScrollPercent()
	}
	m.viewport.SetContent(rendered)
	maxOffset := max(0, m.viewport.TotalLineCount()-m.viewport.Height())
	m.viewport.SetYOffset(int(math.Round(fraction * float64(maxOffset))))
	return nil
}

// rerenderOrQuit is rerender for Update: a failure is kept in renderErr and
// quits the program so Page can return it.
func (m *pagerModel) rerenderOrQuit() tea.Cmd {
	if err := m.rerender(); err != nil {
		m.renderErr = err
		return tea.Quit
	}
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
		parts = append(parts, titleStyle.Render(m.title))
	}
	parts = append(parts, m.viewport.View())
	parts = append(parts, m.footer())
	return strings.Join(parts, "\n")
}

// footerPercentWidth is the width of the "100%  " scroll indicator that
// precedes the key help in the footer.
const footerPercentWidth = 6

func (m *pagerModel) footer() string {
	return fmt.Sprintf("%3.0f%%  ", m.viewport.ScrollPercent()*100) + m.help.View(m.keys)
}

func (m *pagerModel) viewportHeight() int {
	chrome := 1
	if m.title != "" {
		chrome++
	}
	return max(1, m.height-chrome)
}
