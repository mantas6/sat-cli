package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRenderMarkdownContainsHeadingText(t *testing.T) {
	rendered, err := RenderMarkdown("# Pager heading\n", 80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "Pager heading") {
		t.Fatalf("RenderMarkdown() = %q, want heading text", rendered)
	}
}

func TestPagerScrollingKeysChangeOffset(t *testing.T) {
	render := func(string, int) (string, error) {
		lines := make([]string, 40)
		for index := range lines {
			lines[index] = fmt.Sprintf("line %d", index)
		}
		return strings.Join(lines, "\n"), nil
	}
	model, err := newPagerModel("raw", PageOptions{Width: 40, Height: 8}, render)
	if err != nil {
		t.Fatal(err)
	}

	model.Update(pagerKey("j"))
	if model.viewport.YOffset != 1 {
		t.Fatalf("offset after j = %d, want 1", model.viewport.YOffset)
	}
	model.Update(pagerKey("pgdown"))
	if model.viewport.YOffset <= 1 {
		t.Fatalf("offset after pgdown = %d, want greater than 1", model.viewport.YOffset)
	}
	model.Update(pagerKey("G"))
	bottom := model.viewport.YOffset
	model.Update(pagerKey("g"))
	if bottom == 0 || model.viewport.YOffset != 0 {
		t.Fatalf("offsets after G and g = (%d, %d), want bottom then zero", bottom, model.viewport.YOffset)
	}
}

func TestPagerResizeRerendersAtNewWidth(t *testing.T) {
	var widths []int
	render := func(markdown string, width int) (string, error) {
		widths = append(widths, width)
		return fmt.Sprintf("%s at %d", markdown, width), nil
	}
	model, err := newPagerModel("raw markdown", PageOptions{Width: 80, Height: 24}, render)
	if err != nil {
		t.Fatal(err)
	}

	model.Update(tea.WindowSizeMsg{Width: 42, Height: 12})
	if got := fmt.Sprint(widths); got != "[80 42]" {
		t.Fatalf("render widths = %s, want [80 42]", got)
	}
	if model.viewport.Width != 42 || model.viewport.Height != 11 {
		t.Fatalf("viewport size = (%d, %d), want (42, 11)", model.viewport.Width, model.viewport.Height)
	}
	if !strings.Contains(model.View(), "raw markdown at 42") {
		t.Fatalf("View() = %q, want resized rendering", model.View())
	}

	model.Update(tea.WindowSizeMsg{})
	if got := fmt.Sprint(widths); got != "[80 42]" {
		t.Fatalf("render widths after zero resize = %s, want unchanged", got)
	}
}

func TestPagerQuitKeys(t *testing.T) {
	keys := []tea.KeyMsg{
		pagerKey("q"),
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
	}
	for _, message := range keys {
		t.Run(message.String(), func(t *testing.T) {
			model, err := newPagerModel("content", PageOptions{}, func(markdown string, _ int) (string, error) {
				return markdown, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			_, command := model.Update(message)
			if command == nil || !model.quitting {
				t.Fatalf("Update(%q) did not quit", message.String())
			}
		})
	}
}

func pagerKey(value string) tea.KeyMsg {
	switch value {
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}
	}
}
