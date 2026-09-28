// Package command defines sat's Cobra command tree and injectable boundaries.
package command

import "github.com/spf13/cobra"

// NewRootCommand builds the sat command tree. Register future commands with
// root.AddCommand(newXCommand(app)); each constructor remains easy to test.
// app must not be nil; use NewDefaultApp for the operating-system wiring.
func NewRootCommand(app *App) *cobra.Command {
	normalizeApp(app)

	root := &cobra.Command{
		Use:   "sat",
		Short: "Command-line client for Satellite",
		Long: `Command-line client for Satellite.

Environment:
  SAT_JOURNAL_STATE  State directory (default: $XDG_STATE_HOME/sat or ~/.local/state/sat)
  XDG_STATE_HOME     Base for the default state directory (used as $XDG_STATE_HOME/sat)
  SAT_URL_PATH       Override path of the base URL file (default: <state>/url)
  SAT_TOKEN_PATH     Override path of the token file (default: <state>/token)
  REMOTE_HOST        SSH host for sat run (default: derived from the base URL)
  REMOTE_USER        SSH user for sat run (default: none)
  REMOTE_ROOT        Remote release directory for sat run (default: $HOME/Sat/current)`,
		Version:       app.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetVersionTemplate("sat {{.Version}}\n")

	for _, constructor := range subcommands {
		root.AddCommand(constructor(app))
	}

	return root
}

// subcommands lists every top-level command constructor. Each command file
// registers itself from an init function via registerCommand so that files
// can be added without editing this one.
var subcommands []func(*App) *cobra.Command

func registerCommand(constructor func(*App) *cobra.Command) {
	subcommands = append(subcommands, constructor)
}
