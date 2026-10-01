package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAuthGetURLCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "get-url",
		Short: "Print the configured base URL",
		Long: `Print the configured Satellite base URL followed by a newline, without any
label, for use in scripts:

  curl "$(sat auth get-url)/api/..."

Nothing is printed to stdout when the URL is missing or invalid.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseURL, err := app.Config.BaseURL()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), baseURL)
			return err
		},
	}
}
