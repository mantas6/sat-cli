package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mantas6/sat-cli/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(newAuthCommand)
}

func newAuthCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "auth",
		Short: "Show and configure authentication",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printAuthStatus(cmd, app)
		},
	}

	command.AddCommand(newLoginCommand(app))
	return command
}

// printAuthStatus writes a summary of the configured authentication to stdout.
// It never prints the token value itself.
func printAuthStatus(cmd *cobra.Command, app *App) error {
	out := cmd.OutOrStdout()

	baseURL := "not configured"
	if value, err := app.Config.BaseURL(); err == nil {
		baseURL = value
	} else if !errors.Is(err, config.ErrBaseURLMissing) {
		baseURL = fmt.Sprintf("invalid (%v)", err)
	}

	token := "not configured"
	if app.Config.HasToken() {
		token = "configured"
	}

	lines := []struct {
		label string
		value string
	}{
		{"State directory:", app.Config.Dir()},
		{"Base URL:", baseURL},
		{"URL file:", app.Config.URLPath() + envSuffix(app, "SAT_URL_PATH")},
		{"Token:", token},
		{"Token file:", app.Config.TokenPath() + envSuffix(app, "SAT_TOKEN_PATH")},
	}

	for _, line := range lines {
		if _, err := fmt.Fprintf(out, "%-16s %s\n", line.label, line.value); err != nil {
			return err
		}
	}

	return nil
}

// envSuffix returns a " (from NAME)" annotation when the named environment
// override is set, and an empty string otherwise.
func envSuffix(app *App, name string) string {
	if app.Getenv == nil {
		return ""
	}
	if strings.TrimSpace(app.Getenv(name)) == "" {
		return ""
	}

	return fmt.Sprintf("  (from %s)", name)
}
