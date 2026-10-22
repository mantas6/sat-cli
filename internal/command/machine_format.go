package command

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

// missingText stands for a reading the machine did not report.
const missingText = "-"

// machineView is how machines are rendered: the time ages are measured
// against, whether to colour the output and how many cells a line may take.
type machineView struct {
	Now   time.Time
	Color bool
	// Width is the number of cells a line may take. Zero means unlimited.
	Width int
}

// machineView colours output only when stdout is a terminal, NO_COLOR is
// unset and TERM is not dumb. Lines are limited to the terminal width, or 80
// cells when it is unknown; piped output is not limited.
func (a *App) machineView() machineView {
	view := machineView{Now: a.Now()}
	if !a.IsTTY(a.Stdout) {
		return view
	}
	view.Color = a.Getenv("NO_COLOR") == "" && a.Getenv("TERM") != "dumb"
	view.Width = 80
	if width, _, ok := a.TermSize(); ok && width > 0 {
		view.Width = width
	}
	return view
}

// machineColumns are the columns of formatMachineTable. Drop ranks decide
// which readings go first on a narrow terminal; name and status stay.
var machineColumns = []ui.Column{
	{Header: "NAME", MinWidth: 8},
	{Header: "STATUS"},
	{Header: "SEEN", Align: ui.AlignRight, Drop: 2},
	{Header: "UPTIME", Align: ui.AlignRight, Drop: 4},
	{Header: "CPU", Align: ui.AlignRight, Drop: 3},
	{Header: "LOAD", Align: ui.AlignRight, Drop: 5},
	{Header: "MEM", Align: ui.AlignRight, Drop: 1, Flag: true},
	{Header: "DISK", Align: ui.AlignRight, Drop: 1, Flag: true},
	{Header: "TEMP", Align: ui.AlignRight, Drop: 6},
}

// formatMachineTable renders machines as a table with a header row, or ""
// when there are none. Missing readings are shown as "-", high memory and
// disk usage is flagged with "!" and an offline machine's readings are muted.
func formatMachineTable(machines []api.Machine, view machineView) string {
	if len(machines) == 0 {
		return ""
	}
	rows := make([][]ui.Cell, 0, len(machines))
	for _, machine := range machines {
		reading := func(text string) ui.Cell {
			return readingCell(text, machine.Online)
		}
		seen := missingText
		if machine.ReceivedAt != nil {
			seen = ageText(view.Now, *machine.ReceivedAt)
		}
		status := ui.Cell{Text: "online", Tone: ui.ToneGood}
		if !machine.Online {
			status = ui.Cell{Text: "offline", Tone: ui.ToneBad}
		}
		rows = append(rows, []ui.Cell{
			{Text: plainText(machine.Name), Tone: ui.ToneStrong},
			status,
			reading(seen),
			reading(uptimeText(machine.Uptime)),
			reading(percentText(machine.CPU.UsagePercentage)),
			reading(loadText(machine.LoadAverage.One)),
			usageCell(machine.Memory.UsagePercentage, machine.Memory.UsageHigh, machine.Online),
			usageCell(machine.Disk.UsagePercentage, machine.Disk.UsageHigh, machine.Online),
			reading(tempText(machine.CPU.Temperature)),
		})
	}
	return ui.RenderTable(machineColumns, rows, ui.TextOptions{Color: view.Color, Width: view.Width})
}

// readingCell mutes a missing reading and every reading of an offline
// machine.
func readingCell(text string, online bool) ui.Cell {
	cell := ui.Cell{Text: text}
	if text == missingText || !online {
		cell.Tone = ui.ToneMuted
	}
	return cell
}

// usageCell is a usage percentage that is flagged with "!" and shown as bad
// when high, even on an offline machine.
func usageCell(percentage *float64, high, online bool) ui.Cell {
	cell := readingCell(percentText(percentage), online)
	if high && percentage != nil {
		cell.Tone = ui.ToneBad
		cell.Flag = "!"
	}
	return cell
}

// roundText rounds value to a whole number without a "-0".
func roundText(value float64) string {
	rounded := math.Round(value)
	if rounded == 0 {
		rounded = 0 // turns -0 into 0
	}
	return strconv.FormatFloat(rounded, 'f', 0, 64)
}

// percentText renders a percentage rounded to a whole number, for example
// "87%".
func percentText(value *float64) string {
	if value == nil {
		return missingText
	}
	return roundText(*value) + "%"
}

// tempText renders a temperature in degrees Celsius rounded to a whole
// number, for example "78°C".
func tempText(value *float64) string {
	if value == nil {
		return missingText
	}
	return roundText(*value) + "°C"
}

// loadText renders a load average with two decimals, for example "6.12".
func loadText(value *float64) string {
	if value == nil {
		return missingText
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

// uptimeText renders the API's human-readable uptime, or one derived from
// the milliseconds when that is missing.
func uptimeText(uptime api.MachineUptime) string {
	if text := plainText(uptime.ForHumans); text != "" {
		return text
	}
	if uptime.Milliseconds == nil {
		return missingText
	}
	return durationText(time.Duration(*uptime.Milliseconds) * time.Millisecond)
}

// ageText renders how long before now then was, for example "8s" or "3m".
// A time after now counts as "0s".
func ageText(now, then time.Time) string {
	return durationText(now.Sub(then))
}

// durationText renders duration in its largest whole unit of seconds,
// minutes, hours or days. Negative durations count as "0s".
func durationText(duration time.Duration) string {
	day := 24 * time.Hour
	switch {
	case duration < time.Minute:
		return strconv.FormatInt(int64(max(0, duration/time.Second)), 10) + "s"
	case duration < time.Hour:
		return strconv.FormatInt(int64(duration/time.Minute), 10) + "m"
	case duration < day:
		return strconv.FormatInt(int64(duration/time.Hour), 10) + "h"
	default:
		return strconv.FormatInt(int64(duration/day), 10) + "d"
	}
}

// plainText removes escape sequences and control characters from a string
// sent by the API, so it cannot restyle or move the terminal cursor.
func plainText(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(text))
}
