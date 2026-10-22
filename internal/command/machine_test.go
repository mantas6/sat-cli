package command

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

// listMachines returns a fakeAPI whose ListMachines returns machines.
func listMachines(machines []api.Machine) *fakeAPI {
	return &fakeAPI{listMachines: func(context.Context) ([]api.Machine, error) {
		return machines, nil
	}}
}

// withMachineNow pins App.Now to machineNow.
func withMachineNow() testOption {
	return func(app *App) { app.Now = func() time.Time { return machineNow } }
}

// withTermSize reports a terminal of width columns.
func withTermSize(width int) testOption {
	return func(app *App) {
		app.TermSize = func() (int, int, bool) { return width, 24, true }
	}
}

func TestMachineListPrintsTable(t *testing.T) {
	t.Parallel()
	machines := machineFixtures(t)
	for _, args := range [][]string{{"machine", "list"}, {"machines", "ls"}} {
		app, stdout, stderr := newTestApp(t, withAPI(listMachines(machines)), withMachineNow())
		if err := run(t, app, args...); err != nil {
			t.Fatal(err)
		}
		want := formatMachineTable(machines, machineView{Now: machineNow})
		if got := stdout.String(); got != want || !strings.Contains(got, "alpha") {
			t.Fatalf("%v stdout =\n%s\nwant\n%s", args, got, want)
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v stderr = %q, want empty", args, stderr.String())
		}
	}
}

func TestMachineListJSON(t *testing.T) {
	t.Parallel()
	app, stdout, _ := newTestApp(t, withAPI(listMachines(machineFixtures(t))), withTTY())
	if err := run(t, app, "machine", "list", "--json"); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil || len(decoded) != 2 {
		t.Fatalf("stdout is not an array of two machines (%v): %s", err, got)
	}
	for _, want := range []string{`"swap_used_gb": 3.7252902984619141`, "<8-Core>", "[\n  {\n    \"name\": \"alpha\""} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "]\n") || strings.Contains(got, `\u003c`) || strings.Contains(got, "\x1b") {
		t.Fatalf("stdout = %q, want an uncoloured array without HTML escapes ending in a newline", got)
	}
}

func TestMachineListJSONWithoutRawEncodesMachine(t *testing.T) {
	t.Parallel()
	machine := api.Machine{Name: "a<b", Online: true, CPU: api.MachineCPU{UsagePercentage: new(12.5)}}
	app, stdout, _ := newTestApp(t, withAPI(listMachines([]api.Machine{machine})))
	if err := run(t, app, "machine", "list", "--json"); err != nil {
		t.Fatal(err)
	}
	var decoded []api.Machine
	if err := json.Unmarshal(stdout.Bytes(), &decoded); err != nil || len(decoded) != 1 {
		t.Fatalf("stdout is not an array of one machine (%v): %s", err, stdout.String())
	}
	got := decoded[0]
	if got.Name != "a<b" || !got.Online || got.CPU.UsagePercentage == nil || *got.CPU.UsagePercentage != 12.5 {
		t.Fatalf("decoded machine = %+v, want %+v", got, machine)
	}
	if !strings.Contains(stdout.String(), `"name": "a<b"`) || !strings.HasSuffix(stdout.String(), "]\n") {
		t.Fatalf("stdout = %q, want indented JSON without HTML escapes", stdout.String())
	}
}

func TestMachineListEmpty(t *testing.T) {
	t.Parallel()
	app, stdout, stderr := newTestApp(t, withAPI(listMachines(nil)))
	if err := run(t, app, "machine", "list"); err != nil {
		t.Fatal(err)
	}
	if stdout.Len() != 0 || stderr.String() != "No machines.\n" {
		t.Fatalf("stdout = %q, stderr = %q, want nothing and No machines.", stdout.String(), stderr.String())
	}

	app, stdout, stderr = newTestApp(t, withAPI(listMachines(nil)))
	if err := run(t, app, "machine", "list", "--json"); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "[]\n" || stderr.Len() != 0 {
		t.Fatalf("--json stdout = %q, stderr = %q, want [] and nothing", stdout.String(), stderr.String())
	}
}

