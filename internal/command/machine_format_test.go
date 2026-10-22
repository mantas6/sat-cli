package command

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

// machineNow is the pinned time machine ages are measured against.
var machineNow = time.Date(2026, 10, 22, 14, 3, 12, 0, time.UTC)

// machineAgo returns a report time duration before machineNow.
func machineAgo(duration time.Duration) *time.Time {
	at := machineNow.Add(-duration)
	return &at
}

// mockupMachines are the machines of the list mockup: an offline machine
// with high disk usage, an online one with high memory usage, one that only
// reported its load and uptime in milliseconds, and a healthy one.
func mockupMachines() []api.Machine {
	uptime := int64((3*24 + 5) * time.Hour / time.Millisecond)
	return []api.Machine{
		{
			Name: "backup-nas", ReceivedAt: machineAgo(3*time.Hour + 20*time.Minute),
			Uptime:      api.MachineUptime{ForHumans: "12d"},
			CPU:         api.MachineCPU{UsagePercentage: new(2.6), Temperature: new(41.2)},
			LoadAverage: api.MachineLoadAverage{One: new(0.08)},
			Memory:      api.MachineMemory{UsagePercentage: new(40.6)},
			Disk:        api.MachineDisk{UsagePercentage: new(93.1), UsageHigh: true},
		},
		{
			Name: "build-01", Online: true, ReceivedAt: machineAgo(8 * time.Second),
			Uptime:      api.MachineUptime{ForHumans: "5h"},
			CPU:         api.MachineCPU{UsagePercentage: new(87.4), Temperature: new(78.25)},
			LoadAverage: api.MachineLoadAverage{One: new(6.12)},
			Memory:      api.MachineMemory{UsagePercentage: new(91.2), UsageHigh: true},
			Disk:        api.MachineDisk{UsagePercentage: new(42.0)},
		},
		{
			Name: "pi-garage", Online: true, ReceivedAt: machineAgo(41 * time.Second),
			Uptime:      api.MachineUptime{Milliseconds: &uptime},
			LoadAverage: api.MachineLoadAverage{One: new(0.31)},
		},
		{
			Name: "web-1", Online: true, ReceivedAt: machineAgo(12 * time.Second),
			Uptime:      api.MachineUptime{ForHumans: "34d"},
			CPU:         api.MachineCPU{UsagePercentage: new(12.0), Temperature: new(54.0)},
			LoadAverage: api.MachineLoadAverage{One: new(0.52)},
			Memory:      api.MachineMemory{UsagePercentage: new(63.0)},
			Disk:        api.MachineDisk{UsagePercentage: new(71.0)},
		},
	}
}

const machineTableMockup = `NAME        STATUS   SEEN  UPTIME  CPU  LOAD  MEM   DISK   TEMP
backup-nas  offline    3h     12d   3%  0.08  41%    93%!  41°C
build-01    online     8s      5h  87%  6.12  91%!   42%   78°C
pi-garage   online    41s      3d    -  0.31    -      -      -
web-1       online    12s     34d  12%  0.52  63%    71%   54°C
`

func TestFormatMachineTableWidths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		width int
		want  string
	}{
		{width: 0, want: machineTableMockup},
		{width: 80, want: machineTableMockup},
		// TEMP goes first.
		{width: 60, want: `NAME        STATUS   SEEN  UPTIME  CPU  LOAD  MEM   DISK
backup-nas  offline    3h     12d   3%  0.08  41%    93%!
build-01    online     8s      5h  87%  6.12  91%!   42%
pi-garage   online    41s      3d    -  0.31    -      -
web-1       online    12s     34d  12%  0.52  63%    71%
`},
		// Then LOAD and UPTIME.
		{width: 50, want: `NAME        STATUS   SEEN  CPU  MEM   DISK
backup-nas  offline    3h   3%  41%    93%!
build-01    online     8s  87%  91%!   42%
pi-garage   online    41s    -    -      -
web-1       online    12s  12%  63%    71%
`},
		// Then CPU, SEEN and the rightmost of MEM and DISK.
		{width: 30, want: `NAME        STATUS   MEM
backup-nas  offline  41%
build-01    online   91%!
pi-garage   online     -
web-1       online   63%
`},
		// Then MEM; NAME is truncated to its minimum and the lines are cut.
		{width: 12, want: `NAME      ST
backup-…  of
build-01  on
pi-gara…  on
web-1     on
`},
	}
	for _, test := range tests {
		got := formatMachineTable(mockupMachines(), machineView{Now: machineNow, Width: test.width})
		if got != test.want {
			t.Errorf("formatMachineTable() at width %d =\n%s\nwant\n%s", test.width, got, test.want)
		}
	}
}

