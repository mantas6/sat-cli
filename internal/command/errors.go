package command

import (
	"errors"
	"fmt"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
	"github.com/spf13/cobra"
)

// loginHint tells the user how to fix missing or rejected credentials.
const loginHint = "run `sat auth login`"

// ExitError asks the executable to return a specific process exit code.
type ExitError struct {
	Code int
	Err  error
}

// Error returns the underlying error text.
func (e ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit with code %d", e.Code)
	}
	return e.Err.Error()
}

// Unwrap exposes the underlying error.
func (e ExitError) Unwrap() error {
	return e.Err
}

// withLoginHint appends loginHint to errors that `sat auth login` fixes: a
// missing base URL or token, or a token the server rejected. The api and
// config packages stay free of CLI wording; errors.Is still matches the
// original sentinels.
func withLoginHint(err error) error {
	if errors.Is(err, config.ErrBaseURLMissing) ||
		errors.Is(err, config.ErrTokenMissing) ||
		errors.Is(err, api.ErrUnauthorized) {
		return fmt.Errorf("%w; %s", err, loginHint)
	}
	return err
}

// addLoginHints wraps the RunE of cmd and all of its descendants with
// withLoginHint.
func addLoginHints(cmd *cobra.Command) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			return withLoginHint(run(cmd, args))
		}
	}
	for _, child := range cmd.Commands() {
		addLoginHints(child)
	}
}
