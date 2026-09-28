package command

import "github.com/spf13/cobra"

func newWeatherCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "weather [place]",
		Aliases: []string{"wt"},
		Short:   "Show the weather forecast (server default place when omitted)",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var place string
			if len(args) == 1 {
				place = args[0]
			}

			client, err := publicClient(app)
			if err != nil {
				return err
			}
			forecast, err := client.Weather(cmd.Context(), place)
			if err != nil {
				return err
			}

			return writeLine(app.Stdout, forecast)
		},
	}
}
