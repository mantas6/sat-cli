package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

// selectorFunc runs an interactive selector. Tests replace runSelector to
// avoid driving a real terminal.
type selectorFunc func(context.Context, io.Reader, io.Writer, []ui.Item, ui.SelectOptions) (ui.Item, error)

var runSelector selectorFunc = ui.Select

var errSelectorNeedsTerminal = errors.New("interactive selection requires a terminal on stdin and stdout")

// selectItem resolves a selection from items. A query that narrows the list to
// one entry is returned without opening the UI; otherwise the interactive
// selector runs, which requires a terminal on both stdin and stdout.
func selectItem(cmd *cobra.Command, app *App, items []ui.Item, opts ui.SelectOptions) (ui.Item, error) {
	item, done, err := ui.Resolve(items, opts.Query)
	if err != nil {
		return ui.Item{}, err
	}
	if done {
		return item, nil
	}

	if !interactiveStreams(app) {
		return ui.Item{}, errSelectorNeedsTerminal
	}
	fillTerminalSize(app, &opts)

	item, err = runSelector(cmd.Context(), app.Stdin, app.Stdout, items, opts)
	if errors.Is(err, ui.ErrCancelled) {
		return ui.Item{}, ExitError{Code: 130, Err: ui.ErrCancelled}
	}
	if err != nil {
		return ui.Item{}, fmt.Errorf("select item: %w", err)
	}
	return item, nil
}

// interactiveStreams reports whether stdin and stdout are both terminals.
// Streams that are not *os.File are treated as terminals only when the
// selector has been stubbed, which keeps command tests hermetic.
func interactiveStreams(app *App) bool {
	input, inputIsFile := app.Stdin.(*os.File)
	output, outputIsFile := app.Stdout.(*os.File)
	if !inputIsFile || !outputIsFile {
		return selectorStubbed()
	}
	return app.IsTerminal(int(input.Fd())) && app.IsTerminal(int(output.Fd()))
}

func fillTerminalSize(app *App, opts *ui.SelectOptions) {
	output, ok := app.Stdout.(*os.File)
	if !ok || (opts.Width != 0 && opts.Height != 0) {
		return
	}
	width, height, err := app.TerminalSize(int(output.Fd()))
	if err != nil {
		return
	}
	if opts.Width == 0 {
		opts.Width = width
	}
	if opts.Height == 0 {
		opts.Height = height
	}
}

// selectorStubbed reports whether tests replaced the real selector.
func selectorStubbed() bool {
	return fmt.Sprintf("%p", runSelector) != fmt.Sprintf("%p", selectorFunc(ui.Select))
}
