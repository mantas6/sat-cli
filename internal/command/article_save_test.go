package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
)

// newWorkspace returns a workspace directory holding contents.md and, when id
// is not empty, an id file.
func newWorkspace(t *testing.T, contents, id string) string {
	t.Helper()
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "contents.md"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if id != "" {
		if err := os.WriteFile(filepath.Join(workDir, "id"), []byte(id), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return workDir
}

func TestSaveArticleRequiresContents(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)
	_, err := saveArticle(t.Context(), app, &fakeAPI{}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "contents.md") {
		t.Fatalf("saveArticle() error = %v, want missing contents.md error", err)
	}
}

func TestSaveArticleCreateThenUpdateLifecycle(t *testing.T) {
	t.Parallel()
	var createdContents, updatedContents string
	var updatedID int
	client := &fakeAPI{createArticle: func(_ context.Context, contents string) (api.Article, error) {
		createdContents = contents
		return api.Article{ID: 42, WordCount: 3}, nil
	}, updateArticleContents: func(_ context.Context, id int, contents string) (api.Article, error) {
		updatedID, updatedContents = id, contents
		return api.Article{ID: 42, WordCount: 5}, nil
	}}
	cfg := newFakeConfig(t)
	cfg.caches = map[string][]string{articleCacheName: {"cached"}}
	app, stdout, _ := newTestApp(t, withConfig(cfg))
	workDir := newWorkspace(t, "one two three", "")

	if _, err := saveArticle(t.Context(), app, client, workDir); err != nil {
		t.Fatal(err)
	}
	if createdContents != "one two three" {
		t.Fatalf("CreateArticle() contents = %q", createdContents)
	}
	id, err := os.ReadFile(filepath.Join(workDir, "id"))
	if err != nil {
		t.Fatal(err)
	}
	if string(id) != "42\n" {
		t.Fatalf("id file = %q, want 42 newline", id)
	}
	info, err := os.Stat(filepath.Join(workDir, "id"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("id mode = %v, want 0600", info.Mode().Perm())
	}
	if backups, err := filepath.Glob(filepath.Join(workDir, "backup-*.md")); err != nil || len(backups) != 1 {
		t.Fatalf("backups = %#v, error = %v; want one", backups, err)
	}
	if _, exists := cfg.caches[articleCacheName]; exists {
		t.Fatal("article cache kept after save, want removed")
	}

	if err := os.WriteFile(filepath.Join(workDir, "contents.md"), []byte("updated article contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveArticle(t.Context(), app, client, workDir); err != nil {
		t.Fatal(err)
	}
	if updatedID != 42 || updatedContents != "updated article contents" {
		t.Fatalf("UpdateArticleContents() = (%d, %q)", updatedID, updatedContents)
	}
	if stdout.String() != "Word count: 3\nWord count: 5\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSaveArticleRejectsInvalidWorkspaceID(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{updateArticleContents: func(context.Context, int, string) (api.Article, error) {
		t.Error("UpdateArticleContents() called with an invalid ID")
		return api.Article{}, nil
	}}
	app, _, _ := newTestApp(t)
	workDir := newWorkspace(t, "text", "..\n")

	_, err := saveArticle(t.Context(), app, client, workDir)
	if err == nil || !strings.Contains(err.Error(), "invalid article ID") {
		t.Fatalf("saveArticle() error = %v, want invalid article ID", err)
	}
}

func TestSaveArticleFailureKeepsIDAndCacheButCreatesBackup(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("update failed")
	client := &fakeAPI{updateArticleContents: func(context.Context, int, string) (api.Article, error) {
		return api.Article{}, wantErr
	}}
	cfg := newFakeConfig(t)
	cfg.caches = map[string][]string{articleCacheName: {"cached"}}
	app, stdout, _ := newTestApp(t, withConfig(cfg))
	workDir := newWorkspace(t, "changed", "9\n")

	_, err := saveArticle(t.Context(), app, client, workDir)
	if !errors.Is(err, wantErr) {
		t.Fatalf("saveArticle() error = %v, want %v", err, wantErr)
	}
	id, readErr := os.ReadFile(filepath.Join(workDir, "id"))
	if readErr != nil || string(id) != "9\n" {
		t.Fatalf("id file = %q, error = %v; want unchanged", id, readErr)
	}
	if _, exists := cfg.caches[articleCacheName]; !exists {
		t.Fatal("article cache removed after a failed save, want retained")
	}
	backups, globErr := filepath.Glob(filepath.Join(workDir, "backup-*.md"))
	if globErr != nil || len(backups) != 1 {
		t.Fatalf("backups = %#v, error = %v; want one", backups, globErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestArticleSaveHookReportsErrorsOnStdout(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("update failed")
	client := &fakeAPI{updateArticleContents: func(context.Context, int, string) (api.Article, error) {
		return api.Article{}, wantErr
	}}
	app, stdout, stderr := newTestApp(t, withAPI(client))
	workDir := newWorkspace(t, "changed", "9\n")

	err := run(t, app, "article", "save", "--work-dir", workDir, "--hook")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 || exitErr.Err != nil {
		t.Fatalf("Execute() error = %#v, want silent ExitError code 1", err)
	}
	if stdout.String() != "update failed\n" || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q; want the error on stdout only", stdout.String(), stderr.String())
	}
}

func TestArticleSaveRequiresWorkDir(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)
	if err := run(t, app, "article", "save"); err == nil || err.Error() != "work directory must not be empty" {
		t.Fatalf("Execute() error = %v, want work directory error", err)
	}
}
