package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderMarkdownContainsHeadingText(t *testing.T) {
	rendered, err := renderMarkdown("# Pager heading\n", 80, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ansi.Strip(rendered), "Pager heading") {
		t.Fatalf("renderMarkdown() = %q, want heading text", rendered)
	}
}

func TestPagerScrollingKeysChangeOffset(t *testing.T) {
	render := func(string, int, bool) (string, error) {
		lines := make([]string, 40)
		for index := range lines {
			lines[index] = fmt.Sprintf("line %d", index)
		}
		return strings.Join(lines, "\n"), nil
	}
	model, err := newPagerModel("raw", PageOptions{Size: Size{Width: 40, Height: 8}}, render)
	if err != nil {
		t.Fatal(err)
	}

	model.Update(keyMsg("j"))
	if model.viewport.YOffset() != 1 {
		t.Fatalf("offset after j = %d, want 1", model.viewport.YOffset())
	}
	model.Update(keyMsg("pgdown"))
	if model.viewport.YOffset() <= 1 {
		t.Fatalf("offset after pgdown = %d, want greater than 1", model.viewport.YOffset())
	}
	model.Update(keyMsg("G"))
	bottom := model.viewport.YOffset()
	model.Update(keyMsg("g"))
	if bottom == 0 || model.viewport.YOffset() != 0 {
		t.Fatalf("offsets after G and g = (%d, %d), want bottom then zero", bottom, model.viewport.YOffset())
	}
}

func TestPagerResizeRerendersAtNewWidth(t *testing.T) {
	var widths []int
	render := func(markdown string, width int, _ bool) (string, error) {
		widths = append(widths, width)
		return fmt.Sprintf("%s at %d", markdown, width), nil
	}
	model, err := newPagerModel("raw markdown", PageOptions{Size: Size{Width: 80, Height: 24}}, render)
	if err != nil {
		t.Fatal(err)
	}

	model.Update(tea.WindowSizeMsg{Width: 42, Height: 12})
	if got := fmt.Sprint(widths); got != "[80 42]" {
		t.Fatalf("render widths = %s, want [80 42]", got)
	}
	if model.viewport.Width() != 42 || model.viewport.Height() != 11 {
		t.Fatalf("viewport size = (%d, %d), want (42, 11)", model.viewport.Width(), model.viewport.Height())
	}
	if !strings.Contains(model.View().Content, "raw markdown at 42") {
		t.Fatalf("View() = %q, want resized rendering", model.View().Content)
	}

	model.Update(tea.WindowSizeMsg{})
	if got := fmt.Sprint(widths); got != "[80 42]" {
		t.Fatalf("render widths after zero resize = %s, want unchanged", got)
	}
}

func TestPagerQuitKeys(t *testing.T) {
	keys := []tea.KeyPressMsg{
		keyMsg("q"),
		keyMsg("esc"),
		keyMsg("ctrl+c"),
	}
	for _, message := range keys {
		t.Run(message.String(), func(t *testing.T) {
			model, err := newPagerModel("content", PageOptions{}, func(markdown string, _ int, _ bool) (string, error) {
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

func TestPagerRerendersOnlyWhenBackgroundStyleChanges(t *testing.T) {
	var styles []bool
	model, err := newPagerModel("md", PageOptions{Size: Size{Width: 40, Height: 8}}, func(markdown string, _ int, dark bool) (string, error) {
		styles = append(styles, dark)
		return markdown, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if model.Init() == nil {
		t.Fatal("Init() = nil, want a background colour request")
	}

	model.Update(tea.BackgroundColorMsg{Color: color.Black}) // already dark
	model.Update(tea.BackgroundColorMsg{Color: color.White})
	model.Update(tea.BackgroundColorMsg{Color: color.White}) // unchanged
	if got := fmt.Sprint(styles); got != "[true false]" {
		t.Fatalf("render styles = %s, want [true false]", got)
	}
}
