package command

import (
	"errors"
	"strings"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

func newArticleReadCommand(app *App) *cobra.Command {
	var id string
	var raw bool
	command := &cobra.Command{
		Use:     "read [query]",
		Aliases: []string{"arl"},
		Short:   "Read a journal article",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rawID := strings.TrimSpace(id)
			if cmd.Flags().Changed("id") && rawID == "" {
				return errors.New("article ID must not be empty")
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			if rawID == "" {
				items, err := cachedArticleItems(cmd.Context(), app, client, true)
				if err != nil {
					return err
				}
				item, err := selectItem(cmd.Context(), app, items, ui.SelectOptions{
					Title: "Articles",
					Query: strings.Join(args, " "),
				})
				if err != nil {
					return err
				}
				rawID = item.ID
			}
			articleID, err := parseArticleID(rawID)
			if err != nil {
				return err
			}

			article, err := client.GetArticle(cmd.Context(), articleID)
			if err != nil {
				return err
			}
			if raw || !app.IsTTY(app.Stdout) {
				return writeLine(app.Stdout, article.Contents)
			}

			// An unknown size stays zero and the pager falls back to 80x24.
			width, height, _ := app.TermSize()
			return app.Page(cmd.Context(), app.Stdin, app.Stdout, article.Contents, ui.PageOptions{
				Title:  "Article",
				Width:  width,
				Height: height,
			})
		},
	}
	command.Flags().StringVar(&id, "id", "", "article ID")
	command.Flags().BoolVar(&raw, "raw", false, "print raw Markdown")
	return command
}
