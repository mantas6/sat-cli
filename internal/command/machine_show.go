package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

var errMachineNameRequired = errors.New("machine name is required when stdin and stdout are not terminals")

func newMachineShowCommand(app *App) *cobra.Command {
	var asJSON, raw bool
	command := &cobra.Command{
		Use:   "show [name]",
		Short: "Show the latest readings of a machine",
		Long: "Show the latest readings of one machine. The name must match exactly; without one, " +
			"a picker of all machines opens on a terminal.\n\n" +
			"With --json, the API response's data value is printed unrounded for scripts. With " +
			"--raw, only the stats modules the machine reported are printed as JSON.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = strings.TrimSpace(args[0])
				if name == "" {
					return errors.New("machine name must not be empty")
				}
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			if name == "" {
				if !app.interactive() {
					return errMachineNameRequired
				}
				name, err = pickMachine(cmd.Context(), app, client)
				if err != nil {
					return err
				}
			}

			machine, err := client.GetMachine(cmd.Context(), name)
			if err != nil {
				return machineError(err, name)
			}
			switch {
			case asJSON:
				return writeJSON(app.Stdout, machineJSON(machine))
			case raw:
				stats, err := machineStatsJSON(machine)
				if err != nil {
					return err
				}
				_, err = app.Stdout.Write(stats)
				return err
			default:
				return writeLine(app.Stdout, formatMachine(machine, app.machineView()))
			}
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print the API response as JSON")
	command.Flags().BoolVar(&raw, "raw", false, "print the stats modules as JSON")
	command.MarkFlagsMutuallyExclusive("json", "raw")
	return command
}

// pickMachine lets the user choose a machine from all known machines and
// returns its name.
func pickMachine(ctx context.Context, app *App, client APIClient) (string, error) {
	machines, err := client.ListMachines(ctx)
	if err != nil {
		return "", machineError(err, "")
	}
	now := app.Now()
	items := make([]ui.Item, 0, len(machines))
	for _, machine := range machines {
		items = append(items, machineItem(machine, now))
	}
	item, err := selectItem(ctx, app, items, ui.SelectOptions{Title: "Machines"})
	if err != nil {
		return "", err
	}
	return item.ID, nil
}

// machineStatsJSON returns the machine's stats modules as indented JSON. The
// array is taken from the object as the API sent it so number literals are
// kept; a missing or null array prints as [].
func machineStatsJSON(machine api.Machine) ([]byte, error) {
	if len(machine.Raw) == 0 {
		stats := machine.Stats
		if stats == nil {
			stats = []api.MachineStat{}
		}
		var out bytes.Buffer
		if err := writeJSON(&out, stats); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}

	var fields struct {
		Stats json.RawMessage `json:"stats"`
	}
	if err := json.Unmarshal(machine.Raw, &fields); err != nil {
		return nil, fmt.Errorf("decode machine stats: %w", err)
	}
	if len(fields.Stats) == 0 || string(fields.Stats) == "null" {
		fields.Stats = json.RawMessage("[]")
	}
	return indentJSON(fields.Stats)
}