func TestMachineListOnTerminal(t *testing.T) {
	t.Parallel()
	machines := mockupMachines()

	t.Run("colour", func(t *testing.T) {
		t.Parallel()
		app, stdout, _ := newTestApp(t, withAPI(listMachines(machines)), withMachineNow(), withTTY(), withTermSize(100))
		if err := run(t, app, "machine", "list"); err != nil {
			t.Fatal(err)
		}
		want := formatMachineTable(machines, machineView{Now: machineNow, Color: true, Width: 100})
		if got := stdout.String(); got != want || !strings.Contains(got, "\x1b[") {
			t.Fatalf("stdout = %q, want coloured %q", got, want)
		}
	})

	t.Run("NO_COLOR", func(t *testing.T) {
		t.Parallel()
		app, stdout, _ := newTestApp(t, withAPI(listMachines(machines)), withMachineNow(), withTTY(), withTermSize(100),
			withEnv(map[string]string{"NO_COLOR": "1"}))
		if err := run(t, app, "machine", "list"); err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); got != machineTableMockup {
			t.Fatalf("stdout =\n%q\nwant\n%q", got, machineTableMockup)
		}
	})

	t.Run("narrow", func(t *testing.T) {
		t.Parallel()
		app, stdout, _ := newTestApp(t, withAPI(listMachines(machines)), withMachineNow(), withTTY(), withTermSize(50),
			withEnv(map[string]string{"TERM": "dumb"}))
		if err := run(t, app, "machine", "list"); err != nil {
			t.Fatal(err)
		}
		header, _, _ := strings.Cut(stdout.String(), "\n")
		if header != "NAME        STATUS   SEEN  CPU  MEM   DISK" {
			t.Fatalf("header = %q, want TEMP, LOAD and UPTIME dropped", header)
		}
	})
}