func TestFormatMachineTableEmpty(t *testing.T) {
	t.Parallel()
	if got := formatMachineTable(nil, machineView{Now: machineNow}); got != "" {
		t.Fatalf("formatMachineTable(nil) = %q, want empty", got)
	}
}

func TestFormatMachineTableMissingReadings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		machine api.Machine
		want    string
	}{
		{
			name:    "all null",
			machine: api.Machine{Name: "blank"},
			want: "NAME   STATUS   SEEN  UPTIME  CPU  LOAD  MEM   DISK   TEMP\n" +
				"blank  offline     -       -    -     -    -      -      -\n",
		},
		{
			name: "received_at null on an online machine",
			machine: api.Machine{
				Name: "fresh", Online: true,
				Uptime: api.MachineUptime{ForHumans: "1m"},
				CPU:    api.MachineCPU{UsagePercentage: new(5.0)},
			},
			want: "NAME   STATUS  SEEN  UPTIME  CPU  LOAD  MEM   DISK   TEMP\n" +
				"fresh  online     -      1m   5%     -    -      -      -\n",
		},
		{
			// A high flag without a reading has nothing to flag.
			name: "high without a percentage",
			machine: api.Machine{
				Name: "odd", Online: true, ReceivedAt: machineAgo(0),
				Memory: api.MachineMemory{UsageHigh: true},
				Disk:   api.MachineDisk{UsageHigh: true},
			},
			want: "NAME  STATUS  SEEN  UPTIME  CPU  LOAD  MEM   DISK   TEMP\n" +
				"odd   online    0s       -    -     -    -      -      -\n",
		},
	}
	for _, test := range tests {
		got := formatMachineTable([]api.Machine{test.machine}, machineView{Now: machineNow})
		if got != test.want {
			t.Errorf("%s: formatMachineTable() =\n%s\nwant\n%s", test.name, got, test.want)
		}
	}
}

func TestFormatMachineTableStripsControlCharacters(t *testing.T) {
	t.Parallel()
	machine := api.Machine{
		Name:   "evil\x1b[31m\x1b]0;title\x07-\r\nbox\t\u009b\x00",
		Online: true,
		Uptime: api.MachineUptime{ForHumans: "\x1b[2J5h\a"},
	}
	got := formatMachineTable([]api.Machine{machine}, machineView{Now: machineNow})
	want := "NAME      STATUS  SEEN  UPTIME  CPU  LOAD  MEM   DISK   TEMP\n" +
		"evil-box  online     -      5h    -     -    -      -      -\n"
	if got != want {
		t.Fatalf("formatMachineTable() =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatMachineTablePlainHasNoEscapes(t *testing.T) {
	t.Parallel()
	for width := 0; width <= 100; width++ {
		got := formatMachineTable(mockupMachines(), machineView{Now: machineNow, Width: width})
		if strings.Contains(got, "\x1b") {
			t.Fatalf("formatMachineTable() at width %d has an escape sequence: %q", width, got)
		}
	}
}

func TestFormatMachineTableColor(t *testing.T) {
	t.Parallel()
	for _, width := range []int{0, 80, 50, 12} {
		plain := formatMachineTable(mockupMachines(), machineView{Now: machineNow, Width: width})
		colored := formatMachineTable(mockupMachines(), machineView{Now: machineNow, Width: width, Color: true})
		if colored == plain {
			t.Fatalf("formatMachineTable() at width %d is not coloured: %q", width, colored)
		}
		if got := ansi.Strip(colored); got != plain {
			t.Fatalf("ansi.Strip(coloured) at width %d =\n%s\nwant\n%s", width, got, plain)
		}
	}

	colored := formatMachineTable(mockupMachines(), machineView{Now: machineNow, Color: true})
	for _, text := range []string{"online", "offline", "93%", "91%"} {
		styled := regexp.MustCompile(`\x1b\[[0-9;]+m` + regexp.QuoteMeta(text) + `\x1b\[`)
		if !styled.MatchString(colored) {
			t.Errorf("formatMachineTable() does not style %q: %q", text, colored)
		}
	}
}

func TestPercentText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value *float64
		want  string
	}{
		{nil, "-"},
		{new(0.0), "0%"},
		{new(0.4), "0%"},
		{new(-0.2), "0%"},
		{new(42.5), "43%"},
		{new(87.4), "87%"},
		{new(99.5), "100%"},
		{new(100.0), "100%"},
	}
	for index, test := range tests {
		if got := percentText(test.value); got != test.want {
			t.Errorf("case %d: percentText() = %q, want %q", index, got, test.want)
		}
	}
}

