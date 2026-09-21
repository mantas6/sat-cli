package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
	"github.com/mantas6/sat-cli/internal/ui"
)

type articleLifecycleAPI struct {
	*stubAPI
	list     func(context.Context, bool) ([]api.Article, error)
	get      func(context.Context, string) (api.ArticleContents, error)
	create   func(context.Context, string) (api.Article, error)
	update   func(context.Context, string, string) (api.Article, error)
	assign   func(context.Context, string, string) (api.Article, error)
	journals func(context.Context) ([]api.Journal, error)
}

func (a *articleLifecycleAPI) ListArticles(ctx context.Context, all bool) ([]api.Article, error) {
	if a.list == nil {
		return nil, nil
	}
	return a.list(ctx, all)
}

func (a *articleLifecycleAPI) GetArticle(ctx context.Context, id string) (api.ArticleContents, error) {
	if a.get == nil {
		return api.ArticleContents{}, nil
	}
	return a.get(ctx, id)
}

func (a *articleLifecycleAPI) CreateArticle(ctx context.Context, contents string) (api.Article, error) {
	if a.create == nil {
		return api.Article{}, nil
	}
	return a.create(ctx, contents)
}

func (a *articleLifecycleAPI) UpdateArticleContents(ctx context.Context, id, contents string) (api.Article, error) {
	if a.update == nil {
		return api.Article{}, nil
	}
	return a.update(ctx, id, contents)
}

func (a *articleLifecycleAPI) AssignArticleJournal(ctx context.Context, id, journal string) (api.Article, error) {
	if a.assign == nil {
		return api.Article{}, nil
	}
	return a.assign(ctx, id, journal)
}

func (a *articleLifecycleAPI) ListJournals(ctx context.Context) ([]api.Journal, error) {
	if a.journals == nil {
		return nil, nil
	}
	return a.journals(ctx)
}

type recordingArticleRunner struct {
	name string
	args []string
	err  error
	run  func(name string, args []string) error
}

func (r *recordingArticleRunner) Run(_ context.Context, name string, args []string, _ io.Reader, _, _ io.Writer) error {
	r.name = name
	r.args = append([]string(nil), args...)
	if r.run != nil {
		return r.run(name, args)
	}
	return r.err
}

func newArticleLifecycleApp(t *testing.T, client APIClient) (*App, *config.Store, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	store := config.NewStore(t.TempDir(), func(string) string { return "" })
	if err := store.SetBaseURL("https://satellite.test"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetToken("secret"); err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	app := &App{
		Config: store,
		NewAPIClient: func(string, string) (APIClient, error) {
			return client, nil
		},
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: stderr,
		Now:    time.Now,
	}
	return app, store, stdout, stderr
}

func TestArticleAssignCacheHitAvoidsJournalRequestAndInvalidatesArticles(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	var selected []ui.Item
	runSelector = func(_ context.Context, _ io.Reader, _ io.Writer, items []ui.Item, options ui.SelectOptions) (ui.Item, error) {
		selected = append([]ui.Item(nil), items...)
		if options.Title != "Journals" {
			t.Fatalf("selector title = %q", options.Title)
		}
		return items[1], nil
	}

	var gotID, gotJournal string
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, journals: func(context.Context) ([]api.Journal, error) {
		t.Fatal("ListJournals() called on cache hit")
		return nil, nil
	}, assign: func(_ context.Context, id, journal string) (api.Article, error) {
		gotID, gotJournal = id, journal
		return api.Article{}, nil
	}}
	app, store, _, _ := newArticleLifecycleApp(t, client)
	if err := store.WriteCacheLines(journalCacheName, []string{"Default", "Work"}); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCacheLines(articleCacheName, []string{"1\tCached"}); err != nil {
		t.Fatal(err)
	}

	if err := executeArticleTestCommand(app, "article", "assign", "12"); err != nil {
		t.Fatal(err)
	}
	if gotID != "12" || gotJournal != "Work" {
		t.Fatalf("AssignArticleJournal() = (%q, %q), want (12, Work)", gotID, gotJournal)
	}
	wantItems := []ui.Item{{ID: "Default", Columns: []string{"Default"}}, {ID: "Work", Columns: []string{"Work"}}}
	if !reflect.DeepEqual(selected, wantItems) {
		t.Fatalf("selector items = %#v, want %#v", selected, wantItems)
	}
	if _, exists, err := store.ReadCacheLines(articleCacheName); err != nil || exists {
		t.Fatalf("article cache exists = %v, error = %v; want removed", exists, err)
	}
}

func TestArticleAssignCacheMissWritesReturnedOrder(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	runSelector = func(_ context.Context, _ io.Reader, _ io.Writer, items []ui.Item, _ ui.SelectOptions) (ui.Item, error) {
		return items[0], nil
	}

	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, journals: func(context.Context) ([]api.Journal, error) {
		return []api.Journal{{ID: 3, Title: "Default"}, {ID: 7, Title: "Notes"}}, nil
	}, assign: func(_ context.Context, _, journal string) (api.Article, error) {
		if journal != "Default" {
			t.Fatalf("journal = %q, want first/default journal", journal)
		}
		return api.Article{}, nil
	}}
	app, store, _, _ := newArticleLifecycleApp(t, client)

	if err := executeArticleTestCommand(app, "article", "assign", "1"); err != nil {
		t.Fatal(err)
	}
	lines, exists, err := store.ReadCacheLines(journalCacheName)
	if err != nil || !exists {
		t.Fatalf("journal cache exists = %v, error = %v", exists, err)
	}
	if want := []string{"Default", "Notes"}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("journal cache = %#v, want %#v", lines, want)
	}
}

