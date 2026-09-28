package command

import (
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
