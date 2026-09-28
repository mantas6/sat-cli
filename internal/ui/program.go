package ui

import (
	"context"
	"errors"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

// Fallback terminal dimensions used when neither the options nor the output
// stream report a size.
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// Size is a terminal size in cells. A zero or negative dimension is unknown.
type Size struct {
	Width, Height int
}

// orDefault replaces unknown dimensions with the 80x24 fallback.
func (s Size) orDefault() Size {
	if s.Width <= 0 {
		s.Width = defaultWidth
	}
	if s.Height <= 0 {
		s.Height = defaultHeight
	}
	return s
}

// initialSize fills unknown dimensions of s from the terminal behind out, when
// out is one, and then from the fallback. bubbletea reports the real size
// once the program starts; this only shapes the first frame.
func initialSize(s Size, out io.Writer) Size {
	if s.Width > 0 && s.Height > 0 {
		return s
	}
	if file, ok := out.(*os.File); ok {
		if width, height, err := term.GetSize(int(file.Fd())); err == nil {
			if s.Width <= 0 {
				s.Width = width
			}
			if s.Height <= 0 {
				s.Height = height
			}
		}
	}
	return s.orDefault()
}

// errInterrupted reports that the program stopped because of SIGINT without
// ctx being cancelled. Callers map it to their own notion of a clean exit.
var errInterrupted = errors.New("interrupted")

// runProgram runs model on the given streams and maps bubbletea's shutdown
// errors the same way for every UI: once ctx is cancelled its error wins
// (bubbletea may observe the same signal first and quit gracefully or report
// tea.ErrProgramKilled), and an interrupt becomes errInterrupted.
func runProgram(ctx context.Context, model tea.Model, in io.Reader, out io.Writer, opts ...tea.ProgramOption) (tea.Model, error) {
	opts = append([]tea.ProgramOption{
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithContext(ctx),
	}, opts...)
	final, err := tea.NewProgram(model, opts...).Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return final, ctxErr
	}
	if errors.Is(err, tea.ErrInterrupted) {
		return final, errInterrupted
	}
	return final, err
}
