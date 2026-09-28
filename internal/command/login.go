package command

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const maxURLPromptAttempts = 3

func newLoginCommand(app *App) *cobra.Command {
	var replaceURL bool
	var urlOnly bool

	command := &cobra.Command{
		Use:   "login",
		Short: "Configure the Satellite URL and authentication token",
		Long: `Configure the Satellite URL and authentication token.

The URL is prompted for when it is missing. Use --replace-url to prompt for it
even when configured. The token is always prompted for and replaced; use
--url-only to prompt for and replace only the URL. Piped input is read as plain
text, while token input from a terminal is hidden.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			prompter := loginPrompter{
				app:    app,
				reader: bufio.NewReader(app.Stdin),
			}

			if !app.Config.HasBaseURL() || replaceURL || urlOnly {
				if err := prompter.promptURL(ctx); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(app.Stderr, "URL saved."); err != nil {
					return err
				}
			}

			if urlOnly {
				return nil
			}
			if app.Config.HasToken() {
				if _, err := fmt.Fprintln(app.Stderr, "Token is already defined; it will be replaced"); err != nil {
					return err
				}
			}
			if err := prompter.promptToken(ctx); err != nil {
				return err
			}
			_, err := fmt.Fprintln(app.Stderr, "Token saved.")
			return err
		},
	}

	command.Flags().BoolVar(&replaceURL, "replace-url", false, "prompt for and replace the configured URL")
	command.Flags().BoolVar(&urlOnly, "url-only", false, "prompt for and replace only the URL")
	return command
}

type loginPrompter struct {
	app    *App
	reader *bufio.Reader
}

func (p loginPrompter) promptURL(ctx context.Context) error {
	interactive := p.app.IsTTY(p.app.Stdin)
	attempts := 1
	if interactive {
		attempts = maxURLPromptAttempts
	}

	var lastErr error
	for range attempts {
		value, err := p.readLine(ctx, "Base URL: ")
		if err != nil {
			return fmt.Errorf("read base URL: %w", err)
		}
		if err := p.app.Config.SetBaseURL(strings.TrimSpace(value)); err != nil {
			lastErr = err
			if !interactive {
				return err
			}
			if _, writeErr := fmt.Fprintf(p.app.Stderr, "Invalid base URL: %v\n", err); writeErr != nil {
				return writeErr
			}
			continue
		}
		return nil
	}

	return lastErr
}

func (p loginPrompter) promptToken(ctx context.Context) error {
	var value string
	if file, ok := p.app.Stdin.(*os.File); ok && p.app.IsTTY(file) {
		if _, err := fmt.Fprint(p.app.Stderr, "Token: "); err != nil {
			return err
		}
		password, err := p.readHiddenToken(ctx, int(file.Fd()))
		// Terminate the "Token: " line so a cancelled prompt (or the hidden
		// input) is not glued to the next shell prompt.
		if _, writeErr := fmt.Fprintln(p.app.Stderr); writeErr != nil {
			return writeErr
		}
		value = string(password)
		// Ctrl+D with no input surfaces as io.EOF; fall through to SetToken so
		// the user sees ErrEmptyToken instead of "read token: EOF".
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read token: %w", err)
		}
	} else {
		line, err := p.readLine(ctx, "Token: ")
		if err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		value = line
	}

	return p.app.Config.SetToken(strings.TrimSpace(value))
}

// readHiddenToken reads the token without echo, cancelling on ctx.Done(). Since
// term.ReadPassword's deferred restore never runs while its blocking read is
// stuck in the goroutine, capture the terminal state up front and restore it
// ourselves when the context is cancelled.
func (p loginPrompter) readHiddenToken(ctx context.Context, fd int) ([]byte, error) {
	state, stateErr := p.app.TerminalState(fd)
	password, err := readWithContext(ctx, func() ([]byte, error) {
		return p.app.ReadPassword(fd)
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if stateErr == nil {
			_ = p.app.RestoreTerminal(fd, state)
		}
	}
	return password, err
}

func (p loginPrompter) readLine(ctx context.Context, prompt string) (string, error) {
	if _, err := fmt.Fprint(p.app.Stderr, prompt); err != nil {
		return "", err
	}
	value, err := readWithContext(ctx, func() (string, error) {
		return p.reader.ReadString('\n')
	})
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return value, err
}

// readWithContext runs a blocking read in a goroutine and returns whichever
// happens first: the read result or context cancellation. On cancel it returns
// ctx.Err(); the goroutine is left to unblock on its own once the underlying
// read returns.
func readWithContext[T any](ctx context.Context, read func() (T, error)) (T, error) {
	type result struct {
		value T
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		value, err := read()
		ch <- result{value: value, err: err}
	}()

	select {
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	case r := <-ch:
		return r.value, r.err
	}
}
