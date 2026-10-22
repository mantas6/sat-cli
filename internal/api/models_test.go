package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMachineDecoding(t *testing.T) {
	t.Parallel()
	full := `{
		"name": "build-01",
		"online": true,
		"received_at": "2026-10-22T10:00:00+03:00",
		"uptime": {"milliseconds": 18000000, "for_humans": "5h", "exceeds_day": false},
		"kernel_version": "6.8.0-45-generic",
		"cpu": {"name": "AMD Ryzen 7 5800X 8-Core Processor", "temperature": 78, "usage_percentage": 87.25},
		"load_average": {"1m": 6.12, "5m": 4.8, "15m": 3.95},
		"memory": {
			"usage_percentage": 91, "usage_with_swap_percentage": 78.5, "usage_high": true,
			"used_gb": 14.1, "used_with_swap_gb": 15.2, "total_gb": 15.5, "swap_used_gb": 1.1, "swap_total_gb": 4
		},
		"disk": {
			"usage_percentage": 42, "free_percentage": 58, "usage_high": false,
			"used_gb": 196.3, "free_gb": 3.7252902984619141, "total_gb": 467, "temperature": 38
		}
	}`
	tests := []struct {
		name  string
		input string
		want  Machine // Raw is always the input
		// receivedAt is compared with time.Equal; zero means ReceivedAt is nil.
		receivedAt time.Time
	}{
		{
			name:  "full",
			input: full,
			want: Machine{
				Name:          "build-01",
				Online:        true,
				Uptime:        MachineUptime{Milliseconds: new(int64(18000000)), ForHumans: "5h"},
				KernelVersion: "6.8.0-45-generic",
				CPU:           MachineCPU{Name: "AMD Ryzen 7 5800X 8-Core Processor", Temperature: new(78.0), UsagePercentage: new(87.25)},
				LoadAverage:   MachineLoadAverage{One: new(6.12), Five: new(4.8), Fifteen: new(3.95)},
				Memory: MachineMemory{
					UsagePercentage: new(91.0), UsageWithSwapPercentage: new(78.5), UsageHigh: true,
					UsedGB: new(14.1), UsedWithSwapGB: new(15.2), TotalGB: new(15.5), SwapUsedGB: new(1.1), SwapTotalGB: new(4.0),
				},
				Disk: MachineDisk{
					UsagePercentage: new(42.0), FreePercentage: new(58.0),
					UsedGB: new(196.3), FreeGB: new(3.7252902984619141), TotalGB: new(467.0), Temperature: new(38.0),
				},
			},
			receivedAt: time.Date(2026, 10, 22, 7, 0, 0, 0, time.UTC),
		},
		{
			name: "all-null numbers",
			input: `{"name":"pi","online":false,"received_at":null,"kernel_version":null,
				"uptime":{"milliseconds":null,"for_humans":null,"exceeds_day":false},
				"cpu":{"name":null,"temperature":null,"usage_percentage":null},
				"load_average":{"1m":null,"5m":null,"15m":null},
				"memory":{"usage_percentage":null,"usage_with_swap_percentage":null,"usage_high":false,"used_gb":null,
					"used_with_swap_gb":null,"total_gb":null,"swap_used_gb":null,"swap_total_gb":null},
				"disk":{"usage_percentage":null,"free_percentage":null,"usage_high":false,"used_gb":null,"free_gb":null,
					"total_gb":null,"temperature":null}}`,
			want: Machine{Name: "pi"},
		},
		{
			name:  "null nested objects",
			input: `{"name":"pi","online":true,"uptime":null,"cpu":null,"load_average":null,"memory":null,"disk":null}`,
			want:  Machine{Name: "pi", Online: true},
		},
		{
			name:  "zero readings are kept",
			input: `{"name":"idle","cpu":{"usage_percentage":0},"memory":{"swap_total_gb":0}}`,
			want:  Machine{Name: "idle", CPU: MachineCPU{UsagePercentage: new(0.0)}, Memory: MachineMemory{SwapTotalGB: new(0.0)}},
		},
		{
			name:  "stats missing",
			input: `{"name":"pi"}`,
			want:  Machine{Name: "pi"},
		},
		{
			name:  "stats present",
			input: `{"name":"pi","stats":[{"type":"cpu","result":{"cores": [1, 2.50]}},{"type":"uptime","result":42}]}`,
			want: Machine{Name: "pi", Stats: []MachineStat{
				{Type: "cpu", Result: json.RawMessage(`{"cores": [1, 2.50]}`)},
				{Type: "uptime", Result: json.RawMessage(`42`)},
			}},
		},
		{
			name:  "stats null result",
			input: `{"name":"pi","stats":[{"type":"disk","result":null}]}`,
			want:  Machine{Name: "pi", Stats: []MachineStat{{Type: "disk", Result: json.RawMessage(`null`)}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var got Machine
			if err := json.Unmarshal([]byte(test.input), &got); err != nil {
				t.Fatal(err)
			}
			if string(got.Raw) != test.input {
				t.Fatalf("Raw = %s, want the input byte-for-byte", got.Raw)
			}
			switch {
			case test.receivedAt.IsZero() && got.ReceivedAt != nil:
				t.Fatalf("ReceivedAt = %v, want nil", got.ReceivedAt)
			case !test.receivedAt.IsZero() && (got.ReceivedAt == nil || !got.ReceivedAt.Equal(test.receivedAt)):
				t.Fatalf("ReceivedAt = %v, want %v", got.ReceivedAt, test.receivedAt)
			}
			got.ReceivedAt = nil
			test.want.Raw = json.RawMessage(test.input)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Machine = %#v, want %#v", got, test.want)
			}
		})
	}

	t.Run("raw is a copy", func(t *testing.T) {
		t.Parallel()
		input := []byte(`{"name":"pi"}`)
		var got Machine
		if err := got.UnmarshalJSON(input); err != nil {
			t.Fatal(err)
		}
		copy(input, "XXXXXXXXXXXXX")
		if string(got.Raw) != `{"name":"pi"}` {
			t.Fatalf("Raw = %s after the input buffer changed", got.Raw)
		}
	})

	t.Run("null leaves the machine unchanged", func(t *testing.T) {
		t.Parallel()
		got := Machine{Name: "kept"}
		if err := json.Unmarshal([]byte(`null`), &got); err != nil {
			t.Fatal(err)
		}
		if got.Name != "kept" || got.Raw != nil {
			t.Fatalf("Machine = %#v, want it unchanged", got)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		t.Parallel()
		var got Machine
		err := json.Unmarshal([]byte(`{"name":"pi","cpu":{"usage_percentage":"high"}}`), &got)
		var typeError *json.UnmarshalTypeError
		if !errors.As(err, &typeError) {
			t.Fatalf("Unmarshal() = %T %v, want *json.UnmarshalTypeError", err, err)
		}
	})
}

func TestMachineEnvelope(t *testing.T) {
	t.Parallel()
	type call func(ctx context.Context, client *Client) (any, error)
	list := func(ctx context.Context, c *Client) (any, error) { return c.ListMachines(ctx) }
	get := func(ctx context.Context, c *Client) (any, error) { return c.GetMachine(ctx, "alpha") }
	const (
		listMissing = "GET /api/machines: decode response: missing data"
		getMissing  = "GET /api/machines/alpha: decode response: missing data"
	)
	tests := []struct {
		name   string
		call   call
		status int // 0 means 200
		body   string
		// want is the exact error, or its prefix when prefix is true.
		want     string
		prefix   bool
		sentinel error
	}{
		{name: "list empty object", call: list, body: `{}`, want: listMissing},
		{name: "get empty object", call: get, body: `{}`, want: getMissing},
		{name: "list null data", call: list, body: `{"data":null}`, want: listMissing},
		{name: "get null data", call: get, body: `{"data":null}`, want: getMissing},
		{name: "list empty body", call: list, body: ``, want: listMissing},
		{name: "get empty body", call: get, body: ``, want: getMissing},
		{name: "list bare array", call: list, body: `[{"name":"alpha"}]`, want: "GET /api/machines: decode response: json: ", prefix: true},
		{name: "list malformed", call: list, body: `{"data":[{"name":`, want: "GET /api/machines: decode response: ", prefix: true},
		{name: "get malformed", call: get, body: `{"data":{"name":`, want: "GET /api/machines/alpha: decode response: ", prefix: true},
		{
			name: "list forbidden", call: list, status: http.StatusForbidden, body: `{"message":"denied"}`,
			want: "GET /api/machines: HTTP 403 Forbidden", prefix: true, sentinel: ErrForbidden,
		},
		{
			name: "get forbidden", call: get, status: http.StatusForbidden, body: `{"message":"denied"}`,
			want: "GET /api/machines/alpha: HTTP 403 Forbidden", prefix: true, sentinel: ErrForbidden,
		},
		{
			name: "get not found", call: get, status: http.StatusNotFound, body: `{"message":"Not found."}`,
			want: "GET /api/machines/alpha: HTTP 404 Not Found", prefix: true, sentinel: ErrNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if test.status != 0 {
					writer.WriteHeader(test.status)
				}
				_, _ = io.WriteString(writer, test.body)
			}))
			t.Cleanup(server.Close)

			_, err := test.call(context.Background(), newTestClient(t, server.URL, "token"))
			if err == nil {
				t.Fatal("call succeeded, want an error")
			}
			if test.prefix {
				if !strings.HasPrefix(err.Error(), test.want) || strings.HasSuffix(err.Error(), "missing data") {
					t.Fatalf("error = %q, want prefix %q", err, test.want)
				}
			} else if err.Error() != test.want {
				t.Fatalf("error = %q, want %q", err, test.want)
			}
			if test.sentinel != nil && !errors.Is(err, test.sentinel) {
				t.Fatalf("errors.Is(err, %v) = false", test.sentinel)
			}
		})
	}

	t.Run("list empty data", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(writer, `{"data":[]}`)
		}))
		t.Cleanup(server.Close)

		machines, err := newTestClient(t, server.URL, "token").ListMachines(context.Background())
		if err != nil || machines == nil || len(machines) != 0 {
			t.Fatalf("ListMachines() = %#v, %v; want an empty slice", machines, err)
		}
	})

	t.Run("base URL prefix", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(writer, `{}`)
		}))
		t.Cleanup(server.Close)

		_, err := newTestClient(t, server.URL+"/sat/", "token").GetMachine(context.Background(), "alpha")
		if want := "GET /sat/api/machines/alpha: decode response: missing data"; err == nil || err.Error() != want {
			t.Fatalf("GetMachine() error = %v, want %q", err, want)
		}
	})
}
