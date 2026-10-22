package command

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

var errMachineNameRequired = errors.New("machine name is required when stdin and stdout are not terminals")

func newMachineShowCommand(app *App) *cobra.Command {
	var asJSON, raw bool
	var follow time.Duration
	command := &cobra.Command{
		Use:   "show [name] [interval]",
		Short: "Show the latest readings of a machine",
		Long: "Show the latest readings of one machine. The name must match exactly; without one, " +
			"a picker of all machines opens on a terminal.\n\n" +
			"With --json, the API response's data value is printed unrounded for scripts. With " +
			"--raw, only the stats modules the machine reported are printed as JSON.\n\n" +
			"With --follow, the machine is refreshed in a full-screen terminal. An optional duration " +
			"may follow the name, for example `sat machine show web-1 --follow 10s`; without a name, " +
			"use `sat machine show --follow=10s`, because a single argument is always the name.",
		Args: cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = strings.TrimSpace(args[0])
				if name == "" {
					return errors.New("machine name must not be empty")
				}
			}
			following := cmd.Flags().Changed("follow")
			if err := checkMachineFollow(app, following, &follow, args[min(len(args), 1):]); err != nil {
				return err
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

			if following {
				return followMachine(cmd.Context(), app, client, name, follow)
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
	addFollowFlag(command, &follow)
	command.MarkFlagsMutuallyExclusive("json", "raw", "follow")
	return command
}

// followMachine shows the named machine full-screen, refreshing every
// interval. The machine is fetched once up front so an unknown name or a
// missing permission ends the command instead of showing the refresh failure
// badge forever.
func followMachine(ctx context.Context, app *App, client APIClient, name string, interval time.Duration) error {
	fetch := func(ctx context.Context) (string, error) {
		machine, err := client.GetMachine(ctx, name)
		if err != nil {
			return "", machineError(err, name)
		}
		return formatMachine(machine, app.machineView()), nil
	}
	if _, err := fetch(ctx); err != nil {
		// `show --follow 10s` passes 10s as the name, since a single argument
		// is always the name.
		if _, parseErr := time.ParseDuration(name); parseErr == nil && errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("%w; use --follow=10s to pick a machine with a custom interval", err)
		}
		return err
	}
	return runFollow(ctx, app, interval, "machine "+plainText(name), fetch)
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
