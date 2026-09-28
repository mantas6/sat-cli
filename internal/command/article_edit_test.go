package command

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

// saveFromEditor returns a fakeRunner run func that behaves like an editor
// session whose save hook stored contents and the article ID in the workspace.
func saveFromEditor(contents, id string) func(string, []string) error {
	return func(_ string, args []string) error {
		workDir := filepath.Dir(args[len(args)-1])
		if err := os.WriteFile(filepath.Join(workDir, "contents.md"), []byte(contents), 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(workDir, "id"), []byte(id), 0o600)
	}
}

func TestArticleEditOffersNewFirstAndFetchesRecentArticles(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{listArticles: func(_ context.Context, all bool) ([]api.Article, error) {
		if all {
			t.Error("ListArticles() all = true, want recent list")
		}
		return []api.Article{{ID: 1, Title: "Shared article older"}, {ID: 2, Title: "Shared article recent"}}, nil
	}}
	app, _, stderr := newTestApp(t, withAPI(client), withRunner(&fakeRunner{}), withTTY())
	var gotItems []ui.Item
	var gotOpts ui.SelectOptions
	app.Select = selectIndex(0, &gotItems, &gotOpts)

	if err := run(t, app, "article", "edit", "Shared", "article"); err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{newArticleItemID, "2", "1"}
	gotIDs := make([]string, len(gotItems))
	for index, item := range gotItems {
		gotIDs[index] = item.ID
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) || gotItems[0].Columns[0] != "New" {
		t.Fatalf("selector items = %#v, want New then newest articles first", gotItems)
	}
	if !reflect.DeepEqual(gotItems[1], articleItem(api.Article{ID: 2, Title: "Shared article recent"})) {
		t.Fatalf("selector item = %#v, want articleItem columns", gotItems[1])
	}
	if gotOpts.Query != "Shared article" || gotOpts.Title != "Articles" {
		t.Fatalf("selector options = %#v", gotOpts)
	}
	if stderr.String() != "Nothing saved.\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestArticleEditSelectedArticleOpensIt(t *testing.T) {
	t.Parallel()
	var gotID int
	client := &fakeAPI{
		listArticles: func(context.Context, bool) ([]api.Article, error) {
			return []api.Article{{ID: 4, Title: "Only"}}, nil
		},
		getArticle: func(_ context.Context, id int) (api.ArticleContents, error) {
			gotID = id
			return api.ArticleContents{Contents: "text"}, nil
		},
	}
	runner := &fakeRunner{}
	app, _, _ := newTestApp(t, withAPI(client), withRunner(runner))

	// "only" narrows the list to one article, so no terminal is needed.
	if err := run(t, app, "article", "edit", "only"); err != nil {
		t.Fatal(err)
	}
	if gotID != 4 || runner.calls != 1 {
		t.Fatalf("GetArticle() ID = %d, editor runs = %d; want 4 and 1", gotID, runner.calls)
	}
}

func TestArticleEditWithIDDownloadsFilesAndBuildsEditorInvocation(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{listArticles: func(context.Context, bool) ([]api.Article, error) {
		t.Error("ListArticles() called with --id")
		return nil, nil
	}, getArticle: func(_ context.Context, id int) (api.ArticleContents, error) {
		if id != 27 {
			t.Errorf("GetArticle() ID = %d", id)
		}
		return api.ArticleContents{Contents: "# Existing\n"}, nil
	}}
	runner := &fakeRunner{run: func(_ string, args []string) error {
		contentsPath := args[len(args)-1]
		contents, err := os.ReadFile(contentsPath)
		if err != nil || string(contents) != "# Existing\n" {
			t.Errorf("contents file = %q, error = %v", contents, err)
		}
		id, err := os.ReadFile(filepath.Join(filepath.Dir(contentsPath), "id"))
		if err != nil || string(id) != "27\n" {
			t.Errorf("id file = %q, error = %v", id, err)
		}
		return nil
	}}
	app, stdout, _ := newTestApp(t, withAPI(client), withRunner(runner))
	app.Executable = func() (string, error) { return "/opt/Satellite's tools/sat", nil }

	if err := run(t, app, "article", "edit", "--id", "27"); err != nil {
		t.Fatal(err)
	}
	if runner.name != editorBinary || len(runner.args) != 3 || runner.args[0] != "-c" {
		t.Fatalf("Run() = %q %#v", runner.name, runner.args)
	}
	if runner.stdin != app.Stdin || runner.stdout != app.Stdout || runner.stderr != app.Stderr {
		t.Fatal("editor not attached to the app streams")
	}
	command := runner.args[1]
	workDir := filepath.Dir(runner.args[2])
	for _, want := range []string{"set nospell", "autocmd BufWritePost <buffer>", "'/opt/Satellite''s tools/sat'", "'article', 'save', '--work-dir'", vimString(workDir)} {
		if !strings.Contains(command, want) {
			t.Fatalf("editor command %q does not contain %q", command, want)
		}
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want nothing for an existing article", stdout.String())
	}
}

