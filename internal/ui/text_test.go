package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// machineTable returns a table shaped like a list of machines, with drop
// ranks, flag columns, a truncatable name and every tone.
func machineTable() ([]Column, [][]Cell) {
	columns := []Column{
		{Header: "NAME", MinWidth: 8},
		{Header: "STATUS"},
		{Header: "SEEN", Align: AlignRight, Drop: 2},
		{Header: "CPU", Align: AlignRight, Drop: 3},
		{Header: "MEM", Align: AlignRight, Drop: 1, Flag: true},
		{Header: "DISK", Align: AlignRight, Drop: 1, Flag: true},
		{Header: "TEMP", Align: AlignRight, Drop: 4},
	}
	rows := [][]Cell{
		{
			{Text: "backup-nas-in-the-basement", Tone: ToneStrong},
			{Text: "offline", Tone: ToneBad},
			{Text: "3h", Tone: ToneMuted},
			{Text: "3%", Tone: ToneMuted},
			{Text: "41%", Tone: ToneMuted},
			{Text: "93%", Tone: ToneBad, Flag: "!"},
			{Text: "41°C", Tone: ToneMuted},
		},
		{
			{Text: "build-01", Tone: ToneStrong},
			{Text: "online", Tone: ToneGood},
			{Text: "8s"},
			{Text: "87%"},
			{Text: "91%", Tone: ToneBad, Flag: "!"},
			{Text: "42%"},
			{Text: "78°C"},
		},
		{
			{Text: "pi", Tone: ToneStrong},
			{Text: "online", Tone: ToneGood},
			{Text: "41s"},
			{Text: "-", Tone: ToneMuted},
		},
	}
	return columns, rows
}

func machineDetail() []Section {
	return []Section{
		{
			Title: []Cell{{Text: "build-01", Tone: ToneStrong}, {Text: "online", Tone: ToneGood}},
			Fields: []Field{
				{Label: "Uptime", Value: []Cell{{Text: "5h"}}},
				{Label: "Kernel", Value: []Cell{{Text: "6.8.0-45-generic"}}},
			},
		},
		{
			Title: []Cell{{Text: "Memory", Tone: ToneStrong}},
			Fields: []Field{
				{Label: "Used", Value: []Cell{{Text: "91%", Tone: ToneBad}, {Text: "14.1 / 15.5 GB"}, {Text: "(high)", Tone: ToneBad}}},
				{Label: "Swap", Value: []Cell{{Text: "-", Tone: ToneMuted}}},
			},
		},
		{
			Title: []Cell{{Text: "Disk", Tone: ToneStrong}},
			Fields: []Field{
				{Label: "Temperature", Value: []Cell{{Text: "38°C"}}},
				{Label: "Notes"},
			},
		},
	}
}

