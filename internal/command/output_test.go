package command

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	value := map[string]any{
		"html": "<a & b>",
		"raw":  json.RawMessage(`{"n":3.7252902984619141,"list":[1,2]}`),
	}
	if err := writeJSON(&out, value); err != nil {
		t.Fatal(err)
	}
	want := `{
  "html": "<a & b>",
  "raw": {
    "n": 3.7252902984619141,
    "list": [
      1,
      2
    ]
  }
}
`
	if got := out.String(); got != want {
		t.Fatalf("writeJSON() =\n%s\nwant\n%s", got, want)
	}
}

func TestWriteJSONEmptySlice(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := writeJSON(&out, []any{}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "[]\n" {
		t.Fatalf("writeJSON([]) = %q, want []\\n", got)
	}
}
