package ui

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// viewLines returns the selector view as plain text lines.
func viewLines(model *selectorModel) []string {
	return strings.Split(strings.TrimRight(ansi.Strip(model.View().Content), "\n"), "\n")
}

func letterItems(n int) []Item {
	items := make([]Item, n)
	for index := range items {
		items[index] = Item{ID: string(rune('a' + index)), Columns: []string{string(rune('A' + index))}}
	}
	return items
}

func TestFilterRanksMatchesAndKeepsTiesStable(t *testing.T) {
	t.Parallel()
	list := newCatalog([]Item{
		{ID: "first-tie", Columns: []string{"Alpha Song"}},
		{ID: "exact", Columns: []string{"alp"}},
		{ID: "second-tie", Columns: []string{"Alpha", "Song"}},
		{ID: "none", Columns: []string{"Beta"}},
	})

	if got := list.filter("alp", nil); !slices.Equal(got, []int{1, 0, 2}) {
		t.Fatalf("filter(alp) = %v, want [1 0 2]: exact first, ties in input order", got)
	}
	if got := list.filter("alp", []int{2, 3}); !slices.Equal(got, []int{2}) {
		t.Fatalf("filter(alp, within [2 3]) = %v, want [2]", got)
	}
	if got := list.filter("", nil); !slices.Equal(got, []int{0, 1, 2, 3}) {
		t.Fatalf("filter(\"\") = %v, want every index", got)
	}
}

func TestSelectorNarrowingMatchesFullFilter(t *testing.T) {
	t.Parallel()
	var items []Item
	for _, title := range []string{"Alpha song", "alpine", "Beta alp", "Gamma", "ALPS", "a-l-p-h-a", "Sal Paradise", "lap"} {
		items = append(items, Item{ID: title, Columns: []string{title, "artist"}})
	}
	model := newSelectorModel(items, SelectOptions{})

	// Extend, delete and extend again: every step must agree with a full
	// filter of the current query.
	for _, step := range []string{"a", "l", "p", "backspace", "backspace", "p", "h", "ctrl+u", "s", "a"} {
		model.Update(keyMsg(step))
		want := model.catalog.filter(model.input.Value(), nil)
		if !slices.Equal(model.matches, want) {
			t.Fatalf("after %q query %q: matches = %v, want %v", step, model.input.Value(), model.matches, want)
		}
	}
}

func TestSelectorSelectsStableIDAfterFiltering(t *testing.T) {
	t.Parallel()
	model := newSelectorModel([]Item{
		{ID: "one", Columns: []string{"Alpha first"}},
		{ID: "two", Columns: []string{"Alpha second"}},
		{ID: "three", Columns: []string{"Beta"}},
	}, SelectOptions{Query: "Alpha"})

	model.Update(keyMsg("up"))
	_, command := model.Update(keyMsg("enter"))
	if _, ok := command().(tea.QuitMsg); !ok {
		t.Fatalf("enter returned %T, want tea.QuitMsg", command())
	}
	selected, err := model.selection()
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "two" {
		t.Fatalf("selected ID = %q, want two", selected.ID)
	}
}

func TestSelectorNavigationWrapsAndPagesAreBounded(t *testing.T) {
	t.Parallel()
	model := newSelectorModel(letterItems(8), SelectOptions{Size: Size{Height: 6}})

	steps := []struct {
		key  string
		want int
	}{
		{"down", 7}, {"up", 0}, {"ctrl+p", 1}, {"ctrl+n", 0}, {"ctrl+n", 7}, {"ctrl+p", 0},
		{"pgup", 3}, {"pgup", 6}, {"pgup", 7},
		{"pgdown", 4}, {"pgdown", 1}, {"pgdown", 0},
	}
	for _, step := range steps {
		model.Update(keyMsg(step.key))
		if model.cursor != step.want {
			t.Fatalf("cursor after %q = %d, want %d", step.key, model.cursor, step.want)
		}
	}
}

func TestSelectorViewRendersBottomUp(t *testing.T) {
	t.Parallel()
	model := newSelectorModel(letterItems(3), SelectOptions{Title: "Letters"})

	lines := viewLines(model)
	if len(lines) != 5 {
		t.Fatalf("View() produced %d lines, want 5: %q", len(lines), lines)
	}
	if want := []string{"  C", "  B", "> A", "Letters"}; !slices.Equal(lines[:4], want) {
		t.Fatalf("View() rows = %q, want %q", lines[:4], want)
	}
	if !strings.Contains(lines[4], "Filter") {
		t.Fatalf("last line = %q, want the filter input", lines[4])
	}

	model.Update(keyMsg("up"))
	if lines := viewLines(model); lines[1] != "> B" || lines[2] != "  A" {
		t.Fatalf("after up, rows = %q, want marker on B", lines[:3])
	}
}

func TestSelectorScrollsOffsetWithCursor(t *testing.T) {
	t.Parallel()
	// Height 5 leaves two visible rows above the title and prompt.
	model := newSelectorModel(letterItems(5), SelectOptions{Title: "T", Size: Size{Height: 5}})

	if lines := viewLines(model); !slices.Equal(lines[:2], []string{"  B", "> A"}) {
		t.Fatalf("initial rows = %q", lines[:2])
	}
	model.Update(keyMsg("up"))
	model.Update(keyMsg("up"))
	if model.offset != 1 {
		t.Fatalf("offset = %d, want 1", model.offset)
	}
	if lines := viewLines(model); !slices.Equal(lines[:2], []string{"> C", "  B"}) {
		t.Fatalf("rows after scrolling up = %q, want [> C   B]", lines[:2])
	}
	model.Update(keyMsg("down")) // back to B, still visible
	if model.offset != 1 {
		t.Fatalf("offset after down = %d, want 1", model.offset)
	}
	model.Update(keyMsg("down"))
	model.Update(keyMsg("down")) // wraps to E, the last item
	if model.cursor != 4 || model.offset != 3 {
		t.Fatalf("cursor, offset after wrap = %d, %d; want 4, 3", model.cursor, model.offset)
	}

	// Growing the terminal shows everything, so the offset returns to 0.
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if model.offset != 0 {
		t.Fatalf("offset after growing = %d, want 0", model.offset)
	}
}

