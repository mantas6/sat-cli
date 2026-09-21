package command

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

var runPager = ui.Page

func newArticleReadCommand(app *App) *cobra.Command {
	var id string
	var raw bool
	command := &cobra.Command{
		Use:   "read [query]",
		Short: "Read a journal article",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			articleID := strings.TrimSpace(id)
			if cmd.Flags().Changed("id") && articleID == "" {
				return errors.New("article ID must not be empty")
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			if articleID == "" {
				items, err := cachedArticleItems(cmd.Context(), app, client, true)
				if err != nil {
					return err
				}
				item, err := selectItem(cmd, app, items, ui.SelectOptions{
					Title: "Articles",
					Query: strings.Join(args, " "),
				})
				if err != nil {
					return err
				}
				articleID = item.ID
			}

			article, err := client.GetArticle(cmd.Context(), articleID)
			if err != nil {
				return err
			}
			output, terminal := app.Stdout.(*os.File)
			if raw || !terminal || !app.IsTerminal(int(output.Fd())) {
				return writeArticleContents(app.Stdout, article.Contents)
			}

			width, height, err := app.TerminalSize(int(output.Fd()))
			if err != nil || width <= 0 {
				width = 80
			}
			if err != nil || height <= 0 {
				height = 24
			}
			return runPager(cmd.Context(), app.Stdin, app.Stdout, article.Contents, ui.PageOptions{
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

func writeArticleContents(writer io.Writer, contents string) error {
	if _, err := io.WriteString(writer, contents); err != nil {
		return err
	}
	if !strings.HasSuffix(contents, "\n") {
		_, err := io.WriteString(writer, "\n")
		return err
	}
	return nil
}
