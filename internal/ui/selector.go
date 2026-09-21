// Package ui contains reusable terminal user interfaces.
package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	Title  string
	Query  string
	Width  int
	Height int
}

// ErrCancelled indicates that the user closed a selector without choosing.
var ErrCancelled = errors.New("selection cancelled")

// Select runs the interactive selector on the given terminal streams and
// returns the chosen item.
func Select(ctx context.Context, in io.Reader, out io.Writer, items []Item, opts SelectOptions) (Item, error) {
	item, done, err := Resolve(items, opts.Query)
	if err != nil || done {
		return item, err
	}

	model := newSelectorModel(items, opts)
	program := tea.NewProgram(model, tea.WithInput(in), tea.WithOutput(out), tea.WithContext(ctx))
	final, err := program.Run()
	if err != nil {
		return Item{}, err
	}

	result, ok := final.(*selectorModel)
	if !ok {
		return Item{}, ErrCancelled
	}
	return result.selection()
}

// Resolve applies the non-interactive part of a selection: it fails for an
// empty list or a query with no matches, and returns done=true with the single
// item when the query narrows the list to exactly one entry. Callers can run it
// before checking for a terminal so query-only invocations work when piped.
func Resolve(items []Item, query string) (Item, bool, error) {
	if len(items) == 0 {
		return Item{}, false, errors.New("nothing to select")
	}
	if query == "" {
		return Item{}, false, nil
	}

	filtered := filterItems(items, query)
	switch len(filtered) {
	case 0:
		return Item{}, false, fmt.Errorf("no items match %q", query)
	case 1:
		return filtered[0], true, nil
	default:
		return Item{}, false, nil
	}
}

// filterItems fuzzy-matches query against the joined display columns. Results
// are ranked by score, retaining input order for equal scores.
func filterItems(items []Item, query string) []Item {
	if query == "" {
		return append([]Item(nil), items...)
	}

	labels := make([]string, len(items))
	for index, item := range items {
		labels[index] = strings.Join(item.Columns, " ")
	}
	matches := fuzzy.FindNoSort(query, labels)
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	filtered := make([]Item, len(matches))
	for index, match := range matches {
		filtered[index] = items[match.Index]
	}
	return filtered
}

type selectorModel struct {
	title        string
	items        []Item
	filtered     []Item
	input        textinput.Model
	cursor       int
	offset       int
	width        int
	height       int
	selected     Item
	hasSelection bool
	cancelled    bool
}

func newSelectorModel(items []Item, opts SelectOptions) *selectorModel {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "Filter"
	input.SetValue(opts.Query)
	input.CursorEnd()
	input.Focus()

	width := opts.Width
	if width <= 0 {
		width = 80
	}
	height := opts.Height
	if height <= 0 {
		height = 24
	}
	input.Width = max(1, width-2)

	return &selectorModel{
		title:    opts.Title,
		items:    append([]Item(nil), items...),
		filtered: filterItems(items, opts.Query),
		input:    input,
		width:    width,
		height:   height,
	}
}

func (m *selectorModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *selectorModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		// A zero-sized report (for example from a pty without dimensions)
		// carries no information, so keep the previous or default size.
		if message.Width > 0 {
			m.width = message.Width
			m.input.Width = max(1, m.width-2)
		}
		if message.Height > 0 {
			m.height = message.Height
		}
		m.keepCursorVisible()
		return m, nil
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.filtered) > 0 {
				m.selected = m.filtered[m.cursor]
				m.hasSelection = true
				return m, tea.Quit
			}
			return m, nil
		case "up", "ctrl+p":
			m.move(-1)
			return m, nil
		case "down", "ctrl+n":
			m.move(1)
			return m, nil
		case "pgup":
			m.movePage(-1)
			return m, nil
		case "pgdown":
			m.movePage(1)
			return m, nil
		}
	}

	previousQuery := m.input.Value()
	var command tea.Cmd
	m.input, command = m.input.Update(message)
	if m.input.Value() != previousQuery {
		m.filtered = filterItems(m.items, m.input.Value())
		m.cursor = 0
		m.offset = 0
	}
	return m, command
}

func (m *selectorModel) View() string {
	var lines []string
	if m.title != "" {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(m.title))
	}
	lines = append(lines, m.input.View())

	if len(m.filtered) == 0 {
		lines = append(lines, "No matches.")
		return strings.Join(lines, "\n") + "\n"
	}

	end := min(len(m.filtered), m.offset+m.visibleRows())
	widths := columnWidths(m.filtered[m.offset:end], max(1, m.width-2))
	for index := m.offset; index < end; index++ {
		prefix := "  "
		if index == m.cursor {
			prefix = "> "
		}
		lines = append(lines, prefix+renderColumns(m.filtered[index].Columns, widths))
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m *selectorModel) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor = (m.cursor + delta + len(m.filtered)) % len(m.filtered)
	m.keepCursorVisible()
}

func (m *selectorModel) movePage(direction int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor = max(0, min(len(m.filtered)-1, m.cursor+direction*m.visibleRows()))
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
	m.offset = max(0, min(m.offset, max(0, len(m.filtered)-rows)))
}

func (m *selectorModel) selection() (Item, error) {
	if m.cancelled || !m.hasSelection {
		return Item{}, ErrCancelled
	}
	return m.selected, nil
}

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
	rendered := make([]string, len(columns))
	for index, column := range columns {
		if index >= len(widths) {
			break
		}
		value := ansi.Truncate(column, widths[index], "")
		if index < len(columns)-1 {
			value += strings.Repeat(" ", max(0, widths[index]-lipgloss.Width(value)))
		}
		rendered[index] = value
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
