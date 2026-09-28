// Package ui contains reusable terminal user interfaces.
package ui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// Item is a selectable value. ID is not displayed or searched unless it is
// also included explicitly in Columns.
type Item struct {
	ID      string
	Columns []string
}

// SelectOptions configures a selector.
type SelectOptions struct {
	Size
	Title string
	Query string
}

var (
	// ErrCancelled indicates that the user closed a selector without
	// choosing.
	ErrCancelled = errors.New("selection cancelled")
	// ErrNoItems indicates that there was nothing to select from.
	ErrNoItems = errors.New("nothing to select")
	// ErrNoMatch indicates that the initial query matched no item. The
	// returned error wraps it together with the query.
	ErrNoMatch = errors.New("no items match")
	// ErrNeedsTerminal indicates that the choice needs the interactive
	// selector but stdin or stdout is not a terminal.
	ErrNeedsTerminal = errors.New("interactive selection requires a terminal on stdin and stdout")
)

// ellipsis marks a column cut short to fit the terminal width.
const ellipsis = "…"

// Select returns the chosen item. It fails with ErrNoItems for an empty list
// and with ErrNoMatch when opts.Query matches nothing, and returns the item
// straight away when the query narrows the list to one entry, so query-only
// invocations work without a terminal. Otherwise it runs the interactive
// selector, which needs a terminal on in and out (ErrNeedsTerminal).
//
// The selector uses a bottom-up (fzf-style) layout: rows render above the
// filter prompt with index 0 nearest the prompt, so "up" moves towards later
// items and "down" moves towards the best match.
func Select(ctx context.Context, in io.Reader, out io.Writer, items []Item, opts SelectOptions) (Item, error) {
	list := newCatalog(items)
	matches, err := list.resolve(opts.Query)
	if err != nil {
		return Item{}, err
	}
	if opts.Query != "" && len(matches) == 1 {
		return items[matches[0]], nil
	}
	if !isTerminal(in) || !isTerminal(out) {
		return Item{}, ErrNeedsTerminal
	}
	opts.Size = initialSize(opts.Size, out)
	return runSelector(ctx, in, out, newSelectorModelFrom(list, matches, opts))
}

// runSelector runs model as a bubbletea program and returns its choice.
func runSelector(ctx context.Context, in io.Reader, out io.Writer, model *selectorModel, programOpts ...tea.ProgramOption) (Item, error) {
	final, err := runProgram(ctx, model, in, out, programOpts...)
	if errors.Is(err, errInterrupted) {
		return Item{}, ErrCancelled
	}
	if err != nil {
		return Item{}, err
	}
	result, ok := final.(*selectorModel)
	if !ok {
		return Item{}, ErrCancelled
	}
	return result.selection()
}

// catalog holds the items with the labels the fuzzy filter searches, so the
// labels are joined once rather than on every keystroke.
type catalog struct {
	items  []Item
	labels []string
}

func newCatalog(items []Item) *catalog {
	labels := make([]string, len(items))
	for index, item := range items {
		labels[index] = strings.Join(item.Columns, " ")
	}
	return &catalog{items: items, labels: labels}
}

// resolve filters for query and reports ErrNoItems or ErrNoMatch.
func (c *catalog) resolve(query string) ([]int, error) {
	if len(c.items) == 0 {
		return nil, ErrNoItems
	}
	matches := c.filter(query, nil)
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w %q", ErrNoMatch, query)
	}
	return matches, nil
}

// filter fuzzy-matches query against the item labels and returns the indices
// of the matches ranked by score, keeping input order for equal scores. When
// within is non-nil only those indices, in ascending order, are searched.
func (c *catalog) filter(query string, within []int) []int {
	if within == nil {
		within = make([]int, len(c.items))
		for index := range within {
			within[index] = index
		}
	}
	if query == "" {
		return within
	}

	labels := make([]string, len(within))
	for position, index := range within {
		labels[position] = c.labels[index]
	}
	found := fuzzy.FindNoSort(query, labels)
	slices.SortStableFunc(found, func(a, b fuzzy.Match) int {
		return cmp.Compare(b.Score, a.Score)
	})

	matches := make([]int, len(found))
	for position, match := range found {
		matches[position] = within[match.Index]
	}
	return matches
}