func TestArticleAssignUsesSuppliedTitleDirectly(t *testing.T) {
	var gotJournal string
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, journals: func(context.Context) ([]api.Journal, error) {
		t.Fatal("ListJournals() called with supplied title")
		return nil, nil
	}, assign: func(_ context.Context, _, journal string) (api.Article, error) {
		gotJournal = journal
		return api.Article{}, nil
	}}
	app, _, _, _ := newArticleLifecycleApp(t, client)
	if err := executeArticleTestCommand(app, "article", "assign", "1", "Journal with spaces"); err != nil {
		t.Fatal(err)
	}
	if gotJournal != "Journal with spaces" {
		t.Fatalf("journal = %q", gotJournal)
	}
}

func TestArticleAssignFailureKeepsArticleCache(t *testing.T) {
	wantErr := errors.New("assignment failed")
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, assign: func(context.Context, string, string) (api.Article, error) {
		return api.Article{}, wantErr
	}}
	app, store, _, _ := newArticleLifecycleApp(t, client)
	if err := store.WriteCacheLines(articleCacheName, []string{"1\tCached"}); err != nil {
		t.Fatal(err)
	}

	err := executeArticleTestCommand(app, "article", "assign", "1", "Default")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
	if _, exists, err := store.ReadCacheLines(articleCacheName); err != nil || !exists {
		t.Fatalf("article cache exists = %v, error = %v; want retained", exists, err)
	}
}

func TestSaveArticleRequiresContents(t *testing.T) {
	app, _, _, _ := newArticleLifecycleApp(t, &articleLifecycleAPI{stubAPI: &stubAPI{}})
	_, err := saveArticle(context.Background(), app, &articleLifecycleAPI{stubAPI: &stubAPI{}}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "contents.md") {
		t.Fatalf("saveArticle() error = %v, want missing contents.md error", err)
	}
}

func TestSaveArticleCreateThenUpdateLifecycle(t *testing.T) {
	var createdContents, updatedID, updatedContents string
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, create: func(_ context.Context, contents string) (api.Article, error) {
		createdContents = contents
		return api.Article{ID: 42, WordCount: 3}, nil
	}, update: func(_ context.Context, id, contents string) (api.Article, error) {
		updatedID, updatedContents = id, contents
		return api.Article{ID: 42, WordCount: 5}, nil
	}}
	app, store, stdout, _ := newArticleLifecycleApp(t, client)
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "contents.md"), []byte("one two three"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCacheLines(articleCacheName, []string{"cached"}); err != nil {
		t.Fatal(err)
	}

	if _, err := saveArticle(context.Background(), app, client, workDir); err != nil {
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
	if _, exists, err := store.ReadCacheLines(articleCacheName); err != nil || exists {
		t.Fatalf("article cache exists = %v, error = %v; want removed", exists, err)
	}

	if err := os.WriteFile(filepath.Join(workDir, "contents.md"), []byte("updated article contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := saveArticle(context.Background(), app, client, workDir); err != nil {
		t.Fatal(err)
	}
	if updatedID != "42" || updatedContents != "updated article contents" {
		t.Fatalf("UpdateArticleContents() = (%q, %q)", updatedID, updatedContents)
	}
	if stdout.String() != "Word count: 3\nWord count: 5\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSaveArticleFailureKeepsIDAndCacheButCreatesBackup(t *testing.T) {
	wantErr := errors.New("update failed")
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, update: func(context.Context, string, string) (api.Article, error) {
		return api.Article{}, wantErr
	}}
	app, store, stdout, _ := newArticleLifecycleApp(t, client)
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "contents.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "id"), []byte("9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCacheLines(articleCacheName, []string{"cached"}); err != nil {
		t.Fatal(err)
	}

	_, err := saveArticle(context.Background(), app, client, workDir)
	if !errors.Is(err, wantErr) {
		t.Fatalf("saveArticle() error = %v, want %v", err, wantErr)
	}
	id, readErr := os.ReadFile(filepath.Join(workDir, "id"))
	if readErr != nil || string(id) != "9\n" {
		t.Fatalf("id file = %q, error = %v; want unchanged", id, readErr)
	}
	if _, exists, err := store.ReadCacheLines(articleCacheName); err != nil || !exists {
		t.Fatalf("article cache exists = %v, error = %v; want retained", exists, err)
	}
	backups, globErr := filepath.Glob(filepath.Join(workDir, "backup-*.md"))
	if globErr != nil || len(backups) != 1 {
		t.Fatalf("backups = %#v, error = %v; want one", backups, globErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}
