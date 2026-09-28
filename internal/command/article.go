package command

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newArticleCommand(app *App) *cobra.Command {
	command := &cobra.Command{
		Use:   "article",
		Short: "Read and write journal articles",
		// NoArgs rejects unknown subcommands; cobra only validates args of
		// runnable commands, so RunE must stay to print help.
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
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

// parseArticleID converts an article ID from a flag, selector item or
// workspace file into the positive integer the API expects.
func parseArticleID(value string) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid article ID %q", value)
	}
	return id, nil
}

// newArlCommand is the hidden top-level `sat arl` shortcut for
// `sat article read`, matching the former arl script.
func newArlCommand(app *App) *cobra.Command {
	command := newArticleReadCommand(app)
	command.Use = "arl [query]"
	command.Aliases = nil
	command.Hidden = true
	return command
}

// articleItem is the picker item and cache record of one article: ID, title,
// word count, creation date and journal title.
func articleItem(article api.Article) ui.Item {
	journalTitle := ""
	if article.Journal != nil {
		journalTitle = article.Journal.Title
	}
	return ui.Item{
		ID: strconv.Itoa(article.ID),
		Columns: []string{
			cacheFieldReplacer.Replace(article.Title),
			fmt.Sprintf("%dw", article.WordCount),
			cacheFieldReplacer.Replace(article.CreatedAt),
			cacheFieldReplacer.Replace(journalTitle),
		},
	}
}

// newestFirst returns the picker items of articles, which the API lists
// oldest first, with the newest article first.
func newestFirst(articles []api.Article) []ui.Item {
	items := make([]ui.Item, len(articles))
	for index, article := range articles {
		items[index] = articleItem(article)
	}
	slices.Reverse(items)
	return items
}

func cachedArticleItems(ctx context.Context, app *App, client APIClient, all bool) ([]ui.Item, error) {
	lines, exists, err := app.Config.ReadCacheLines(articleCacheName)
	if err != nil {
		return nil, err
	}
	if exists {
		items := parseCacheLines(lines)
		slices.Reverse(items)
		return items, nil
	}

	if _, err := fmt.Fprintln(app.Stderr, "Fetching articles list..."); err != nil {
		return nil, err
	}
	articles, err := client.ListArticles(ctx, all)
	if err != nil {
		return nil, err
	}
	items := make([]ui.Item, len(articles))
	lines = make([]string, len(articles))
	for index, article := range articles {
		items[index] = articleItem(article)
		lines[index] = cacheLine(items[index])
	}
	if err := app.Config.WriteCacheLines(articleCacheName, lines); err != nil {
		return nil, err
	}
	slices.Reverse(items)
	return items, nil
}
