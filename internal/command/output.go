package command

import (
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
