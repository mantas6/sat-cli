package command

import (
	"context"
	"strings"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

func assignArticle(ctx context.Context, command *cobra.Command, app *App, client APIClient, id, journal string) error {
	if journal == "" {
		titles, exists, err := app.Config.ReadCacheLines(journalCacheName)
		if err != nil {
			return err
		}
		if !exists {
			journals, err := client.ListJournals(ctx)
			if err != nil {
				return err
			}
			titles = make([]string, len(journals))
			for index, item := range journals {
				titles[index] = item.Title
			}
			if err := app.Config.WriteCacheLines(journalCacheName, titles); err != nil {
				return err
			}
		}

		items := make([]ui.Item, 0, len(titles))
		for _, title := range titles {
			if title = strings.TrimSpace(title); title != "" {
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