func TestMachineErrors(t *testing.T) {
	t.Parallel()
	httpError := func(status int) *api.HTTPError {
		return &api.HTTPError{StatusCode: status, Method: http.MethodGet, Path: "/api/machines"}
	}
	show := []string{"machine", "show", "ghost"}
	tests := []struct {
		name string
		// args default to machine list. Every case runs on a terminal, so a
		// show without a name lists the machines for the picker.
		args     []string
		err      *api.HTTPError
		sentinel error
		want     func(message string) bool
	}{
		{
			name: "forbidden", err: httpError(http.StatusForbidden), sentinel: api.ErrForbidden,
			want: func(message string) bool {
				return message == "the API token lacks permission to inspect machines (HTTP 403); use a token that has it and run `sat auth login`"
			},
		},
		{
			name: "unauthorized", err: httpError(http.StatusUnauthorized), sentinel: api.ErrUnauthorized,
			want: func(message string) bool {
				return strings.HasPrefix(message, "GET /api/machines: HTTP 401") && strings.HasSuffix(message, "; run `sat auth login`")
			},
		},
		{
			name: "not found while listing", err: httpError(http.StatusNotFound), sentinel: api.ErrNotFound,
			want: func(message string) bool { return message == httpError(http.StatusNotFound).Error() },
		},
		{
			name: "server error", err: httpError(http.StatusInternalServerError),
			want: func(message string) bool { return message == httpError(http.StatusInternalServerError).Error() },
		},
		{
			name: "show not found", args: show, err: httpError(http.StatusNotFound), sentinel: api.ErrNotFound,
			want: func(message string) bool {
				return message == "machine \"ghost\" not found; run `sat machine list` to see known machines"
			},
		},
		{
			name: "show forbidden", args: show, err: httpError(http.StatusForbidden), sentinel: api.ErrForbidden,
			want: func(message string) bool {
				return message == "the API token lacks permission to inspect machines (HTTP 403); use a token that has it and run `sat auth login`"
			},
		},
		{
			name: "show unauthorized", args: show, err: httpError(http.StatusUnauthorized), sentinel: api.ErrUnauthorized,
			want: func(message string) bool {
				return strings.HasPrefix(message, "GET /api/machines: HTTP 401") && strings.HasSuffix(message, "; run `sat auth login`")
			},
		},
		{
			name: "show server error", args: show, err: httpError(http.StatusInternalServerError),
			want: func(message string) bool { return message == httpError(http.StatusInternalServerError).Error() },
		},
		{
			name: "picker not found", args: []string{"machine", "show"}, err: httpError(http.StatusNotFound), sentinel: api.ErrNotFound,
			want: func(message string) bool { return message == httpError(http.StatusNotFound).Error() },
		},
		{
			name: "picker forbidden", args: []string{"machine", "show"}, err: httpError(http.StatusForbidden), sentinel: api.ErrForbidden,
			want: func(message string) bool {
				return message == "the API token lacks permission to inspect machines (HTTP 403); use a token that has it and run `sat auth login`"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeAPI{
				listMachines: func(context.Context) ([]api.Machine, error) { return nil, test.err },
				getMachine:   func(context.Context, string) (api.Machine, error) { return api.Machine{}, test.err },
			}
			args := test.args
			if args == nil {
				args = []string{"machine", "list"}
			}
			app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
			err := run(t, app, args...)
			if err == nil || !test.want(err.Error()) {
				t.Fatalf("err = %v, unexpected wording", err)
			}
			if test.sentinel != nil && !errors.Is(err, test.sentinel) {
				t.Errorf("err = %v, want errors.Is %v", err, test.sentinel)
			}
			var httpErr *api.HTTPError
			if !errors.As(err, &httpErr) || httpErr != test.err {
				t.Errorf("errors.As(err, *api.HTTPError) = %v, want the original error", httpErr)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}
		})
	}

	t.Run("not found for a name", func(t *testing.T) {
		t.Parallel()
		original := httpError(http.StatusNotFound)
		err := machineError(original, "web\x1b1")
		if got, want := err.Error(), "machine \"web\\x1b1\" not found; run `sat machine list` to see known machines"; got != want {
			t.Fatalf("machineError() = %q, want %q", got, want)
		}
		var httpErr *api.HTTPError
		if !errors.Is(err, api.ErrNotFound) || !errors.As(err, &httpErr) || httpErr != original {
			t.Fatalf("machineError() = %v, want errors.Is ErrNotFound and errors.As the original", err)
		}
	})

	t.Run("other errors pass through", func(t *testing.T) {
		t.Parallel()
		original := errors.New("connection refused")
		if got := machineError(original, "alpha"); got != original {
			t.Fatalf("machineError() = %v, want the original error", got)
		}
		if got := machineError(nil, ""); got != nil {
			t.Fatalf("machineError(nil) = %v, want nil", got)
		}
	})
}

// getMachine returns a fakeAPI whose GetMachine returns the machine with the
// requested name from machines and records the names it was asked for.
func getMachine(t *testing.T, machines []api.Machine, names *[]string) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		listMachines: func(context.Context) ([]api.Machine, error) { return machines, nil },
		getMachine: func(_ context.Context, name string) (api.Machine, error) {
			if names != nil {
				*names = append(*names, name)
			}
			for _, machine := range machines {
				if machine.Name == name {
					return machine, nil
				}
			}
			t.Errorf("GetMachine(%q), want a known machine", name)
			return api.Machine{}, &api.HTTPError{StatusCode: http.StatusNotFound}
		},
	}
}

