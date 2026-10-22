package command

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// writeLine writes text and terminates it with a newline unless it already
// ends with one.
func writeLine(w io.Writer, text string) error {
	if _, err := io.WriteString(w, text); err != nil {
		return err
	}
	if strings.HasSuffix(text, "\n") {
		return nil
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// writeJSON writes value as JSON indented by two spaces with a trailing
// newline. HTML characters are not escaped and json.RawMessage values keep
// their number literals.
func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// indentJSON indents raw by two spaces and adds a trailing newline. Number
// literals and escapes are kept as they are.
func indentJSON(raw json.RawMessage) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
