package command

import (
	"context"
	"strings"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

func assignArticle(ctx context.Context, command *cobra.Command, app *App, client APIClient, id, journal string) error {
	if journal == "" {
		journals, err := client.ListJournals(ctx)
		if err != nil {
			return err
		}

		items := make([]ui.Item, 0, len(journals))
		for _, item := range journals {
			if title := strings.TrimSpace(item.Title); title != "" {
				items = append(items, ui.Item{ID: title, Columns: []string{title}})
			}
		}
		item, err := selectItem(command, app, items, ui.SelectOptions{Title: "Journals"})
		if err != nil {
			return err
		}
		journal = item.ID
	}

	if _, err := client.AssignArticleJournal(ctx, id, journal); err != nil {
		return err
	}
	return app.Config.RemoveCache(articleCacheName)
}