// selectorKeyMap holds the selector's bindings; any other key edits the
// filter input.
type selectorKeyMap struct {
	Choose   key.Binding
	Cancel   key.Binding
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
}

var selectorKeys = selectorKeyMap{
	Choose:   key.NewBinding(key.WithKeys("enter")),
	Cancel:   key.NewBinding(key.WithKeys("esc", "ctrl+c")),
	Up:       key.NewBinding(key.WithKeys("up", "ctrl+p")),
	Down:     key.NewBinding(key.WithKeys("down", "ctrl+n")),
	PageUp:   key.NewBinding(key.WithKeys("pgup")),
	PageDown: key.NewBinding(key.WithKeys("pgdown")),
}

var (
	cursorMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	cursorRowStyle    = lipgloss.NewStyle().Bold(true)
)

// selectorModel renders a bottom-up selector: visible rows are drawn above the
// filter input, with index 0 immediately above the prompt and higher indices
// stacking upwards. cursor and offset index matches; only the View and the
// navigation key directions are inverted to match the layout.
type selectorModel struct {
	title   string
	catalog *catalog
	// matches are the catalog indices matching query, best first.
	matches []int
	query   string
	// widths are the column widths for all matches at the current width.
	widths    []int
	input     textinput.Model
	cursor    int
	offset    int
	width     int
	height    int
	selected  Item
	chosen    bool
	cancelled bool
	// done is set once the user chose or cancelled; the view is then
	// empty so the inline selector leaves no trace in the terminal.
	done bool
}

func newSelectorModel(items []Item, opts SelectOptions) *selectorModel {
	list := newCatalog(items)
	return newSelectorModelFrom(list, list.filter(opts.Query, nil), opts)
}

// newSelectorModelFrom builds the model from an already filtered catalog so
// the initial query is not matched again.
func newSelectorModelFrom(list *catalog, matches []int, opts SelectOptions) *selectorModel {
	size := opts.orDefault()
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Filter"
	input.SetValue(opts.Query)
	input.CursorEnd()
	input.Focus()
	input.SetWidth(max(1, size.Width-2))

	model := &selectorModel{
		title:   opts.Title,
		catalog: list,
		matches: matches,
		query:   opts.Query,
		input:   input,
		width:   size.Width,
		height:  size.Height,
	}
	model.updateWidths()
	return model
}

func (m *selectorModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *selectorModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		// A zero-sized report (for example from a pty without dimensions)
		// carries no information, so keep the previous or default size.
		if message.Width > 0 && message.Width != m.width {
			m.width = message.Width
			m.input.SetWidth(max(1, m.width-2))
			m.updateWidths()
		}
		if message.Height > 0 {
			m.height = message.Height
		}
		m.keepCursorVisible()
		return m, nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(message, selectorKeys.Cancel):
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		case key.Matches(message, selectorKeys.Choose):
			if len(m.matches) > 0 {
				m.selected = m.catalog.items[m.matches[m.cursor]]
				m.chosen = true
				m.done = true
				return m, tea.Quit
			}
			return m, nil
		case key.Matches(message, selectorKeys.Up):
			m.move(1)
			return m, nil
		case key.Matches(message, selectorKeys.Down):
			m.move(-1)
			return m, nil
		case key.Matches(message, selectorKeys.PageUp):
			m.movePage(1)
			return m, nil
		case key.Matches(message, selectorKeys.PageDown):
			m.movePage(-1)
			return m, nil
		}
	}

	var command tea.Cmd
	m.input, command = m.input.Update(message)
	if query := m.input.Value(); query != m.query {
		m.applyQuery(query)
	}
	return m, command
}

