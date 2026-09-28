package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

const (
	articleWorkspaceMaxAge = 30 * 24 * time.Hour
	// newArticleItemID identifies the "New" entry in the article edit picker.
	newArticleItemID = "new"
)

const editorBinary = "nvim"

func newArticleEditCommand(app *App) *cobra.Command {
	var id string
	command := &cobra.Command{
		Use:   "edit [query]",
		Short: "Edit a journal article",
		Args:  cobra.ArbitraryArgs,
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
				articles, err := client.ListArticles(cmd.Context(), false)
				if err != nil {
					return err
				}
				items := append([]ui.Item{{ID: newArticleItemID, Columns: []string{"New"}}}, newestFirst(articles)...)
				item, err := selectItem(cmd.Context(), app, items, ui.SelectOptions{
					Title: "Articles",
					Query: strings.Join(args, " "),
				})
				if err != nil {
					return err
				}
				if item.ID == newArticleItemID {
					return editArticle(cmd.Context(), app, client, 0, true)
				}
				rawID = item.ID
			}
			articleID, err := parseArticleID(rawID)
			if err != nil {
				return err
			}

			return editArticle(cmd.Context(), app, client, articleID, false)
		},
	}
	command.Flags().StringVar(&id, "id", "", "article ID")
	return command
}

func newArticleNewCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "new",
		Short: "Create a journal article",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := apiClient(app)
			if err != nil {
				return err
			}
			return editArticle(cmd.Context(), app, client, 0, true)
		},
	}
}

// editArticle opens the article with id in the editor, or an empty workspace
// when isNew is true (id is then ignored).
func editArticle(ctx context.Context, app *App, client APIClient, id int, isNew bool) error {
	tmpDir, err := app.Config.TmpDir()
	if err != nil {
		return err
	}
	defer func() { _ = cleanupWorkspaces(tmpDir, app.Now(), articleWorkspaceMaxAge) }()

	workDir, err := os.MkdirTemp(tmpDir, "")
	if err != nil {
		return fmt.Errorf("create article workspace: %w", err)
	}
	contentsPath := filepath.Join(workDir, "contents.md")
	if !isNew {
		article, err := client.GetArticle(ctx, id)
		if err != nil {
			return err
		}
		if err := os.WriteFile(contentsPath, []byte(article.Contents), 0o600); err != nil {
			return fmt.Errorf("write article contents: %w", err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "id"), []byte(strconv.Itoa(id)+"\n"), 0o600); err != nil {
			return fmt.Errorf("write article ID: %w", err)
		}
	}

	executable, err := app.Executable()
	if err != nil {
		return fmt.Errorf("resolve sat executable: %w", err)
	}
	args := []string{"-c", editorCommand(executable, workDir), contentsPath}
	if err := app.Runner.Run(ctx, editorBinary, args, app.Stdin, app.Stdout, app.Stderr); err != nil {
		return childExitError(err)
	}

	if !isNew {
		return nil
	}
	contentsInfo, contentsErr := os.Stat(contentsPath)
	idBytes, idErr := os.ReadFile(filepath.Join(workDir, "id"))
	if contentsErr != nil && !errors.Is(contentsErr, os.ErrNotExist) {
		return fmt.Errorf("inspect article contents: %w", contentsErr)
	}
	if idErr != nil && !errors.Is(idErr, os.ErrNotExist) {
		return fmt.Errorf("read article ID: %w", idErr)
	}
	if errors.Is(contentsErr, os.ErrNotExist) || contentsInfo.IsDir() || errors.Is(idErr, os.ErrNotExist) || strings.TrimSpace(string(idBytes)) == "" {
		_, err := fmt.Fprintln(app.Stderr, "Nothing saved.")
		return err
	}
	savedID, err := parseArticleID(string(idBytes))
	if err != nil {
		return err
	}
	return assignArticle(ctx, app, client, savedID, "")
}

func assignArticle(ctx context.Context, app *App, client APIClient, id int, journal string) error {
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
		item, err := selectItem(ctx, app, items, ui.SelectOptions{Title: "Journals"})
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

func vimString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func vimList(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = vimString(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func editorCommand(executable, workDir string) string {
	command := vimList([]string{executable, "article", "save", "--work-dir", workDir, "--hook"})
	return "set nospell | autocmd BufWritePost <buffer> let g:sat_out = trim(system(" + command + ")) | if v:shell_error | echohl ErrorMsg | echomsg " + vimString("sat: save failed: ") + " . g:sat_out | echohl None | else | echo g:sat_out | endif"
}

func cleanupWorkspaces(tmpDir string, now time.Time, maxAge time.Duration) error {
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("read temporary state directory: %w", err)
	}
	cutoff := now.Add(-maxAge)
	var firstErr error
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(tmpDir, entry.Name())); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
