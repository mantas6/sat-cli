package ui

import (
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderMarkdownContainsHeadingText(t *testing.T) {
	t.Parallel()
	for _, dark := range []bool{true, false} {
		rendered, err := renderMarkdown("# Pager heading\n", 80, dark)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(ansi.Strip(rendered), "Pager heading") {
			t.Fatalf("renderMarkdown(dark=%t) = %q, want heading text", dark, rendered)
		}
	}
}

// numberedLines renders n lines "line 0".."line n-1" regardless of input.
func numberedLines(n int) markdownRenderFunc {
	return func(string, int, bool) (string, error) {
		lines := make([]string, n)
		for index := range lines {
			lines[index] = fmt.Sprintf("line %d", index)
		}
		return strings.Join(lines, "\n"), nil
	}
}

func newTestPager(t *testing.T, opts PageOptions, render markdownRenderFunc) *pagerModel {
	t.Helper()
	model, err := newPagerModel("raw markdown", opts, render)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

// resize delivers a WindowSizeMsg and, when the pager schedules a debounced
// re-render, runs that command and delivers its message too.
func resize(model *pagerModel, width, height int) {
	_, command := model.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if command != nil {
		model.Update(command())
	}
}

func TestPagerScrollingKeysChangeOffset(t *testing.T) {
	t.Parallel()
	model := newTestPager(t, PageOptions{Size: Size{Width: 40, Height: 8}}, numberedLines(40))

	steps := []struct {
		key  string
		want func(offset int) bool
		desc string
	}{
		{"j", func(o int) bool { return o == 1 }, "1"},
		{"down", func(o int) bool { return o == 2 }, "2"},
		{"k", func(o int) bool { return o == 1 }, "1"},
		{"space", func(o int) bool { return o > 1 }, "> 1"},
		{"g", func(o int) bool { return o == 0 }, "0"},
		{"pgdown", func(o int) bool { return o == 7 }, "7"},
		{"b", func(o int) bool { return o == 0 }, "0"},
		{"d", func(o int) bool { return o == 3 }, "3"},
		{"u", func(o int) bool { return o == 0 }, "0"},
		{"G", func(o int) bool { return o == 33 }, "33"},
		{"home", func(o int) bool { return o == 0 }, "0"},
		{"end", func(o int) bool { return o == 33 }, "33"},
	}
	for _, step := range steps {
		model.Update(keyMsg(step.key))
		if offset := model.viewport.YOffset(); !step.want(offset) {
			t.Fatalf("offset after %q = %d, want %s", step.key, offset, step.desc)
		}
	}
}

func TestPagerResizeRerendersAtNewWidthAfterDebounce(t *testing.T) {
	t.Parallel()
	var widths []int
	model := newTestPager(t, PageOptions{Size: Size{Width: 80, Height: 24}}, func(markdown string, width int, _ bool) (string, error) {
		widths = append(widths, width)
		return fmt.Sprintf("%s at %d", markdown, width), nil
	})

	_, command := model.Update(tea.WindowSizeMsg{Width: 42, Height: 12})
	if got := fmt.Sprint(widths); got != "[80]" {
		t.Fatalf("render widths before debounce = %s, want [80]", got)
	}
	if command == nil {
		t.Fatal("width change did not schedule a re-render")
	}
	model.Update(command())
	if got := fmt.Sprint(widths); got != "[80 42]" {
		t.Fatalf("render widths = %s, want [80 42]", got)
	}
	if model.viewport.Width() != 42 || model.viewport.Height() != 11 {
		t.Fatalf("viewport size = (%d, %d), want (42, 11)", model.viewport.Width(), model.viewport.Height())
	}
	if !strings.Contains(model.View().Content, "raw markdown at 42") {
		t.Fatalf("View() = %q, want resized rendering", model.View().Content)
	}

	if _, command := model.Update(tea.WindowSizeMsg{}); command != nil {
		t.Fatal("zero-sized resize scheduled a re-render")
	}
	resize(model, 80, 24)
	if got := fmt.Sprint(widths); got != "[80 42]" {
		t.Fatalf("render widths after returning to 80 = %s, want the cached rendering", got)
	}
}

func TestPagerDropsStaleResizeRenders(t *testing.T) {
	t.Parallel()
	var widths []int
	model := newTestPager(t, PageOptions{Size: Size{Width: 80, Height: 24}}, func(_ string, width int, _ bool) (string, error) {
		widths = append(widths, width)
		return "content", nil
	})

	_, first := model.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	_, second := model.Update(tea.WindowSizeMsg{Width: 50, Height: 24})
	model.Update(first())
	model.Update(second())
	if got := fmt.Sprint(widths); got != "[80 50]" {
		t.Fatalf("render widths = %s, want only the settled width [80 50]", got)
	}
}

func TestPagerResizeClampsOffsetWhenTaller(t *testing.T) {
	t.Parallel()
	model := newTestPager(t, PageOptions{Size: Size{Width: 40, Height: 11}}, numberedLines(20))
	model.Update(keyMsg("G"))
	if got := model.viewport.YOffset(); got != 10 {
		t.Fatalf("offset at bottom = %d, want 10", got)
	}

	resize(model, 40, 16)
	if got := model.viewport.YOffset(); got != 5 {
		t.Fatalf("offset after growing = %d, want 5 (clamped to the new bottom)", got)
	}
	if !model.viewport.AtBottom() {
		t.Fatal("viewport should still be at the bottom")
	}
}

func TestPagerWidthChangeKeepsRelativeScrollPosition(t *testing.T) {
	t.Parallel()
	// Narrower widths wrap into proportionally more lines.
	render := func(_ string, width int, _ bool) (string, error) {
		return numberedLines(4000/width)("", width, true)
	}
	model := newTestPager(t, PageOptions{Size: Size{Width: 40, Height: 11}}, render)
	// 100 lines, 10 visible: max offset 90.
	for range 45 {
		model.Update(keyMsg("j"))
	}

	resize(model, 20, 11)
	// 200 lines, max offset 190: half way is 95.
	if got := model.viewport.YOffset(); got != 95 {
		t.Fatalf("offset after narrowing = %d, want 95", got)
	}

	model.Update(keyMsg("g"))
	resize(model, 40, 11)
	if got := model.viewport.YOffset(); got != 0 {
		t.Fatalf("offset after widening from the top = %d, want 0", got)
	}
}

func TestPagerFooterShowsKeyHelp(t *testing.T) {
	t.Parallel()
	model := newTestPager(t, PageOptions{Title: "Article", Size: Size{Width: 80, Height: 8}}, numberedLines(40))

	lines := strings.Split(ansi.Strip(model.View().Content), "\n")
	if len(lines) != 8 {
		t.Fatalf("View() has %d lines, want 8", len(lines))
	}
	if lines[0] != "Article" {
		t.Fatalf("title line = %q, want Article", lines[0])
	}
	footer := lines[len(lines)-1]
	for _, want := range []string{"0%", "j", "down", "q", "quit"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer = %q, want it to mention %q", footer, want)
		}
	}
}

func TestPagerRenderFailureQuitsWithError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("render failed")
	model := newTestPager(t, PageOptions{Size: Size{Width: 80, Height: 24}}, func(_ string, width int, _ bool) (string, error) {
		if width != 80 {
			return "", wantErr
		}
		return "content", nil
	})

	_, command := model.Update(tea.WindowSizeMsg{Width: 40, Height: 24})
	_, command = model.Update(command())
	if command == nil {
		t.Fatal("render failure did not quit")
	}
	if _, ok := command().(tea.QuitMsg); !ok || !errors.Is(model.renderErr, wantErr) {
		t.Fatalf("command() = %T, renderErr = %v, want tea.QuitMsg and %v", command(), model.renderErr, wantErr)
	}
}