func TestMachineShowByName(t *testing.T) {
	t.Parallel()
	machines := machineFixtures(t)
	var names []string
	app, stdout, stderr := newTestApp(t, withAPI(getMachine(t, machines, &names)), withMachineNow(), withTTY())
	app.TermSize = func() (int, int, bool) { return 100, 24, true }
	if err := run(t, app, "machine", "show", "  alpha \t"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{"alpha"}) {
		t.Fatalf("GetMachine names = %q, want [alpha]", names)
	}
	want := formatMachine(machines[0], machineView{Now: machineNow, Color: true, Width: 100})
	if got := stdout.String(); got != want || !strings.Contains(got, "\x1b[") {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	app, stdout, _ = newTestApp(t, withAPI(getMachine(t, machines, nil)), withMachineNow())
	if err := run(t, app, "machines", "show", "alpha"); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != machineDetailMockup {
		t.Fatalf("piped stdout =\n%s\nwant\n%s", got, machineDetailMockup)
	}
}

// statsMachine is a machine as GetMachine decodes it, with stats modules
// whose number literals must survive.
func statsMachine(t *testing.T) api.Machine {
	t.Helper()
	data := `{"name":"alpha","online":true,"memory":{"swap_used_gb":3.7252902984619141},` +
		`"stats":[{"type":"cpu","result":{"load":[1.10,0.5e1]}},{"type":"note","result":"<ok>"}]}`
	var machine api.Machine
	if err := json.Unmarshal([]byte(data), &machine); err != nil {
		t.Fatal(err)
	}
	return machine
}

func TestMachineShowJSON(t *testing.T) {
	t.Parallel()
	app, stdout, _ := newTestApp(t, withAPI(getMachine(t, []api.Machine{statsMachine(t)}, nil)), withTTY())
	if err := run(t, app, "machine", "show", "alpha", "--json"); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"{\n  \"name\": \"alpha\",\n", `"swap_used_gb": 3.7252902984619141`, `1.10`, `0.5e1`, `"<ok>"`} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout does not contain %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "}\n") || strings.Contains(got, "\x1b") {
		t.Fatalf("stdout = %q, want an uncoloured object ending in a newline", got)
	}
}

func TestMachineShowRaw(t *testing.T) {
	t.Parallel()
	app, stdout, _ := newTestApp(t, withAPI(getMachine(t, []api.Machine{statsMachine(t)}, nil)), withTTY())
	if err := run(t, app, "machine", "show", "alpha", "--raw"); err != nil {
		t.Fatal(err)
	}
	want := `[
  {
    "type": "cpu",
    "result": {
      "load": [
        1.10,
        0.5e1
      ]
    }
  },
  {
    "type": "note",
    "result": "<ok>"
  }
]
`
	if got := stdout.String(); got != want {
		t.Fatalf("stdout =\n%s\nwant\n%s", got, want)
	}
}