// applyQuery refilters for query. A query that extends the previous one can
// only match a subset of the previous matches, so only those are searched.
func (m *selectorModel) applyQuery(query string) {
	var within []int
	if m.query != "" && strings.HasPrefix(query, m.query) {
		within = slices.Clone(m.matches)
		slices.Sort(within)
	}
	m.matches = m.catalog.filter(query, within)
	m.query = query
	m.cursor = 0
	m.offset = 0
	m.updateWidths()
}

// updateWidths recomputes the column widths over every match, not only the
// visible rows, so columns stay aligned while scrolling.
func (m *selectorModel) updateWidths() {
	items := make([]Item, len(m.matches))
	for position, index := range m.matches {
		items[position] = m.catalog.items[index]
	}
	m.widths = columnWidths(items, max(1, m.width-2))
}

func (m *selectorModel) View() tea.View {
	return tea.NewView(m.content())
}

func (m *selectorModel) content() string {
	if m.done {
		return ""
	}

	var lines []string
	if len(m.matches) == 0 {
		lines = append(lines, "No matches.")
	} else {
		// Render rows bottom-up: the highest visible index sits at the top and
		// index 0 (the best match) lands directly above the prompt.
		end := min(len(m.matches), m.offset+m.visibleRows())
		for index := end - 1; index >= m.offset; index-- {
			row := renderColumns(m.catalog.items[m.matches[index]].Columns, m.widths)
			if index == m.cursor {
				lines = append(lines, cursorMarkerStyle.Render(">")+" "+cursorRowStyle.Render(row))
			} else {
				lines = append(lines, "  "+row)
			}
		}
	}

	if m.title != "" {
		lines = append(lines, titleStyle.Render(m.title))
	}
	lines = append(lines, m.input.View())
	return strings.Join(lines, "\n") + "\n"
}

func (m *selectorModel) move(delta int) {
	if len(m.matches) == 0 {
		return
	}
	m.cursor = (m.cursor + delta + len(m.matches)) % len(m.matches)
	m.keepCursorVisible()
}

func (m *selectorModel) movePage(direction int) {
	if len(m.matches) == 0 {
		return
	}
	m.cursor = max(0, min(len(m.matches)-1, m.cursor+direction*m.visibleRows()))
	m.keepCursorVisible()
}

func (m *selectorModel) visibleRows() int {
	return max(1, m.height-3)
}

func (m *selectorModel) keepCursorVisible() {
	rows := m.visibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.matches)-rows)))
}

func (m *selectorModel) selection() (Item, error) {
	if m.cancelled || !m.chosen {
		return Item{}, ErrCancelled
	}
	return m.selected, nil
}

// columnWidths returns the width of each column so that every row fits in
// available cells with two-space separators, shrinking the widest column
// first.
func columnWidths(items []Item, available int) []int {
	columnCount := 0
	for _, item := range items {
		columnCount = max(columnCount, len(item.Columns))
	}
	if columnCount == 0 {
		return nil
	}

	widths := make([]int, columnCount)
	for _, item := range items {
		for index, column := range item.Columns {
			widths[index] = max(widths[index], lipgloss.Width(column))
		}
	}
	for index := range widths {
		widths[index] = max(1, widths[index])
	}

	separators := 2 * (columnCount - 1)
	for sum(widths)+separators > available {
		longest := 0
		for index := 1; index < len(widths); index++ {
			if widths[index] > widths[longest] {
				longest = index
			}
		}
		if widths[longest] <= 1 {
			break
		}
		widths[longest]--
	}
	return widths
}

func renderColumns(columns []string, widths []int) string {
	rendered := make([]string, 0, len(columns))
	for index, column := range columns {
		if index >= len(widths) {
			break
		}
		value := ansi.Truncate(column, widths[index], ellipsis)
		if index < len(columns)-1 {
			value += strings.Repeat(" ", max(0, widths[index]-lipgloss.Width(value)))
		}
		rendered = append(rendered, value)
	}
	return strings.Join(rendered, "  ")
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}
