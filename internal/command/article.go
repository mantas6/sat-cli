package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

const articleCacheName = "list"

func init() {
	registerCommand(newArticleCommand)
}

func newArticleCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:     "article",
		Aliases: []string{"arl"},
		Short:   "Read and write journal articles",
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(
		newArticleReadCommand(app),
		newArticleEditCommand(app),
		newArticleNewCommand(app),
		newArticleSaveCommand(app),
	)
	return command
}

func articleLine(article api.Article) string {
	journalTitle := ""
	if article.Journal != nil {
		journalTitle = article.Journal.Title
	}
	return fmt.Sprintf("%d\t%s\t%dw\t%s\t%s", article.ID, article.Title, article.WordCount, article.CreatedAt, journalTitle)
}

func parseArticleLines(lines []string) []ui.Item {
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
		items = append(items, ui.Item{ID: fields[0], Columns: columns})
	}
	return items
}

func reverseItems(items []ui.Item) []ui.Item {
	reversed := make([]ui.Item, len(items))
	for index := range items {
		reversed[len(items)-1-index] = items[index]
	}
	return reversed
}

func cachedArticleItems(ctx context.Context, app *App, client APIClient, all bool) ([]ui.Item, error) {
	lines, exists, err := app.Config.ReadCacheLines(articleCacheName)
	if err != nil {
		return nil, err
	}
	if exists {
		return reverseItems(parseArticleLines(lines)), nil
	}

	if _, err := fmt.Fprintln(app.Stderr, "Fetching articles list..."); err != nil {
		return nil, err
	}
	articles, err := client.ListArticles(ctx, all)
	if err != nil {
		return nil, err
	}
	lines = make([]string, len(articles))
	for index, article := range articles {
		lines[index] = articleLine(article)
	}
	if err := app.Config.WriteCacheLines(articleCacheName, lines); err != nil {
		return nil, err
	}
	return reverseItems(parseArticleLines(lines)), nil
}