func TestMachineShowRawWithoutStats(t *testing.T) {
	t.Parallel()
	decoded := func(data string) api.Machine {
		var machine api.Machine
		if err := json.Unmarshal([]byte(data), &machine); err != nil {
			t.Fatal(err)
		}
		return machine
	}
	tests := []struct {
		name    string
		machine api.Machine
		want    string
	}{
		{name: "missing", machine: decoded(`{"name":"alpha"}`), want: "[]\n"},
		{name: "null", machine: decoded(`{"name":"alpha","stats":null}`), want: "[]\n"},
		{name: "empty", machine: decoded(`{"name":"alpha","stats":[]}`), want: "[]\n"},
		{name: "decoded without stats", machine: api.Machine{Name: "alpha"}, want: "[]\n"},
		{
			name: "decoded with stats",
			machine: api.Machine{Name: "alpha", Stats: []api.MachineStat{
				{Type: "note", Result: json.RawMessage(`"<ok>"`)},
				{Type: "empty"},
			}},
			want: "[\n  {\n    \"type\": \"note\",\n    \"result\": \"<ok>\"\n  },\n" +
				"  {\n    \"type\": \"empty\",\n    \"result\": null\n  }\n]\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			app, stdout, _ := newTestApp(t, withAPI(getMachine(t, []api.Machine{test.machine}, nil)))
			if err := run(t, app, "machine", "show", "alpha", "--raw"); err != nil {
				t.Fatal(err)
			}
			if got := stdout.String(); got != test.want {
				t.Fatalf("stdout = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMachineShowValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"  "}, want: "machine name must not be empty"},
		{args: []string{""}, want: "machine name must not be empty"},
		{args: []string{"alpha", "--json", "--raw"}, want: "if any flags in the group [json raw follow] are set none of the others can be"},
	}
	for _, test := range tests {
		client := &fakeAPI{getMachine: func(context.Context, string) (api.Machine, error) {
			t.Errorf("%q: unexpected call to GetMachine", test.args)
			return api.Machine{}, nil
		}}
		app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
		err := run(t, app, append([]string{"machine", "show"}, test.args...)...)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%q: err = %v, want %q", test.args, err, test.want)
		}
		if stdout.Len() != 0 {
			t.Errorf("%q: stdout = %q, want empty", test.args, stdout.String())
		}
	}
}

func TestMachineShowPicker(t *testing.T) {
	t.Parallel()
	machines := machineFixtures(t)
	var names []string
	var gotItems []ui.Item
	var gotOpts ui.SelectOptions
	app, stdout, _ := newTestApp(t, withAPI(getMachine(t, machines, &names)), withMachineNow(), withTTY(),
		withEnv(map[string]string{"NO_COLOR": "1"}))
	app.TermSize = func() (int, int, bool) { return 100, 30, true }
	app.Select = selectIndex(1, &gotItems, &gotOpts)
	if err := run(t, app, "machine", "show"); err != nil {
		t.Fatal(err)
	}
	if gotOpts.Title != "Machines" || gotOpts.Query != "" || gotOpts.Width != 100 || gotOpts.Height != 30 {
		t.Fatalf("Select options = %+v, want title Machines at 100x30", gotOpts)
	}
	if len(gotItems) != len(machines) {
		t.Fatalf("Select items = %+v, want one per machine", gotItems)
	}
	for index, item := range gotItems {
		if want := machineItem(machines[index], machineNow); item.ID != machines[index].Name || !slices.Equal(item.Columns, want.Columns) {
			t.Errorf("item %d = %+v, want %+v", index, item, want)
		}
	}
	if !slices.Equal(names, []string{"beta"}) {
		t.Fatalf("GetMachine names = %q, want [beta]", names)
	}
	if got, want := stdout.String(), formatMachine(machines[1], machineView{Now: machineNow, Width: 100}); got != want {
		t.Fatalf("stdout =\n%s\nwant\n%s", got, want)
	}
}

func TestMachineShowWithoutNameNeedsTerminal(t *testing.T) {
	t.Parallel()
	terminal := func(stream any) testOption {
		return func(app *App) {
			app.IsTTY = func(candidate any) bool { return candidate == stream }
		}
	}
	for name, opt := range map[string]testOption{
		"piped":        func(*App) {},
		"stdout piped": func(app *App) { terminal(app.Stdin)(app) },
		"stdin piped":  func(app *App) { terminal(app.Stdout)(app) },
	} {
		client := &fakeAPI{listMachines: func(context.Context) ([]api.Machine, error) {
			t.Errorf("%s: unexpected call to ListMachines", name)
			return nil, nil
		}}
		app, _, _ := newTestApp(t, withAPI(client), opt)
		if err := run(t, app, "machine", "show"); !errors.Is(err, errMachineNameRequired) {
			t.Errorf("%s: err = %v, want errMachineNameRequired", name, err)
		}
	}
}

func TestMachineShowPickerCancelled(t *testing.T) {
	t.Parallel()
	client := getMachine(t, machineFixtures(t), nil)
	client.getMachine = func(context.Context, string) (api.Machine, error) {
		t.Error("unexpected call to GetMachine")
		return api.Machine{}, nil
	}
	app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
	app.Select = func(context.Context, io.Reader, io.Writer, []ui.Item, ui.SelectOptions) (ui.Item, error) {
		return ui.Item{}, ui.ErrCancelled
	}
	err := run(t, app, "machine", "show")
	var exitError ExitError
	if !errors.As(err, &exitError) || exitError.Code != 130 || !errors.Is(err, ui.ErrCancelled) {
		t.Fatalf("err = %#v, want cancellation exit code 130", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestMachineShowPickerWithoutMachines(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t, withAPI(listMachines(nil)), withTTY(), withRealSelect())
	if err := run(t, app, "machine", "show"); !errors.Is(err, ui.ErrNoItems) {
		t.Fatalf("err = %v, want ui.ErrNoItems", err)
	}
}

// followOnce returns a Follow stub that records its options, switches the
// terminal to width columns and calls the fetcher once, storing its text.
func followOnce(app *App, width int, gotOpts *ui.DashboardOptions, gotText *string) FollowFunc {
	return func(ctx context.Context, _ io.Reader, _ io.Writer, fetch ui.Fetcher, opts ui.DashboardOptions) error {
		*gotOpts = opts
		app.TermSize = func() (int, int, bool) { return width, 40, true }
		text, err := fetch(ctx)
		*gotText = text
		return err
	}
}

func TestMachineShowFollowWiring(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		args   []string
		picker bool
		want   time.Duration
	}{
		{name: "flag without value", args: []string{"machine", "show", "alpha", "--follow"}, want: 5 * time.Second},
		{name: "short flag and positional interval", args: []string{"machine", "show", "alpha", "-f", "10s"}, want: 10 * time.Second},
		{name: "equals value", args: []string{"machines", "show", "alpha", "--follow=10s"}, want: 10 * time.Second},
		{name: "picker", args: []string{"machine", "show", "--follow"}, picker: true, want: 5 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			machines := machineFixtures(t)
			var names []string
			var deadlines []bool
			client := getMachine(t, machines, nil)
			lookup := client.getMachine
			client.getMachine = func(ctx context.Context, name string) (api.Machine, error) {
				names = append(names, name)
				_, ok := ctx.Deadline()
				deadlines = append(deadlines, ok)
				return lookup(ctx, name)
			}
			app, stdout, _ := newTestApp(t, withAPI(client), withMachineNow(), withTTY())
			app.TermSize = func() (int, int, bool) { return 120, 40, true }
			if test.picker {
				app.Select = selectIndex(0, nil, nil)
			}
			var gotOpts ui.DashboardOptions
			var gotText string
			app.Follow = followOnce(app, 90, &gotOpts, &gotText)

			if err := run(t, app, test.args...); err != nil {
				t.Fatal(err)
			}
			want := ui.DashboardOptions{Size: ui.Size{Width: 120, Height: 40}, Interval: test.want, Label: "machine alpha"}
			if gotOpts != want {
				t.Fatalf("follow options = %#v, want %#v", gotOpts, want)
			}
			// The fetcher reads the terminal width on every refresh.
			if wantText := formatMachine(machines[0], machineView{Now: machineNow, Color: true, Width: 90}); gotText != wantText {
				t.Fatalf("fetch() =\n%s\nwant\n%s", gotText, wantText)
			}
			if !slices.Equal(names, []string{"alpha", "alpha"}) {
				t.Fatalf("GetMachine names = %q, want alpha at startup and on one refresh", names)
			}
			if !deadlines[1] {
				t.Fatal("GetMachine() context of the refresh has no request timeout")
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func TestMachineShowFollowValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"alpha", "--follow=0s"}, want: "machine follow interval must be greater than zero"},
		{args: []string{"alpha", "--follow=-1s"}, want: "machine follow interval must be greater than zero"},
		{args: []string{"alpha", "--follow", "invalid"}, want: `invalid machine follow interval "invalid"`},
		{args: []string{"alpha", "10s"}, want: "machine follow interval requires --follow"},
		{args: []string{"alpha", "--follow", "1s", "2s"}, want: "accepts between 0 and 2 arg(s), received 3"},
		{args: []string{"alpha", "--follow", "--json"}, want: "if any flags in the group [json raw follow] are set none of the others can be"},
		{args: []string{"alpha", "--follow", "--raw"}, want: "if any flags in the group [json raw follow] are set none of the others can be"},
	}
	for _, test := range tests {
		client := &fakeAPI{
			listMachines: func(context.Context) ([]api.Machine, error) {
				t.Errorf("%q: unexpected call to ListMachines", test.args)
				return nil, nil
			},
			getMachine: func(context.Context, string) (api.Machine, error) {
				t.Errorf("%q: unexpected call to GetMachine", test.args)
				return api.Machine{}, nil
			},
		}
		app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
		err := run(t, app, append([]string{"machine", "show"}, test.args...)...)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%q: err = %v, want %q", test.args, err, test.want)
		}
		if stdout.Len() != 0 {
			t.Errorf("%q: stdout = %q, want empty", test.args, stdout.String())
		}
	}
}

func TestMachineFollowRequiresTerminal(t *testing.T) {
	t.Parallel()
	terminal := func(stream func(*App) any) testOption {
		return func(app *App) {
			want := stream(app)
			app.IsTTY = func(candidate any) bool { return candidate == want }
		}
	}
	modes := map[string]testOption{
		"piped":        func(*App) {},
		"stdout piped": terminal(func(app *App) any { return app.Stdin }),
		"stdin piped":  terminal(func(app *App) any { return app.Stdout }),
	}
	commands := [][]string{
		{"machine", "show", "alpha", "--follow"},
		{"machine", "show", "--follow"},
		{"machine", "list", "--follow"},
	}
	for name, mode := range modes {
		for _, args := range commands {
			client := &fakeAPI{
				listMachines: func(context.Context) ([]api.Machine, error) {
					t.Errorf("%s %q: unexpected call to ListMachines", name, args)
					return nil, nil
				},
				getMachine: func(context.Context, string) (api.Machine, error) {
					t.Errorf("%s %q: unexpected call to GetMachine", name, args)
					return api.Machine{}, nil
				},
			}
			// Follow fails the test if run.
			app, _, _ := newTestApp(t, withAPI(client), mode)
			if err := run(t, app, args...); !errors.Is(err, errMachineFollowNeedsTerminal) {
				t.Errorf("%s %q: err = %v, want errMachineFollowNeedsTerminal", name, args, err)
			}
		}
	}
}

func TestMachineShowFollowStartupFailure(t *testing.T) {
	t.Parallel()
	notFound := &api.HTTPError{StatusCode: http.StatusNotFound, Method: http.MethodGet, Path: "/api/machines/10s"}
	forbidden := &api.HTTPError{StatusCode: http.StatusForbidden, Method: http.MethodGet, Path: "/api/machines/10s"}
	tests := []struct {
		name     string
		args     []string
		err      *api.HTTPError
		sentinel error
		want     string
	}{
		{
			name: "duration as name", args: []string{"machine", "show", "--follow", "10s"}, err: notFound, sentinel: api.ErrNotFound,
			want: "machine \"10s\" not found; run `sat machine list` to see known machines; " +
				"use --follow=10s to pick a machine with a custom interval",
		},
		{
			name: "unknown name", args: []string{"machine", "show", "ghost", "--follow"}, err: notFound, sentinel: api.ErrNotFound,
			want: "machine \"ghost\" not found; run `sat machine list` to see known machines",
		},
		{
			name: "forbidden", args: []string{"machine", "show", "10s", "--follow"}, err: forbidden, sentinel: api.ErrForbidden,
			want: "the API token lacks permission to inspect machines (HTTP 403); use a token that has it and run `sat auth login`",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			client := &fakeAPI{getMachine: func(context.Context, string) (api.Machine, error) {
				calls++
				return api.Machine{}, test.err
			}}
			// Follow fails the test if run.
			app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
			err := run(t, app, test.args...)
			if err == nil || err.Error() != test.want {
				t.Fatalf("err = %v, want %q", err, test.want)
			}
			var httpErr *api.HTTPError
			if !errors.Is(err, test.sentinel) || !errors.As(err, &httpErr) || httpErr != test.err {
				t.Fatalf("err = %v, want errors.Is %v and errors.As the original", err, test.sentinel)
			}
			if calls != 1 || stdout.Len() != 0 {
				t.Fatalf("GetMachine calls = %d, stdout = %q, want one call and no output", calls, stdout.String())
			}
		})
	}
}

func TestMachineListFollowWiring(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want time.Duration
	}{
		{name: "flag without value", args: []string{"machine", "list", "--follow"}, want: 5 * time.Second},
		{name: "alias, short flag and positional interval", args: []string{"machines", "ls", "-f", "10s"}, want: 10 * time.Second},
		{name: "equals value", args: []string{"machine", "list", "--follow=10s"}, want: 10 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			machines := machineFixtures(t)
			var deadlines []bool
			client := &fakeAPI{listMachines: func(ctx context.Context) ([]api.Machine, error) {
				_, ok := ctx.Deadline()
				deadlines = append(deadlines, ok)
				return machines, nil
			}}
			app, stdout, _ := newTestApp(t, withAPI(client), withMachineNow(), withTTY())
			app.TermSize = func() (int, int, bool) { return 120, 40, true }
			var gotOpts ui.DashboardOptions
			var gotText string
			app.Follow = followOnce(app, 60, &gotOpts, &gotText)

			if err := run(t, app, test.args...); err != nil {
				t.Fatal(err)
			}
			want := ui.DashboardOptions{Size: ui.Size{Width: 120, Height: 40}, Interval: test.want, Label: "machines"}
			if gotOpts != want {
				t.Fatalf("follow options = %#v, want %#v", gotOpts, want)
			}
			if wantText := formatMachineTable(machines, machineView{Now: machineNow, Color: true, Width: 60}); gotText != wantText {
				t.Fatalf("fetch() =\n%s\nwant\n%s", gotText, wantText)
			}
			if len(deadlines) != 2 || !deadlines[1] {
				t.Fatalf("ListMachines deadlines = %v, want a startup call and a refresh with a request timeout", deadlines)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		app, _, stderr := newTestApp(t, withAPI(listMachines(nil)), withTTY())
		var gotOpts ui.DashboardOptions
		var gotText string
		app.Follow = followOnce(app, 80, &gotOpts, &gotText)
		if err := run(t, app, "machine", "list", "--follow"); err != nil {
			t.Fatal(err)
		}
		if gotText != "No machines.\n" || stderr.Len() != 0 {
			t.Fatalf("fetch() = %q, stderr = %q, want No machines. and nothing", gotText, stderr.String())
		}
	})
}

func TestMachineListFollowValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--follow=0s"}, want: "machine follow interval must be greater than zero"},
		{args: []string{"--follow", "invalid"}, want: `invalid machine follow interval "invalid"`},
		{args: []string{"10s"}, want: "machine follow interval requires --follow"},
		{args: []string{"alpha"}, want: "machine follow interval requires --follow"},
		{args: []string{"--follow", "1s", "2s"}, want: "accepts at most 1 arg(s), received 2"},
		{args: []string{"--follow", "--json"}, want: "if any flags in the group [json follow] are set none of the others can be"},
	}
	for _, test := range tests {
		client := &fakeAPI{listMachines: func(context.Context) ([]api.Machine, error) {
			t.Errorf("%q: unexpected call to ListMachines", test.args)
			return nil, nil
		}}
		app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
		err := run(t, app, append([]string{"machine", "list"}, test.args...)...)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%q: err = %v, want %q", test.args, err, test.want)
		}
		if stdout.Len() != 0 {
			t.Errorf("%q: stdout = %q, want empty", test.args, stdout.String())
		}
	}
}

func TestMachineListFollowStartupFailure(t *testing.T) {
	t.Parallel()
	forbidden := &api.HTTPError{StatusCode: http.StatusForbidden, Method: http.MethodGet, Path: "/api/machines"}
	calls := 0
	client := &fakeAPI{listMachines: func(context.Context) ([]api.Machine, error) {
		calls++
		return nil, forbidden
	}}
	// Follow fails the test if run.
	app, _, _ := newTestApp(t, withAPI(client), withTTY())
	err := run(t, app, "machine", "list", "--follow")
	if !errors.Is(err, api.ErrForbidden) || !strings.Contains(err.Error(), "lacks permission to inspect machines") {
		t.Fatalf("err = %v, want the reworded 403", err)
	}
	if calls != 1 {
		t.Fatalf("ListMachines calls = %d, want 1", calls)
	}
}