func TestTempAndLoadText(t *testing.T) {
	t.Parallel()
	if got := tempText(nil); got != "-" {
		t.Errorf("tempText(nil) = %q, want -", got)
	}
	if got := tempText(new(78.25)); got != "78°C" {
		t.Errorf("tempText(78.25) = %q, want 78°C", got)
	}
	if got := tempText(new(-0.3)); got != "0°C" {
		t.Errorf("tempText(-0.3) = %q, want 0°C", got)
	}
	if got := loadText(nil); got != "-" {
		t.Errorf("loadText(nil) = %q, want -", got)
	}
	if got := loadText(new(6.1249)); got != "6.12" {
		t.Errorf("loadText(6.1249) = %q, want 6.12", got)
	}
}

func TestUptimeText(t *testing.T) {
	t.Parallel()
	milliseconds := func(duration time.Duration) *int64 {
		value := int64(duration / time.Millisecond)
		return &value
	}
	tests := []struct {
		uptime api.MachineUptime
		want   string
	}{
		{api.MachineUptime{}, "-"},
		{api.MachineUptime{ForHumans: "5h", Milliseconds: milliseconds(time.Minute)}, "5h"},
		{api.MachineUptime{Milliseconds: milliseconds(90 * time.Minute)}, "1h"},
		{api.MachineUptime{Milliseconds: milliseconds(12 * 24 * time.Hour)}, "12d"},
		{api.MachineUptime{ForHumans: "\x1b[1m\r\n", Milliseconds: milliseconds(0)}, "0s"},
	}
	for _, test := range tests {
		if got := uptimeText(test.uptime); got != test.want {
			t.Errorf("uptimeText(%+v) = %q, want %q", test.uptime, got, test.want)
		}
	}
}

func TestAgeText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		age  time.Duration
		want string
	}{
		{0, "0s"},
		{999 * time.Millisecond, "0s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{59*time.Minute + 59*time.Second, "59m"},
		{60 * time.Minute, "1h"},
		{23*time.Hour + 59*time.Minute, "23h"},
		{24 * time.Hour, "1d"},
		{12*24*time.Hour + 23*time.Hour, "12d"},
		{-time.Second, "0s"},
		{-48 * time.Hour, "0s"},
	}
	for _, test := range tests {
		if got := ageText(machineNow, machineNow.Add(-test.age)); got != test.want {
			t.Errorf("ageText(%v ago) = %q, want %q", test.age, got, test.want)
		}
	}
}