func TestSelectorAlignsColumnsAcrossAllMatches(t *testing.T) {
	t.Parallel()
	// Only the first two rows fit; the widest first column is off screen.
	model := newSelectorModel([]Item{
		{ID: "1", Columns: []string{"a", "one"}},
		{ID: "2", Columns: []string{"bb", "two"}},
		{ID: "3", Columns: []string{"a much longer title", "three"}},
	}, SelectOptions{Size: Size{Width: 80, Height: 5}})

	lines := viewLines(model)
	if want := []string{"  bb                   two", "> a                    one"}; !slices.Equal(lines[:2], want) {
		t.Fatalf("rows = %q, want columns aligned to the widest match %q", lines[:2], want)
	}
}

func TestSelectorTruncatesWideColumnsWithEllipsis(t *testing.T) {
	t.Parallel()
	model := newSelectorModel([]Item{
		{ID: "1", Columns: []string{"日本語のタイトル", "アーティスト名"}},
		{ID: "2", Columns: []string{"short", "x"}},
	}, SelectOptions{Size: Size{Width: 20, Height: 10}})

	for _, row := range viewLines(model)[:2] {
		if width := lipgloss.Width(row); width > 20 {
			t.Fatalf("row %q is %d cells, want at most 20", row, width)
		}
	}
	if row := viewLines(model)[1]; !strings.Contains(row, ellipsis) {
		t.Fatalf("row %q, want an ellipsis on the truncated column", row)
	}
}

func TestSelectorQuitClearsView(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"enter", "esc", "ctrl+c"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			model := newSelectorModel(letterItems(2), SelectOptions{})
			_, command := model.Update(keyMsg(name))
			if _, ok := command().(tea.QuitMsg); !ok {
				t.Fatalf("%q returned %T, want tea.QuitMsg", name, command())
			}
			if view := model.View().Content; view != "" {
				t.Fatalf("View() after %q = %q, want empty so the list is cleared", name, view)
			}
		})
	}
}

func TestSelectorCancellationReturnsErrCancelled(t *testing.T) {
	t.Parallel()
	model := newSelectorModel(letterItems(1), SelectOptions{})
	model.Update(keyMsg("esc"))

	if _, err := model.selection(); !errors.Is(err, ErrCancelled) {
		t.Fatalf("selection() error = %v, want ErrCancelled", err)
	}
}

func TestSelectorEmptyStateView(t *testing.T) {
	t.Parallel()
	model := newSelectorModel([]Item{{ID: "secret-id", Columns: []string{"Visible label"}}}, SelectOptions{Query: "missing"})
	view := model.View().Content
	if !strings.Contains(view, "No matches.") {
		t.Fatalf("View() = %q, want empty-state text", view)
	}
	if strings.Contains(view, "secret-id") {
		t.Fatalf("View() = %q, must not render item IDs", view)
	}
	if _, command := model.Update(keyMsg("enter")); command != nil || model.done {
		t.Fatal("enter without matches must not quit")
	}
}

func TestSelectorResizeChangesWidth(t *testing.T) {
	t.Parallel()
	model := newSelectorModel([]Item{{ID: "one", Columns: []string{"A very long column value"}}}, SelectOptions{Size: Size{Width: 80}})
	model.Update(tea.WindowSizeMsg{Width: 12, Height: 8})

	if model.width != 12 || model.input.Width() != 10 {
		t.Fatalf("widths = (%d, %d), want (12, 10)", model.width, model.input.Width())
	}
	if strings.Contains(model.View().Content, "A very long column value") {
		t.Fatal("View() did not truncate a row after resize")
	}
}

func TestResolveSelectsSingleInitialMatchWithoutTerminal(t *testing.T) {
	t.Parallel()
	items := []Item{
		{ID: "one", Columns: []string{"First track"}},
		{ID: "two", Columns: []string{"Second track"}},
	}

	// Neither stream is a terminal, so only the fast path can succeed.
	selected, err := Select(t.Context(), strings.NewReader(""), io.Discard, items, SelectOptions{Query: "Second"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "two" {
		t.Fatalf("Select() = %q, want two", selected.ID)
	}
}

func TestResolveErrors(t *testing.T) {
	t.Parallel()
	one := []Item{{ID: "one", Columns: []string{"One"}}}
	tests := []struct {
		name    string
		items   []Item
		query   string
		wantErr error
		wantMsg string
	}{
		{name: "no items", items: nil, wantErr: ErrNoItems, wantMsg: "nothing to select"},
		{name: "no match", items: one, query: "two", wantErr: ErrNoMatch, wantMsg: `no items match "two"`},
		{name: "needs terminal", items: one, wantErr: ErrNeedsTerminal, wantMsg: ErrNeedsTerminal.Error()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Select(t.Context(), strings.NewReader(""), io.Discard, test.items, SelectOptions{Query: test.query})
			if !errors.Is(err, test.wantErr) || err.Error() != test.wantMsg {
				t.Fatalf("Select() error = %v, want %q wrapping %v", err, test.wantMsg, test.wantErr)
			}
		})
	}
}
