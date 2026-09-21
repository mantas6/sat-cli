package command

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const maxURLPromptAttempts = 3

var readPassword = term.ReadPassword

func init() {
	registerCommand(newLoginCommand)
}

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
		RunE: func(_ *cobra.Command, _ []string) error {
			prompter := loginPrompter{
				app:    app,
				reader: bufio.NewReader(app.Stdin),
			}

			if !app.Config.HasBaseURL() || replaceURL || urlOnly {
				if err := prompter.promptURL(); err != nil {
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
			if err := prompter.promptToken(); err != nil {
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

func (p loginPrompter) promptURL() error {
	interactive := p.stdinIsTerminal()
	attempts := 1
	if interactive {
		attempts = maxURLPromptAttempts
	}

	var lastErr error
	for range attempts {
		value, err := p.readLine("Base URL: ")
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

func (p loginPrompter) promptToken() error {
	var value string
	if file, ok := p.app.Stdin.(*os.File); ok && p.isTerminal(int(file.Fd())) {
		if _, err := fmt.Fprint(p.app.Stderr, "Token: "); err != nil {
			return err
		}
		password, err := readPassword(int(file.Fd()))
		if _, writeErr := fmt.Fprintln(p.app.Stderr); writeErr != nil {
			return writeErr
		}
		if err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		value = string(password)
	} else {
		line, err := p.readLine("Token: ")
		if err != nil {
			return fmt.Errorf("read token: %w", err)
		}
		value = line
	}

	return p.app.Config.SetToken(strings.TrimSpace(value))
}

func (p loginPrompter) readLine(prompt string) (string, error) {
	if _, err := fmt.Fprint(p.app.Stderr, prompt); err != nil {
		return "", err
	}
	value, err := p.reader.ReadString('\n')
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return value, err
}

func (p loginPrompter) stdinIsTerminal() bool {
	file, ok := p.app.Stdin.(*os.File)
	return ok && p.isTerminal(int(file.Fd()))
}

func (p loginPrompter) isTerminal(fd int) bool {
	return p.app.IsTerminal != nil && p.app.IsTerminal(fd)
}
