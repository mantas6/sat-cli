package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/spf13/cobra"
)

var errMachineFollowNeedsTerminal = errors.New("machine follow mode requires a terminal on stdin and stdout")

// machineCompletionTimeout bounds the request made while completing machine
// names, so a slow API does not stall the shell.
const machineCompletionTimeout = 2 * time.Second

func newMachineCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:     "machine",
		Aliases: []string{"machines"},
		Short:   "Inspect machines that report system stats",
		Long: "Inspect machines that report their system stats to Satellite.\n\n" +
			"A machine is online when it reported within the last two minutes. Memory and disk " +
			"usage of 85% or more is flagged as high: with \"!\" by list and \"(high)\" by show.\n\n" +
			"The API token needs permission to inspect machines; the API answers HTTP 403 otherwise.",
		// NoArgs rejects unknown subcommands; cobra only validates args of
		// runnable commands, so RunE must stay to print help.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	command.AddCommand(newMachineListCommand(app), newMachineShowCommand(app))
	return command
}

func newMachineListCommand(app *App) *cobra.Command {
	var asJSON bool
	var follow time.Duration
	command := &cobra.Command{
		Use:     "list [interval]",
		Aliases: []string{"ls"},
		Short:   "List machines with their latest readings",
		Long: "List every machine sorted by name with its online state and latest readings.\n\n" +
			"On a terminal, columns that do not fit its width are dropped. With --json, the API " +
			"response's data value is printed unrounded for scripts.\n\n" +
			"With --follow, the list is refreshed in a full-screen terminal. An optional positional " +
			"duration may be used, for example `sat machine list --follow 10s`.",
		Args: cobra.MaximumNArgs(1),
		// The only positional is a duration, so offer neither names nor files.
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			following := cmd.Flags().Changed("follow")
			if err := checkMachineFollow(app, following, &follow, args); err != nil {
				return err
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			if following {
				fetch := func(ctx context.Context) (string, error) {
					machines, err := client.ListMachines(ctx)
					if err != nil {
						return "", machineError(err, "")
					}
					if len(machines) == 0 {
						return "No machines.\n", nil
					}
					return formatMachineTable(machines, app.machineView()), nil
				}
				// Fetch once up front so a failing request ends the command
				// instead of showing the refresh failure badge forever.
				if _, err := fetch(cmd.Context()); err != nil {
					return err
				}
				return runFollow(cmd.Context(), app, follow, "machines", fetch)
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
	addFollowFlag(command, &follow)
	command.MarkFlagsMutuallyExclusive("json", "follow")
	return command
}

// checkMachineFollow validates follow mode before any request. args holds
// the optional positional interval, which overrides the --follow value and
// requires --follow.
func checkMachineFollow(app *App, following bool, interval *time.Duration, args []string) error {
	if len(args) > 0 {
		if !following {
			return errors.New("machine follow interval requires --follow")
		}
		parsed, err := time.ParseDuration(args[0])
		if err != nil {
			return fmt.Errorf("invalid machine follow interval %q: %w", args[0], err)
		}
		*interval = parsed
	}
	if !following {
		return nil
	}
	if *interval <= 0 {
		return errors.New("machine follow interval must be greater than zero")
	}
	// The follow view reads its quit keys from stdin.
	if !app.interactive() {
		return errMachineFollowNeedsTerminal
	}
	return nil
}

// completeMachineNames offers the names of all known machines, described as
// online or offline, for the first positional argument. The second positional
// is a duration and gets no completions. Any failure, such as missing
// credentials, an HTTP error or the timeout, yields no completions and prints
// nothing, since the output is read by the shell.
func completeMachineNames(app *App) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, machineCompletionTimeout)
		defer cancel()

		client, err := apiClient(app)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		machines, err := client.ListMachines(ctx)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		completions := make([]cobra.Completion, 0, len(machines))
		for _, machine := range machines {
			name := plainText(machine.Name)
			if name == "" {
				continue
			}
			status := "offline"
			if machine.Online {
				status = "online"
			}
			completions = append(completions, cobra.CompletionWithDesc(name, status))
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

// machineJSON returns the machine object as the API sent it, or the decoded
// machine when that is unavailable.
func machineJSON(machine api.Machine) any {
	if len(machine.Raw) == 0 {
		return machine
	}
	return machine.Raw
}