func TestPlainText(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"web-1":                        "web-1",
		"Ryzen™ 7 °C":                  "Ryzen™ 7 °C",
		"\x1b[31mred\x1b[0m":           "red",
		"\x1b]8;;https://x.test\x07ln": "ln",
		"tab\tline\r\nend\x7f\u0085":   "tablineend",
		"":                             "",
	}
	for input, want := range tests {
		if got := plainText(input); got != want {
			t.Errorf("plainText(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAppMachineView(t *testing.T) {
	t.Parallel()
	size := func(width int, ok bool) testOption {
		return func(app *App) {
			app.TermSize = func() (int, int, bool) { return width, 24, ok }
		}
	}
	tests := []struct {
		name string
		opts []testOption
		want machineView
	}{
		{name: "piped", opts: []testOption{size(100, true)}, want: machineView{}},
		{name: "terminal", opts: []testOption{withTTY(), size(100, true)}, want: machineView{Color: true, Width: 100}},
		{name: "unknown size", opts: []testOption{withTTY(), size(0, false)}, want: machineView{Color: true, Width: 80}},
		{name: "NO_COLOR", opts: []testOption{withTTY(), size(100, true), withEnv(map[string]string{"NO_COLOR": "1"})}, want: machineView{Width: 100}},
		{name: "dumb terminal", opts: []testOption{withTTY(), size(100, true), withEnv(map[string]string{"TERM": "dumb"})}, want: machineView{Width: 100}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			app, _, _ := newTestApp(t, test.opts...)
			app.Now = func() time.Time { return machineNow }
			test.want.Now = machineNow
			if got := app.machineView(); got != test.want {
				t.Fatalf("machineView() = %+v, want %+v", got, test.want)
			}
		})
	}
}

// machineDetailMockup is formatMachine of alpha from machineFixturesJSON.
const machineDetailMockup = `alpha  online
  Last report  2026-10-22 14:03:04 (8s ago)
  Uptime       5h
  Kernel       6.8.0-45-generic

CPU
  Model        AMD Ryzen 7 5800X <8-Core>
  Usage        87%
  Temperature  78°C
  Load         6.12  4.80  3.95  (1, 5, 15 min)

Memory
  Used         91%   14.1 / 15.5 GB  (high)
  With swap    78%   15.2 GB
  Swap         3.7 / 4.0 GB

Disk
  Used         42%   196 / 467 GB
  Free         58%   271 GB
  Temperature  38°C
`

func TestFormatMachine(t *testing.T) {
	t.Parallel()
	got := formatMachine(machineFixtures(t)[0], machineView{Now: machineNow})
	if got != machineDetailMockup {
		t.Fatalf("formatMachine() =\n%s\nwant\n%s", got, machineDetailMockup)
	}
	if lines := strings.Count(got, "\n"); lines != 20 {
		t.Fatalf("formatMachine() has %d lines, want 20", lines)
	}
}

func TestFormatMachineMissingReadings(t *testing.T) {
	t.Parallel()
	want := `blank  offline
  Last report  never
  Uptime       -
  Kernel       -

CPU
  Model        -
  Usage        -
  Temperature  -
  Load         -  -  -  (1, 5, 15 min)

Memory
  Used         -     -
  With swap    -     -
  Swap         -

Disk
  Used         -     -
  Free         -     -
  Temperature  -
`
	if got := formatMachine(api.Machine{Name: "blank"}, machineView{Now: machineNow}); got != want {
		t.Fatalf("formatMachine() =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatMachineFields(t *testing.T) {
	t.Parallel()
	// field returns the value of the first line of formatMachine that starts
	// with label.
	field := func(t *testing.T, machine api.Machine, label string) string {
		t.Helper()
		for line := range strings.Lines(formatMachine(machine, machineView{Now: machineNow})) {
			if value, ok := strings.CutPrefix(line, "  "+label+" "); ok {
				return strings.TrimSpace(value)
			}
		}
		t.Fatalf("formatMachine() has no %s field", label)
		return ""
	}
	cest := time.FixedZone("CEST", 2*60*60)
	tests := []struct {
		name    string
		machine api.Machine
		label   string
		want    string
	}{
		{
			name:    "no swap",
			machine: api.Machine{Memory: api.MachineMemory{SwapUsedGB: new(0.0), SwapTotalGB: new(0.0)}},
			label:   "Swap", want: "none",
		},
		{
			name:    "swap total only",
			machine: api.Machine{Memory: api.MachineMemory{SwapTotalGB: new(2.0)}},
			label:   "Swap", want: "- / 2.0 GB",
		},
		{
			name: "full memory",
			machine: api.Machine{Memory: api.MachineMemory{
				UsagePercentage: new(99.6), UsageHigh: true, UsedGB: new(15.45), TotalGB: new(15.5),
			}},
			label: "Used", want: "100%  15.4 / 15.5 GB  (high)",
		},
		{
			name:    "report in another time zone",
			machine: api.Machine{ReceivedAt: new(machineNow.Add(-90 * time.Second).In(cest))},
			label:   "Last report", want: "2026-10-22 14:01:42 (1m ago)",
		},
		{
			name:    "report in the future",
			machine: api.Machine{ReceivedAt: machineAgo(-time.Minute)},
			label:   "Last report", want: "2026-10-22 14:04:12 (0s ago)",
		},
		{
			name:    "control characters",
			machine: api.Machine{KernelVersion: "6.8\x1b[2J\r\n-rc1"},
			label:   "Kernel", want: "6.8-rc1",
		},
		{
			name:    "only control characters",
			machine: api.Machine{CPU: api.MachineCPU{Name: "\x1b[1m\a"}},
			label:   "Model", want: "-",
		},
	}
	for _, test := range tests {
		if got := field(t, test.machine, test.label); got != test.want {
			t.Errorf("%s: %s = %q, want %q", test.name, test.label, got, test.want)
		}
	}
}

func TestFormatMachineWidth(t *testing.T) {
	t.Parallel()
	machine := machineFixtures(t)[0]
	got := formatMachine(machine, machineView{Now: machineNow, Width: 30})
	for _, want := range []string{
		"  Last report  2026-10-22 14:…\n",
		"  Model        AMD Ryzen 7 58…\n",
		"  Used         91%   14.1 / 1…\n",
		"  Swap         3.7 / 4.0 GB\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatMachine() at width 30 does not contain %q:\n%s", want, got)
		}
	}
	for line := range strings.Lines(got) {
		if width := lipgloss.Width(strings.TrimSuffix(line, "\n")); width > 30 {
			t.Errorf("line %q is %d cells wide, want at most 30", line, width)
		}
	}

	if got := formatMachine(machine, machineView{Now: machineNow}); strings.Contains(got, "…") {
		t.Fatalf("formatMachine() without a width is truncated:\n%s", got)
	}
}

func TestFormatMachineColor(t *testing.T) {
	t.Parallel()
	machines := append(machineFixtures(t), api.Machine{Name: "blank"})
	for _, machine := range machines {
		for _, width := range []int{0, 30} {
			plain := formatMachine(machine, machineView{Now: machineNow, Width: width})
			if strings.Contains(plain, "\x1b") {
				t.Fatalf("formatMachine(%s) at width %d has an escape sequence: %q", machine.Name, width, plain)
			}
			colored := formatMachine(machine, machineView{Now: machineNow, Width: width, Color: true})
			if colored == plain {
				t.Fatalf("formatMachine(%s) at width %d is not coloured: %q", machine.Name, width, colored)
			}
			if got := ansi.Strip(colored); got != plain {
				t.Fatalf("ansi.Strip(coloured) of %s at width %d =\n%s\nwant\n%s", machine.Name, width, got, plain)
			}
		}
	}

	colored := formatMachine(machines[0], machineView{Now: machineNow, Color: true})
	for _, text := range []string{"alpha", "online", "91% ", "(high)"} {
		styled := regexp.MustCompile(`\x1b\[[0-9;]+m` + regexp.QuoteMeta(text) + `\x1b\[`)
		if !styled.MatchString(colored) {
			t.Errorf("formatMachine() does not style %q: %q", text, colored)
		}
	}
}

func TestGBText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		value *float64
		want  string
	}{
		{nil, "-"},
		{new(0.0), "0.0"},
		{new(3.7252902984619141), "3.7"},
		{new(15.5), "15.5"},
		{new(99.94), "99.9"},
		{new(99.96), "100"},
		{new(196.0), "196"},
		{new(1862.6), "1863"},
	}
	for index, test := range tests {
		if got := gbText(test.value); got != test.want {
			t.Errorf("case %d: gbText() = %q, want %q", index, got, test.want)
		}
	}
}

