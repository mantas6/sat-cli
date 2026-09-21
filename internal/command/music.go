package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mantas6/sat-cli/internal/config"
	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(newMusicCommand)
}

func newMusicCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "music",
		Short: "Play and control saved music",
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newMusicSyncCommand(app),
		newMusicPlayCommand(app),
		newMusicControlCommand(app, "pause", "pause"),
		newMusicControlCommand(app, "resume", "play"),
		newMusicControlCommand(app, "next", "next"),
		newMusicControlCommand(app, "previous", "previous"),
	)
	return command
}

func newMusicSyncCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync saved tracks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := apiClient(app)
			if err != nil {
				return err
			}
			lines, err := client.SavedTracks(cmd.Context())
			if err != nil {
				return err
			}
			if err := app.Config.WriteCacheLines("tracks", lines); err != nil {
				return err
			}
			_, err = fmt.Fprintf(app.Stderr, "Synced %d tracks.\n", len(lines))
			return err
		},
	}
}

func newMusicPlayCommand(app *App) *cobra.Command {
	var id string
	command := &cobra.Command{
		Use:   "play [query]",
		Short: "Select and play a saved track",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			trackID := strings.TrimSpace(id)
			if cmd.Flags().Changed("id") && trackID == "" {
				return errors.New("track ID must not be empty")
			}

			if trackID == "" {
				lines, exists, err := app.Config.ReadCacheLines("tracks")
				if err != nil {
					return err
				}
				if !exists {
					return errors.New("track cache is missing; run `sat music sync`")
				}
				item, err := selectItem(cmd, app, parseTrackLines(lines), ui.SelectOptions{
					Title: "Saved tracks",
					Query: strings.Join(args, " "),
				})
				if err != nil {
					return err
				}
				trackID = item.ID
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			return client.PlayTrack(cmd.Context(), trackID)
		},
	}
	command.Flags().StringVar(&id, "id", "", "track or album ID")
	return command
}

func newMusicControlCommand(app *App, name, action string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: strings.ToUpper(name[:1]) + name[1:] + " playback",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := apiClient(app)
			if err != nil {
				return err
			}
			return client.ControlPlayback(cmd.Context(), action)
		},
	}
}

func parseTrackLines(lines []string) []ui.Item {
	items := make([]ui.Item, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := config.SplitTabs(line)
		if fields[0] == "" {
			continue
		}
		columns := make([]string, len(fields)-1)
		copy(columns, fields[1:])
		items = append(items, ui.Item{
			ID:      fields[0],
			Columns: columns,
		})
	}
	return items
}
