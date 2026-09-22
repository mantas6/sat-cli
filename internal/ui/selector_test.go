package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestFilterItemsRanksMatchesAndKeepsTiesStable(t *testing.T) {
	items := []Item{
		{ID: "first-tie", Columns: []string{"Alpha Song"}},
		{ID: "exact", Columns: []string{"alp"}},
		{ID: "second-tie", Columns: []string{"Alpha Song"}},
		{ID: "none", Columns: []string{"Beta"}},
	}

	filtered := filterItems(items, "alp")
	if len(filtered) != 3 {
		t.Fatalf("len(filterItems()) = %d, want 3", len(filtered))
	}
	if filtered[0].ID != "exact" {
		t.Fatalf("first match ID = %q, want exact", filtered[0].ID)
	}
	if filtered[1].ID != "first-tie" || filtered[2].ID != "second-tie" {
		t.Fatalf("tied IDs = %q, %q, want stable input order", filtered[1].ID, filtered[2].ID)
	}
}

func TestSelectorSelectsStableIDAfterFiltering(t *testing.T) {
	model := newSelectorModel([]Item{
		{ID: "one", Columns: []string{"Alpha first"}},
		{ID: "two", Columns: []string{"Alpha second"}},
		{ID: "three", Columns: []string{"Beta"}},
	}, SelectOptions{Query: "Alpha"})

	model.Update(key("up"))
	model.Update(key("enter"))
	selected, err := model.selection()
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "two" {
		t.Fatalf("selected ID = %q, want two", selected.ID)
	}
}

func TestSelectorNavigationWrapsAndPagesAreBounded(t *testing.T) {
	items := make([]Item, 8)
	for index := range items {
		items[index] = Item{ID: string(rune('a' + index)), Columns: []string{string(rune('A' + index))}}
	}
	model := newSelectorModel(items, SelectOptions{Height: 6})

	model.Update(key("down"))
	if model.cursor != 7 {
		t.Fatalf("cursor after down = %d, want 7", model.cursor)
	}
	model.Update(key("up"))
	if model.cursor != 0 {
		t.Fatalf("cursor after up = %d, want 0", model.cursor)
	}
	model.Update(key("pgup"))
	model.Update(key("pgup"))
	model.Update(key("pgup"))
	if model.cursor != 7 {
		t.Fatalf("cursor after page up = %d, want 7", model.cursor)
	}
	model.Update(key("pgdown"))
	model.Update(key("pgdown"))
	model.Update(key("pgdown"))
	if model.cursor != 0 {
		t.Fatalf("cursor after page down = %d, want 0", model.cursor)
	}
}

func TestSelectorViewRendersBottomUp(t *testing.T) {
	model := newSelectorModel([]Item{
		{ID: "a", Columns: []string{"A"}},
		{ID: "b", Columns: []string{"B"}},
		{ID: "c", Columns: []string{"C"}},
	}, SelectOptions{})

	lines := strings.Split(strings.TrimRight(model.View(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("View() produced %d lines, want 4: %q", len(lines), lines)
	}
	if lines[0] != "  C" || lines[1] != "  B" || lines[2] != "> A" {
		t.Fatalf("View() rows = %q, want bottom-up order [  C   B > A]", lines[:3])
	}
	if !strings.Contains(ansi.Strip(lines[3]), "Filter") {
		t.Fatalf("last line = %q, want the filter input", lines[3])
	}

	model.Update(key("up"))
	lines = strings.Split(strings.TrimRight(model.View(), "\n"), "\n")
	if lines[1] != "> B" || lines[2] != "  A" {
		t.Fatalf("after up, rows = %q, want marker on B", lines[:3])
	}
}

func TestSelectorCancellationReturnsErrCancelled(t *testing.T) {
	model := newSelectorModel([]Item{{ID: "one", Columns: []string{"One"}}}, SelectOptions{})
	model.Update(key("esc"))

	_, err := model.selection()
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("selection() error = %v, want ErrCancelled", err)
	}
}

func TestSelectorEmptyStateView(t *testing.T) {
	model := newSelectorModel([]Item{{ID: "secret-id", Columns: []string{"Visible label"}}}, SelectOptions{Query: "missing"})
	view := model.View()
	if !strings.Contains(view, "No matches.") {
		t.Fatalf("View() = %q, want empty-state text", view)
	}
	if strings.Contains(view, "secret-id") {
		t.Fatalf("View() = %q, must not render item IDs", view)
	}
}

func TestSelectorResizeChangesWidth(t *testing.T) {
	model := newSelectorModel([]Item{{ID: "one", Columns: []string{"A very long column value"}}}, SelectOptions{Width: 80})
	model.Update(tea.WindowSizeMsg{Width: 12, Height: 8})

	if model.width != 12 || model.input.Width != 10 {
		t.Fatalf("widths = (%d, %d), want (12, 10)", model.width, model.input.Width)
	}
	if strings.Contains(model.View(), "A very long column value") {
		t.Fatal("View() did not truncate a row after resize")
	}
}

func TestSelectionBeforeUISelectsSingleInitialMatch(t *testing.T) {
	items := []Item{
		{ID: "one", Columns: []string{"First track"}},
		{ID: "two", Columns: []string{"Second track"}},
	}

	selected, done, err := Resolve(items, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if !done || selected.ID != "two" {
		t.Fatalf("Resolve() = (%q, %t), want (two, true)", selected.ID, done)
	}
}

func TestSelectionBeforeUIErrorsForEmptyAndNoMatches(t *testing.T) {
	if _, _, err := Resolve(nil, ""); err == nil || err.Error() != "nothing to select" {
		t.Fatalf("empty items error = %v, want nothing to select", err)
	}
	if _, _, err := Resolve([]Item{{Columns: []string{"One"}}}, "two"); err == nil || err.Error() != `no items match "two"` {
		t.Fatalf("no matches error = %v", err)
	}
}

func key(value string) tea.KeyMsg {
	switch value {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
	}
}
