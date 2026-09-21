package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/spf13/cobra"
)

func newArticleSaveCommand(app *App) *cobra.Command {
	var workDir string
	var hook bool
	command := &cobra.Command{
		Use:    "save --work-dir DIR",
		Short:  "Save an article editor workspace",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(workDir) == "" {
				return errors.New("work directory must not be empty")
			}
			client, err := apiClient(app)
			if err == nil {
				_, err = saveArticle(command.Context(), app, client, workDir)
			}
			if err != nil && hook {
				// Vim's list-form system() is shell-free but only captures stdout.
				_, _ = fmt.Fprintln(app.Stdout, err)
				return ExitError{Code: 1}
			}
			return err
		},
	}
	command.Flags().StringVar(&workDir, "work-dir", "", "article workspace directory")
	command.Flags().BoolVar(&hook, "hook", false, "format output for the editor save hook")
	_ = command.Flags().MarkHidden("hook")
	return command
}

func saveArticle(ctx context.Context, app *App, client APIClient, workDir string) (api.Article, error) {
	contents, err := os.ReadFile(filepath.Join(workDir, "contents.md"))
	if err != nil {
		return api.Article{}, fmt.Errorf("read article contents: %w", err)
	}

	backup, err := os.CreateTemp(workDir, "backup-*.md")
	if err != nil {
		return api.Article{}, fmt.Errorf("create article backup: %w", err)
	}
	backupName := backup.Name()
	if err := backup.Chmod(0o600); err != nil {
		_ = backup.Close()
		return api.Article{}, fmt.Errorf("secure article backup %q: %w", backupName, err)
	}
	if _, err := backup.Write(contents); err != nil {
		_ = backup.Close()
		return api.Article{}, fmt.Errorf("write article backup %q: %w", backupName, err)
	}
	if err := backup.Close(); err != nil {
		return api.Article{}, fmt.Errorf("close article backup %q: %w", backupName, err)
	}

	idPath := filepath.Join(workDir, "id")
	idBytes, err := os.ReadFile(idPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return api.Article{}, fmt.Errorf("read article ID: %w", err)
	}
	id := strings.TrimSpace(string(idBytes))

	var article api.Article
	if id == "" {
		article, err = client.CreateArticle(ctx, string(contents))
	} else {
		article, err = client.UpdateArticleContents(ctx, id, string(contents))
	}
	if err != nil {
		return api.Article{}, err
	}

	if err := writeArticleID(idPath, article.ID); err != nil {
		return api.Article{}, err
	}
	if err := app.Config.RemoveCache(articleCacheName); err != nil {
		return api.Article{}, err
	}
	if _, err := fmt.Fprintf(app.Stdout, "Word count: %d\n", article.WordCount); err != nil {
		return api.Article{}, err
	}
	return article, nil
}

func writeArticleID(path string, id int) (err error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".id.tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary article ID file: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary article ID file: %w", err)
	}
	if _, err := temporary.WriteString(strconv.Itoa(id) + "\n"); err != nil {
		return fmt.Errorf("write temporary article ID file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary article ID file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary article ID file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace article ID file: %w", err)
	}
	return nil
}
