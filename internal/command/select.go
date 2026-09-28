package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantas6/sat-cli/internal/ui"
)

var errSelectorNeedsTerminal = errors.New("interactive selection requires a terminal on stdin and stdout")

// selectItem resolves a selection from items. A query that narrows the list to
// one entry is returned without opening the UI; otherwise the interactive
// selector runs, which requires a terminal on both stdin and stdout.
func selectItem(ctx context.Context, app *App, items []ui.Item, opts ui.SelectOptions) (ui.Item, error) {
	item, done, err := ui.Resolve(items, opts.Query)
	if err != nil {
		return ui.Item{}, err
	}
	if done {
		return item, nil
	}

	if !app.interactive() {
		return ui.Item{}, errSelectorNeedsTerminal
	}
	if width, height, ok := app.TermSize(); ok {
		opts.Width, opts.Height = width, height
	}

	item, err = app.Select(ctx, app.Stdin, app.Stdout, items, opts)
	if errors.Is(err, ui.ErrCancelled) {
		return ui.Item{}, ExitError{Code: 130, Err: ui.ErrCancelled}
	}
	if err != nil {
		return ui.Item{}, fmt.Errorf("select item: %w", err)
	}
	return item, nil
}