func TestPagerQuitKeys(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"q", "esc", "ctrl+c"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			model := newTestPager(t, PageOptions{}, func(markdown string, _ int, _ bool) (string, error) {
				return markdown, nil
			})
			_, command := model.Update(keyMsg(name))
			if command == nil {
				t.Fatalf("Update(%q) returned no command", name)
			}
			if _, ok := command().(tea.QuitMsg); !ok || !model.quitting {
				t.Fatalf("Update(%q) command returned %T, want tea.QuitMsg", name, command())
			}
			if model.View().Content != "" {
				t.Fatalf("View() after quit = %q, want empty", model.View().Content)
			}
		})
	}
}

func TestPagerRerendersOnlyWhenBackgroundStyleChanges(t *testing.T) {
	t.Parallel()
	var styles []bool
	model := newTestPager(t, PageOptions{Size: Size{Width: 40, Height: 8}}, func(markdown string, _ int, dark bool) (string, error) {
		styles = append(styles, dark)
		return markdown, nil
	})
	if model.Init() == nil {
		t.Fatal("Init() = nil, want a background colour request")
	}

	model.Update(tea.BackgroundColorMsg{Color: color.Black}) // already dark
	model.Update(tea.BackgroundColorMsg{Color: color.White})
	model.Update(tea.BackgroundColorMsg{Color: color.White}) // unchanged
	model.Update(tea.BackgroundColorMsg{Color: color.Black}) // cached
	if got := fmt.Sprint(styles); got != "[true false]" {
		t.Fatalf("render styles = %s, want [true false]", got)
	}
}