func TestGBPairText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		used, total *float64
		want        string
	}{
		{new(14.1), new(15.5), "14.1 / 15.5 GB"},
		{new(196.0), new(467.0), "196 / 467 GB"},
		{nil, new(15.5), "- / 15.5 GB"},
		{new(14.1), nil, "14.1 / - GB"},
		{nil, nil, "-"},
	}
	for index, test := range tests {
		if got := gbPairText(test.used, test.total); got != test.want {
			t.Errorf("case %d: gbPairText() = %q, want %q", index, got, test.want)
		}
	}
}

func TestReceivedText(t *testing.T) {
	t.Parallel()
	if got := receivedText(nil, machineNow); got != "never" {
		t.Errorf("receivedText(nil) = %q, want never", got)
	}
	vilnius := time.FixedZone("EEST", 3*60*60)
	if got, want := receivedText(machineAgo(3*time.Hour), machineNow.In(vilnius)), "2026-10-22 14:03:12 (3h ago)"; got != want {
		t.Errorf("receivedText() in EEST = %q, want %q", got, want)
	}
}

func TestMachineItem(t *testing.T) {
	t.Parallel()
	machines := machineFixtures(t)
	tests := []struct {
		machine api.Machine
		want    ui.Item
	}{
		{machines[0], ui.Item{ID: "alpha", Columns: []string{"alpha", "online", "seen 8s ago"}}},
		{machines[1], ui.Item{ID: "beta", Columns: []string{"beta", "offline", "seen 3h ago"}}},
		{api.Machine{Name: "blank"}, ui.Item{ID: "blank", Columns: []string{"blank", "offline", "never seen"}}},
		{
			api.Machine{Name: "web\x1b[31m-1\t", Online: true, ReceivedAt: machineAgo(-time.Second)},
			ui.Item{ID: "web\x1b[31m-1\t", Columns: []string{"web-1", "online", "seen 0s ago"}},
		},
	}
	for _, test := range tests {
		got := machineItem(test.machine, machineNow)
		if got.ID != test.want.ID || !slices.Equal(got.Columns, test.want.Columns) {
			t.Errorf("machineItem(%q) = %#v, want %#v", test.machine.Name, got, test.want)
		}
	}
}
