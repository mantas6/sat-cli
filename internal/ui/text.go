package ui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Align is the horizontal alignment of a table column.
type Align int

const (
	// AlignLeft pads values on the right.
	AlignLeft Align = iota
	// AlignRight pads values on the left so numbers line up.
	AlignRight
)

// Tone is the colour role of a cell. It only takes effect when
// TextOptions.Color is set.
type Tone int

const (
	// ToneDefault leaves the text unstyled.
	ToneDefault Tone = iota
	// ToneGood marks a healthy value (green).
	ToneGood
	// ToneBad marks a value that needs attention (red).
	ToneBad
	// ToneMuted marks a missing or stale value (faint).
	ToneMuted
	// ToneStrong emphasises a value (bold).
	ToneStrong
)

// Cell is one value in a table row, section title or detail field.
type Cell struct {
	Text string
	Tone Tone
	// Flag is drawn with the cell's tone in the flag slot of a Column with
	// Flag set, for example "!". It is at most one cell wide and is ignored
	// elsewhere.
	Flag string
}

// Column describes one table column.
type Column struct {
	Header string
	Align  Align
	// Drop ranks the column for dropping when the table is wider than
	// TextOptions.Width: the highest rank goes first, the rightmost of equal
	// ranks first. Zero means the column is never dropped.
	Drop int
	// Flag reserves one cell right after the value for Cell.Flag, so values
	// and the header still line up on their last character.
	Flag bool
	// MinWidth, when positive, lets the column be truncated with "…" down to
	// this many cells once no more columns can be dropped.
	MinWidth int
}

// TextOptions configures RenderTable and RenderDetail.
type TextOptions struct {
	// Color styles cells by their Tone. When false the output contains no
	// escape sequences.
	Color bool
	// Width is the number of cells a line may take. Zero means unlimited.
	Width int
}

// paint styles text by tone when colour is on. It never calls Render when
// colour is off, so plain output has no escape sequences.
func (opts TextOptions) paint(text string, tone Tone) string {
	if !opts.Color || text == "" {
		return text
	}
	switch tone {
	case ToneGood:
		return goodStyle.Render(text)
	case ToneBad:
		return badStyle.Render(text)
	case ToneMuted:
		return mutedStyle.Render(text)
	case ToneStrong:
		return strongStyle.Render(text)
	default:
		return text
	}
}

// RenderTable renders a header row and rows as aligned columns separated by
// two spaces, one line per row with a trailing newline. Rows may have fewer
// cells than columns; missing cells are empty and extra cells are ignored.
//
// When opts.Width is positive and the table is wider, columns are dropped by
// Column.Drop, then columns with a MinWidth are truncated with "…", and as a
// last resort every line is cut at the width. Cells are never wrapped.
func RenderTable(columns []Column, rows [][]Cell, opts TextOptions) string {
	if len(columns) == 0 {
		return ""
	}

	widths := make([]int, len(columns))
	header := make([]Cell, len(columns))
	for index, column := range columns {
		widths[index] = lipgloss.Width(column.Header)
		header[index] = Cell{Text: column.Header, Tone: ToneStrong}
	}
	for _, row := range rows {
		for index, cell := range row {
			if index < len(widths) {
				widths[index] = max(widths[index], lipgloss.Width(cell.Text))
			}
		}
	}
	visible := fitTable(columns, widths, opts.Width)

	var out strings.Builder
	writeLine(&out, tableRow(columns, widths, visible, header), "", opts)
	for _, row := range rows {
		writeLine(&out, tableRow(columns, widths, visible, row), "", opts)
	}
	return out.String()
}

// fitTable returns the indexes of the columns to show within available
// cells, dropping columns by rank and then shrinking the widest truncatable
// column first. It lowers widths in place. The result may still be wider
// than available; the caller cuts the lines.
func fitTable(columns []Column, widths []int, available int) []int {
	visible := make([]int, len(columns))
	for index := range visible {
		visible[index] = index
	}
	if available <= 0 {
		return visible
	}

	for tableWidth(columns, widths, visible) > available {
		drop := -1
		for position, index := range visible {
			rank := columns[index].Drop
			if rank > 0 && (drop < 0 || rank >= columns[visible[drop]].Drop) {
				drop = position
			}
		}
		if drop < 0 {
			break
		}
		visible = slices.Delete(visible, drop, drop+1)
	}

	for tableWidth(columns, widths, visible) > available {
		widest := -1
		for _, index := range visible {
			minWidth := columns[index].MinWidth
			if minWidth > 0 && widths[index] > minWidth && (widest < 0 || widths[index] > widths[widest]) {
				widest = index
			}
		}
		if widest < 0 {
			break
		}
		widths[widest]--
	}
	return visible
}

