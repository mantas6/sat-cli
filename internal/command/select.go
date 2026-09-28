package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantas6/sat-cli/internal/ui"
)

// selectItem resolves a selection from items with app.Select. A query that
// narrows the list to one entry is returned without opening the UI;
// otherwise the interactive selector runs, which requires a terminal on both
// stdin and stdout (ui.ErrNeedsTerminal).
func selectItem(ctx context.Context, app *App, items []ui.Item, opts ui.SelectOptions) (ui.Item, error) {
	if width, height, ok := app.TermSize(); ok {
		opts.Width, opts.Height = width, height
	}

	item, err := app.Select(ctx, app.Stdin, app.Stdout, items, opts)
	switch {
	case err == nil:
		return item, nil
	case errors.Is(err, ui.ErrCancelled):
		return ui.Item{}, ExitError{Code: 130, Err: ui.ErrCancelled}
	case errors.Is(err, ui.ErrNoItems), errors.Is(err, ui.ErrNoMatch), errors.Is(err, ui.ErrNeedsTerminal):
		return ui.Item{}, err
	default:
		return ui.Item{}, fmt.Errorf("select item: %w", err)
	}
}
