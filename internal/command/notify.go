package command

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	registerCommand(newNotifyCommand)
}

func newNotifyCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "notify MESSAGE",
		Short: "Send a notification",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return errors.New("message must not be empty")
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			return client.Notify(cmd.Context(), args[0])
		},
	}
	return command
}
