package command

import (
	"fmt"
	"io"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/spf13/cobra"
)

func newMachineCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:     "machine",
		Aliases: []string{"machines"},
		Short:   "Inspect machines that report system stats",
		Long: "Inspect machines that report their system stats to Satellite.\n\n" +
			"A machine is online when it reported within the last two minutes. Memory and disk " +
			"usage of 85% or more is flagged as high with \"!\".\n\n" +
			"The API token needs permission to inspect machines; the API answers HTTP 403 otherwise.",
		// NoArgs rejects unknown subcommands; cobra only validates args of
		// runnable commands, so RunE must stay to print help.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	command.AddCommand(newMachineListCommand(app))
	return command
}

func newMachineListCommand(app *App) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List machines with their latest readings",
		Long: "List every machine sorted by name with its online state and latest readings.\n\n" +
			"On a terminal, columns that do not fit its width are dropped. With --json, the API " +
			"response's data value is printed unrounded for scripts.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := apiClient(app)
			if err != nil {
				return err
			}
			machines, err := client.ListMachines(cmd.Context())
			if err != nil {
				return machineError(err, "")
			}

			if asJSON {
				values := make([]any, 0, len(machines))
				for _, machine := range machines {
					values = append(values, machineJSON(machine))
				}
				return writeJSON(app.Stdout, values)
			}
			if len(machines) == 0 {
				_, err := fmt.Fprintln(app.Stderr, "No machines.")
				return err
			}
			_, err = io.WriteString(app.Stdout, formatMachineTable(machines, app.machineView()))
			return err
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print the API response as JSON")
	return command
}

// machineJSON returns the machine object as the API sent it, or the decoded
// machine when that is unavailable.
func machineJSON(machine api.Machine) any {
	if len(machine.Raw) == 0 {
		return machine
	}
	return machine.Raw
}