func TestArticleEditExecutableErrorStopsBeforeEditor(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("no executable")
	app, _, _ := newTestApp(t)
	app.Executable = func() (string, error) { return "", wantErr }

	// The default runner fails the test if the editor starts.
	err := run(t, app, "article", "new")
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "resolve sat executable") {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestArticleNewSavedAssignsJournal(t *testing.T) {
	t.Parallel()
	var gotID int
	var gotJournal string
	client := &fakeAPI{listJournals: func(context.Context) ([]api.Journal, error) {
		return []api.Journal{{ID: 1, Title: "Daily"}}, nil
	}, assignArticleJournal: func(_ context.Context, id int, journal string) (api.Article, error) {
		gotID, gotJournal = id, journal
		return api.Article{}, nil
	}}
	runner := &fakeRunner{run: saveFromEditor("new article", "81\n")}
	app, _, _ := newTestApp(t, withAPI(client), withRunner(runner), withTTY())
	app.Select = selectIndex(0, nil, nil)

	if err := run(t, app, "article", "new"); err != nil {
		t.Fatal(err)
	}
	if gotID != 81 || gotJournal != "Daily" {
		t.Fatalf("AssignArticleJournal() = (%d, %q), want (81, Daily)", gotID, gotJournal)
	}
}

func TestArticleNewWithoutSaveDoesNotAssign(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{assignArticleJournal: func(context.Context, int, string) (api.Article, error) {
		t.Error("AssignArticleJournal() called without a save")
		return api.Article{}, nil
	}}
	app, _, stderr := newTestApp(t, withAPI(client), withRunner(&fakeRunner{}))

	if err := run(t, app, "article", "new"); err != nil {
		t.Fatal(err)
	}
	if stderr.String() != "Nothing saved.\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestArticleEditorPreservesExitCode(t *testing.T) {
	t.Parallel()
	execErr := exec.Command("sh", "-c", "exit 6").Run()
	var processErr *exec.ExitError
	if !errors.As(execErr, &processErr) {
		t.Fatalf("test setup error = %v, want *exec.ExitError", execErr)
	}
	app, _, _ := newTestApp(t, withRunner(&fakeRunner{err: execErr}))

	err := run(t, app, "article", "new")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 6 {
		t.Fatalf("Execute() error = %#v, want ExitError code 6", err)
	}
	if !errors.Is(err, execErr) {
		t.Fatal("Execute() error does not wrap editor process error")
	}
}

func TestArticleAssignFetchesJournalsAndInvalidatesArticles(t *testing.T) {
	t.Parallel()
	var gotID int
	var gotJournal string
	var journalsCalls int
	client := &fakeAPI{listJournals: func(context.Context) ([]api.Journal, error) {
		journalsCalls++
		return []api.Journal{{ID: 3, Title: "Default"}, {ID: 7, Title: "Work"}, {ID: 9, Title: "  "}}, nil
	}, assignArticleJournal: func(_ context.Context, id int, journal string) (api.Article, error) {
		gotID, gotJournal = id, journal
		return api.Article{}, nil
	}}
	cfg := newFakeConfig(t)
	cfg.caches = map[string][]string{articleCacheName: {"1\tCached"}}
	app, _, _ := newTestApp(t, withConfig(cfg), withTTY())
	var selected []ui.Item
	var gotOpts ui.SelectOptions
	app.Select = selectIndex(1, &selected, &gotOpts)

	if err := assignArticle(t.Context(), app, client, 12, ""); err != nil {
		t.Fatal(err)
	}
	if journalsCalls != 1 {
		t.Fatalf("ListJournals() calls = %d, want 1", journalsCalls)
	}
	if gotID != 12 || gotJournal != "Work" {
		t.Fatalf("AssignArticleJournal() = (%d, %q), want (12, Work)", gotID, gotJournal)
	}
	// Blank titles are dropped and the API order is kept.
	wantItems := []ui.Item{{ID: "Default", Columns: []string{"Default"}}, {ID: "Work", Columns: []string{"Work"}}}
	if !reflect.DeepEqual(selected, wantItems) || gotOpts.Title != "Journals" {
		t.Fatalf("selector items = %#v, title %q; want %#v, Journals", selected, gotOpts.Title, wantItems)
	}
	if _, exists := cfg.caches[articleCacheName]; exists {
		t.Fatal("article cache kept after assignment, want removed")
	}
	if _, exists := cfg.caches["journals"]; exists {
		t.Fatal("journal cache written, want journals fetched on demand")
	}
}

func TestArticleAssignUsesSuppliedTitleDirectly(t *testing.T) {
	t.Parallel()
	var gotJournal string
	client := &fakeAPI{listJournals: func(context.Context) ([]api.Journal, error) {
		t.Error("ListJournals() called with supplied title")
		return nil, nil
	}, assignArticleJournal: func(_ context.Context, _ int, journal string) (api.Article, error) {
		gotJournal = journal
		return api.Article{}, nil
	}}
	app, _, _ := newTestApp(t)
	if err := assignArticle(t.Context(), app, client, 1, "Journal with spaces"); err != nil {
		t.Fatal(err)
	}
	if gotJournal != "Journal with spaces" {
		t.Fatalf("journal = %q", gotJournal)
	}
}

func TestArticleAssignFailureKeepsArticleCache(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("assignment failed")
	client := &fakeAPI{assignArticleJournal: func(context.Context, int, string) (api.Article, error) {
		return api.Article{}, wantErr
	}}
	cfg := newFakeConfig(t)
	cfg.caches = map[string][]string{articleCacheName: {"1\tCached"}}
	app, _, _ := newTestApp(t, withConfig(cfg))

	if err := assignArticle(t.Context(), app, client, 1, "Default"); !errors.Is(err, wantErr) {
		t.Fatalf("assignArticle() error = %v, want %v", err, wantErr)
	}
	if _, exists := cfg.caches[articleCacheName]; !exists {
		t.Fatal("article cache removed after a failed assignment, want retained")
	}
}

func TestCleanupWorkspacesRemovesOnlyOldEntries(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	oldDir := filepath.Join(tmpDir, "old")
	recentDir := filepath.Join(tmpDir, "recent")
	if err := os.Mkdir(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "backup.md"), []byte("backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(recentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	oldTime := now.Add(-31 * 24 * time.Hour)
	recentTime := now.Add(-29 * 24 * time.Hour)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recentDir, recentTime, recentTime); err != nil {
		t.Fatal(err)
	}

	if err := cleanupWorkspaces(tmpDir, now, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old workspace stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(recentDir); err != nil {
		t.Fatalf("recent workspace removed: %v", err)
	}
}

func TestVimQuotingAndEditorCommand(t *testing.T) {
	t.Parallel()
	if got, want := vimString("path with 'quote'"), "'path with ''quote'''"; got != want {
		t.Fatalf("vimString() = %q, want %q", got, want)
	}
	values := []string{"/path with spaces/sat", "it's", ""}
	if got, want := vimList(values), "['/path with spaces/sat', 'it''s', '']"; got != want {
		t.Fatalf("vimList() = %q, want %q", got, want)
	}
	got := editorCommand("/path with 'quote'/sat", "/tmp/work dir's")
	wantParts := []string{
		"set nospell | autocmd BufWritePost <buffer>",
		"system(['/path with ''quote''/sat', 'article', 'save', '--work-dir', '/tmp/work dir''s', '--hook'])",
		"if v:shell_error",
		"echomsg 'sat: save failed: ' . g:sat_out",
		"echo g:sat_out",
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("editorCommand() = %q, want part %q", got, want)
		}
	}
}

func TestEditorCommandArgumentOrder(t *testing.T) {
	t.Parallel()
	got := editorCommand("/bin/sat", "/tmp/work")
	want := "set nospell | autocmd BufWritePost <buffer> let g:sat_out = trim(system(['/bin/sat', 'article', 'save', '--work-dir', '/tmp/work', '--hook'])) | if v:shell_error | echohl ErrorMsg | echomsg 'sat: save failed: ' . g:sat_out | echohl None | else | echo g:sat_out | endif"
	if got != want {
		t.Fatalf("editorCommand() = %q, want %q", got, want)
	}
}
