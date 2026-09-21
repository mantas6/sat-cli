package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mantas6/sat-cli/internal/ui"
	"github.com/spf13/cobra"
)

const articleWorkspaceMaxAge = 30 * 24 * time.Hour

var (
	executablePath = os.Executable
	editorBinary   = "nvim"
)

func newArticleEditCommand(app *App) *cobra.Command {
	var id string
	command := &cobra.Command{
		Use:   "edit [query]",
		Short: "Edit a journal article",
		Args:  cobra.ArbitraryArgs,
		RunE: func(command *cobra.Command, args []string) error {
			articleID := strings.TrimSpace(id)
			if command.Flags().Changed("id") && articleID == "" {
				return errors.New("article ID must not be empty")
			}

			client, err := apiClient(app)
			if err != nil {
				return err
			}
			if articleID == "" {
				articles, err := client.ListArticles(command.Context(), false)
				if err != nil {
					return err
				}
				lines := make([]string, len(articles))
				for index, article := range articles {
					lines[index] = articleLine(article)
				}
				items := append([]ui.Item{{ID: "0", Columns: []string{"New"}}}, reverseItems(parseArticleLines(lines))...)
				item, err := selectItem(command, app, items, ui.SelectOptions{
					Title: "Articles",
					Query: strings.Join(args, " "),
				})
				if err != nil {
					return err
				}
				articleID = item.ID
			}

			return editArticle(command.Context(), command, app, client, articleID)
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
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := apiClient(app)
			if err != nil {
				return err
			}
			return editArticle(command.Context(), command, app, client, "new")
		},
	}
}

func editArticle(ctx context.Context, command *cobra.Command, app *App, client APIClient, id string) error {
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
	isNew := id == "0" || strings.EqualFold(id, "new")
	if !isNew {
		article, err := client.GetArticle(ctx, id)
		if err != nil {
			return err
		}
		if err := os.WriteFile(contentsPath, []byte(article.Contents), 0o600); err != nil {
			return fmt.Errorf("write article contents: %w", err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "id"), []byte(id+"\n"), 0o600); err != nil {
			return fmt.Errorf("write article ID: %w", err)
		}
	}

	executable, err := executablePath()
	if err != nil {
		return fmt.Errorf("resolve sat executable: %w", err)
	}
	args := []string{"-c", editorCommand(executable, workDir), contentsPath}
	err = app.Runner.Run(ctx, editorBinary, args, app.Stdin, app.Stdout, app.Stderr)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return ExitError{Code: exitErr.ExitCode(), Err: err}
	}
	if err != nil {
		return err
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
	return assignArticle(ctx, command, app, client, strings.TrimSpace(string(idBytes)), "")
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