func TestRenderTableAlignsColumnsAndFlagSlot(t *testing.T) {
	t.Parallel()
	columns := []Column{
		{Header: "NAME"},
		{Header: "CPU", Align: AlignRight},
		{Header: "MEM", Align: AlignRight, Flag: true},
		{Header: "TEMP", Align: AlignRight},
	}
	rows := [][]Cell{
		{{Text: "web-1"}, {Text: "12%"}, {Text: "63%"}, {Text: "54°C"}},
		{{Text: "build-01"}, {Text: "87%"}, {Text: "91%", Flag: "!"}, {Text: "7°C"}},
		{{Text: "pi"}, {Text: "-"}, {Text: "-", Flag: "!?"}},
	}

	got := RenderTable(columns, rows, TextOptions{})
	want := "" +
		"NAME      CPU  MEM   TEMP\n" +
		"web-1     12%  63%   54°C\n" +
		"build-01  87%  91%!   7°C\n" +
		"pi          -    -!\n"
	if got != want {
		t.Fatalf("RenderTable =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderTableDropsHighestRankRightmostFirst(t *testing.T) {
	t.Parallel()
	columns := []Column{
		{Header: "AAAA"},
		{Header: "BBBB", Drop: 1},
		{Header: "CCCC", Drop: 2},
		{Header: "DDDD", Drop: 1},
	}

	for _, test := range []struct {
		width int
		want  string
	}{
		{width: 0, want: "AAAA  BBBB  CCCC  DDDD\n"},
		{width: 22, want: "AAAA  BBBB  CCCC  DDDD\n"},
		{width: 21, want: "AAAA  BBBB  DDDD\n"},
		{width: 15, want: "AAAA  BBBB\n"},
		{width: 9, want: "AAAA\n"},
		{width: 3, want: "AAA\n"},
	} {
		if got := RenderTable(columns, nil, TextOptions{Width: test.width}); got != test.want {
			t.Errorf("width %d: RenderTable = %q, want %q", test.width, got, test.want)
		}
	}
}

func TestRenderTableDropsBeforeTruncatingMinWidth(t *testing.T) {
	t.Parallel()
	columns := []Column{
		{Header: "NAME", MinWidth: 5},
		{Header: "OK"},
		{Header: "X", Drop: 1},
	}
	rows := [][]Cell{{{Text: "backup-nas"}, {Text: "up"}, {Text: "1"}}}

	for _, test := range []struct {
		width int
		want  string
	}{
		{width: 17, want: "NAME        OK  X\nbackup-nas  up  1\n"},
		{width: 16, want: "NAME        OK\nbackup-nas  up\n"},
		{width: 12, want: "NAME      OK\nbackup-…  up\n"},
		{width: 9, want: "NAME   OK\nback…  up\n"},
		{width: 8, want: "NAME   O\nback…  u\n"},
	} {
		if got := RenderTable(columns, rows, TextOptions{Width: test.width}); got != test.want {
			t.Errorf("width %d: RenderTable =\n%s\nwant\n%s", test.width, got, test.want)
		}
	}
}

func TestRenderTableTrimsTrailingWhitespace(t *testing.T) {
	t.Parallel()
	columns := []Column{{Header: "NAME"}, {Header: "NOTE"}, {Header: "FLAG", Flag: true}}
	rows := [][]Cell{
		{{Text: "a"}, {Text: "long note"}, {Text: ""}},
		{{Text: "b"}},
	}

	got := RenderTable(columns, rows, TextOptions{})
	want := "" +
		"NAME  NOTE       FLAG\n" +
		"a     long note\n" +
		"b\n"
	if got != want {
		t.Fatalf("RenderTable = %q, want %q", got, want)
	}
}

func TestRenderTableWithoutColumnsIsEmpty(t *testing.T) {
	t.Parallel()
	if got := RenderTable(nil, [][]Cell{{{Text: "x"}}}, TextOptions{Width: 10}); got != "" {
		t.Fatalf("RenderTable(nil) = %q, want empty", got)
	}
}

func TestRenderDetailSharesLabelWidthAcrossSections(t *testing.T) {
	t.Parallel()
	got := RenderDetail(machineDetail(), TextOptions{})
	want := "" +
		"build-01  online\n" +
		"  Uptime       5h\n" +
		"  Kernel       6.8.0-45-generic\n" +
		"\n" +
		"Memory\n" +
		"  Used         91%  14.1 / 15.5 GB  (high)\n" +
		"  Swap         -\n" +
		"\n" +
		"Disk\n" +
		"  Temperature  38°C\n" +
		"  Notes\n"
	if got != want {
		t.Fatalf("RenderDetail =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderDetailTruncatesLongLines(t *testing.T) {
	t.Parallel()
	sections := []Section{
		{Fields: []Field{{Label: "Model", Value: []Cell{{Text: "AMD Ryzen 7 5800X"}}}}},
		{Title: []Cell{{Text: "A very long title"}}},
	}

	got := RenderDetail(sections, TextOptions{Width: 16})
	want := "" +
		"  Model  AMD Ry…\n" +
		"\n" +
		"A very long tit…\n"
	if got != want {
		t.Fatalf("RenderDetail =\n%s\nwant\n%s", got, want)
	}
}

func TestRenderTextFitsEveryWidth(t *testing.T) {
	t.Parallel()
	columns, rows := machineTable()
	sections := machineDetail()

	for _, color := range []bool{false, true} {
		for width := 1; width <= 120; width++ {
			opts := TextOptions{Color: color, Width: width}
			for name, text := range map[string]string{
				"table":  RenderTable(columns, rows, opts),
				"detail": RenderDetail(sections, opts),
			} {
				if !strings.HasSuffix(text, "\n") {
					t.Fatalf("%s color=%t width=%d: output %q does not end with a newline", name, color, width, text)
				}
				for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
					if got := lipgloss.Width(line); got > width {
						t.Errorf("%s color=%t width=%d: line %q is %d cells wide", name, color, width, line, got)
					}
					if plain := ansi.Strip(line); strings.TrimRight(plain, " ") != plain {
						t.Errorf("%s color=%t width=%d: line %q has trailing whitespace", name, color, width, line)
					}
				}
			}
		}
	}
}

func TestRenderTextColor(t *testing.T) {
	t.Parallel()
	columns, rows := machineTable()
	sections := machineDetail()

	for _, width := range []int{0, 80, 40, 20, 12} {
		plainTable := RenderTable(columns, rows, TextOptions{Width: width})
		plainDetail := RenderDetail(sections, TextOptions{Width: width})
		for name, text := range map[string]string{"table": plainTable, "detail": plainDetail} {
			if strings.Contains(text, "\x1b") {
				t.Errorf("%s width=%d without colour contains an escape sequence: %q", name, width, text)
			}
		}

		colorTable := RenderTable(columns, rows, TextOptions{Color: true, Width: width})
		colorDetail := RenderDetail(sections, TextOptions{Color: true, Width: width})
		if got := ansi.Strip(colorTable); got != plainTable {
			t.Errorf("width=%d: stripped colour table =\n%s\nwant\n%s", width, got, plainTable)
		}
		if got := ansi.Strip(colorDetail); got != plainDetail {
			t.Errorf("width=%d: stripped colour detail =\n%s\nwant\n%s", width, got, plainDetail)
		}
	}

	colorTable := RenderTable(columns, rows, TextOptions{Color: true})
	for _, want := range []string{
		strongStyle.Render("NAME"),
		badStyle.Render("offline"),
		goodStyle.Render("online"),
		mutedStyle.Render("3h"),
		badStyle.Render("91%") + badStyle.Render("!"),
	} {
		if !strings.Contains(want, "\x1b[") {
			t.Fatalf("style output %q has no SGR sequence", want)
		}
		if !strings.Contains(colorTable, want) {
			t.Errorf("colour table does not contain %q:\n%s", want, colorTable)
		}
	}
	if !strings.Contains(colorTable, "  87%  ") {
		t.Errorf("default-tone cell is styled:\n%s", colorTable)
	}

	colorDetail := RenderDetail(sections, TextOptions{Color: true})
	if want := badStyle.Render("91%") + "  14.1 / 15.5 GB  " + badStyle.Render("(high)"); !strings.Contains(colorDetail, want) {
		t.Errorf("colour detail does not contain %q:\n%s", want, colorDetail)
	}
}