// tableWidth returns the width of the visible columns with separators and
// flag slots.
func tableWidth(columns []Column, widths []int, visible []int) int {
	total := 2 * max(0, len(visible)-1)
	for _, index := range visible {
		total += widths[index]
		if columns[index].Flag {
			total++
		}
	}
	return total
}

// tableRow lays out one row as spans: each visible cell truncated with "…"
// to its column width, padded by the column's alignment and followed by the
// flag slot when the column has one.
func tableRow(columns []Column, widths []int, visible []int, cells []Cell) []span {
	spans := make([]span, 0, 3*len(visible))
	for position, index := range visible {
		if position > 0 {
			spans = append(spans, span{text: "  "})
		}
		var cell Cell
		if index < len(cells) {
			cell = cells[index]
		}
		text := ansi.Truncate(cell.Text, widths[index], ellipsis)
		padding := span{text: strings.Repeat(" ", max(0, widths[index]-lipgloss.Width(text)))}
		if columns[index].Align == AlignRight {
			spans = append(spans, padding, span{text: text, tone: cell.Tone})
		} else {
			spans = append(spans, span{text: text, tone: cell.Tone}, padding)
		}
		if columns[index].Flag {
			flag := ansi.Truncate(cell.Flag, 1, "")
			spans = append(spans, span{text: flag, tone: cell.Tone}, span{text: strings.Repeat(" ", 1-lipgloss.Width(flag))})
		}
	}
	return spans
}

// Field is one labelled line of a detail section. Its value cells are joined
// by two spaces.
type Field struct {
	Label string
	Value []Cell
}

// Section is a titled group of fields. A section without title cells has no
// title line.
type Section struct {
	Title  []Cell
	Fields []Field
}

// RenderDetail renders sections separated by a blank line. Each section is
// its title cells joined by two spaces, followed by its fields indented by
// two spaces with the labels padded to the longest label of all sections.
// When opts.Width is positive, longer lines are truncated with "…". Every
// line ends with a newline.
func RenderDetail(sections []Section, opts TextOptions) string {
	labelWidth := 0
	for _, section := range sections {
		for _, field := range section.Fields {
			labelWidth = max(labelWidth, lipgloss.Width(field.Label))
		}
	}

	var out strings.Builder
	for index, section := range sections {
		if index > 0 {
			out.WriteString("\n")
		}
		if len(section.Title) > 0 {
			writeLine(&out, joinCells(nil, section.Title), ellipsis, opts)
		}
		for _, field := range section.Fields {
			label := "  " + field.Label + strings.Repeat(" ", labelWidth-lipgloss.Width(field.Label))
			line := []span{{text: label}}
			if len(field.Value) > 0 {
				line = joinCells(append(line, span{text: "  "}), field.Value)
			}
			writeLine(&out, line, ellipsis, opts)
		}
	}
	return out.String()
}

// span is a run of text drawn with one tone.
type span struct {
	text string
	tone Tone
}

// joinCells appends cells to spans, separated by two spaces.
func joinCells(spans []span, cells []Cell) []span {
	for index, cell := range cells {
		if index > 0 {
			spans = append(spans, span{text: "  "})
		}
		spans = append(spans, span{text: cell.Text, tone: cell.Tone})
	}
	return spans
}

// writeLine writes spans as one line with a trailing newline. When
// opts.Width is positive and the line is wider, it is cut to the width with
// tail appended. Cutting and trimming trailing spaces work on the plain spans
// before they are painted, so no empty styled runs are left behind.
func writeLine(out *strings.Builder, spans []span, tail string, opts TextOptions) {
	if opts.Width > 0 {
		total := 0
		for _, part := range spans {
			total += lipgloss.Width(part.text)
		}
		if total > opts.Width {
			budget := max(0, opts.Width-lipgloss.Width(tail))
			cut := make([]span, 0, len(spans)+1)
			for _, part := range spans {
				text := ansi.Truncate(part.text, budget, "")
				cut = append(cut, span{text: text, tone: part.tone})
				if text != part.text {
					break
				}
				budget -= lipgloss.Width(text)
			}
			spans = append(cut, span{text: tail})
		}
	}
	for len(spans) > 0 {
		last := strings.TrimRight(spans[len(spans)-1].text, " ")
		if last != "" {
			spans[len(spans)-1].text = last
			break
		}
		spans = spans[:len(spans)-1]
	}
	for _, part := range spans {
		out.WriteString(opts.paint(part.text, part.tone))
	}
	out.WriteString("\n")
}
