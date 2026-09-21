package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	registerCommand(newConfigCommand)
}

func newConfigCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "config",
		Short: "Inspect configuration",
		Args:  cobra.ExactArgs(0),
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}

	command.AddCommand(
		&cobra.Command{
			Use:   "path",
			Short: "Print the state directory",
			Args:  cobra.ExactArgs(0),
			RunE: func(command *cobra.Command, _ []string) error {
				_, err := fmt.Fprintln(command.OutOrStdout(), app.Config.Dir())
				return err
			},
		},
		&cobra.Command{
			Use:   "url",
			Short: "Print the effective base URL",
			Args:  cobra.ExactArgs(0),
			RunE: func(command *cobra.Command, _ []string) error {
				baseURL, err := app.Config.BaseURL()
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(command.OutOrStdout(), baseURL)
				return err
			},
		},
		&cobra.Command{
			Use:   "token",
			Short: "Print the authentication token",
			Args:  cobra.ExactArgs(0),
			RunE: func(command *cobra.Command, _ []string) error {
				token, err := app.Config.Token()
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(command.OutOrStdout(), token)
				return err
			},
		},
	)

	return command
}
