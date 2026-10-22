package command

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
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

func TestMachineListRejectsArguments(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)
	if err := run(t, app, "machine", "list", "alpha"); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("Execute(machine list alpha) error = %v, want unknown command", err)
	}
}

func TestMachineErrors(t *testing.T) {
	t.Parallel()
	httpError := func(status int) *api.HTTPError {
		return &api.HTTPError{StatusCode: status, Method: http.MethodGet, Path: "/api/machines"}
	}
	tests := []struct {
		name     string
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeAPI{listMachines: func(context.Context) ([]api.Machine, error) { return nil, test.err }}
			app, stdout, _ := newTestApp(t, withAPI(client))
			err := run(t, app, "machine", "list")
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
