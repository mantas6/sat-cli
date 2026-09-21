package command

import (
	"context"
	"errors"
	"io"
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

func TestArticleEditOffersNewFirstAndFetchesRecentArticles(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	var gotItems []ui.Item
	var gotQuery string
	runSelector = func(_ context.Context, _ io.Reader, _ io.Writer, items []ui.Item, options ui.SelectOptions) (ui.Item, error) {
		gotItems = append([]ui.Item(nil), items...)
		gotQuery = options.Query
		return items[0], nil
	}
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, list: func(_ context.Context, all bool) ([]api.Article, error) {
		if all {
			t.Fatal("ListArticles() all = true, want recent list")
		}
		return []api.Article{{ID: 1, Title: "Shared article older"}, {ID: 2, Title: "Shared article recent"}}, nil
	}}
	app, _, _, stderr := newArticleLifecycleApp(t, client)
	app.Runner = &recordingArticleRunner{}

	if err := executeArticleTestCommand(app, "article", "edit", "Shared", "article"); err != nil {
		t.Fatal(err)
	}
	if len(gotItems) != 3 || gotItems[0].ID != "0" || gotItems[0].Columns[0] != "New" || gotItems[1].ID != "2" {
		t.Fatalf("selector items = %#v, want New then reversed articles", gotItems)
	}
	if gotQuery != "Shared article" {
		t.Fatalf("selector query = %q", gotQuery)
	}
	if stderr.String() != "Nothing saved.\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestArticleEditWithIDDownloadsFilesAndBuildsEditorInvocation(t *testing.T) {
	previousExecutable := executablePath
	executablePath = func() (string, error) { return "/opt/Satellite's tools/sat", nil }
	t.Cleanup(func() { executablePath = previousExecutable })

	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, list: func(context.Context, bool) ([]api.Article, error) {
		t.Fatal("ListArticles() called with --id")
		return nil, nil
	}, get: func(_ context.Context, id string) (api.ArticleContents, error) {
		if id != "27" {
			t.Fatalf("GetArticle() ID = %q", id)
		}
		return api.ArticleContents{Contents: "# Existing\n"}, nil
	}}
	runner := &recordingArticleRunner{run: func(name string, args []string) error {
		if name != "nvim" {
			t.Fatalf("runner name = %q", name)
		}
		contentsPath := args[len(args)-1]
		contents, err := os.ReadFile(contentsPath)
		if err != nil || string(contents) != "# Existing\n" {
			t.Fatalf("contents file = %q, error = %v", contents, err)
		}
		id, err := os.ReadFile(filepath.Join(filepath.Dir(contentsPath), "id"))
		if err != nil || string(id) != "27\n" {
			t.Fatalf("id file = %q, error = %v", id, err)
		}
		return nil
	}}
	app, _, _, _ := newArticleLifecycleApp(t, client)
	app.Runner = runner

	if err := executeArticleTestCommand(app, "article", "edit", "--id", "27"); err != nil {
		t.Fatal(err)
	}
	if runner.name != editorBinary || len(runner.args) != 3 || runner.args[0] != "-c" {
		t.Fatalf("Run() = %q %#v", runner.name, runner.args)
	}
	command := runner.args[1]
	workDir := filepath.Dir(runner.args[2])
	for _, want := range []string{"set nospell", "autocmd BufWritePost <buffer>", "'/opt/Satellite''s tools/sat'", "'article', 'save', '--work-dir'", vimString(workDir)} {
		if !strings.Contains(command, want) {
			t.Fatalf("editor command %q does not contain %q", command, want)
		}
	}
}

func TestArticleNewSavedAssignsJournal(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	runSelector = func(_ context.Context, _ io.Reader, _ io.Writer, items []ui.Item, _ ui.SelectOptions) (ui.Item, error) {
		return items[0], nil
	}

	var gotID, gotJournal string
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, assign: func(_ context.Context, id, journal string) (api.Article, error) {
		gotID, gotJournal = id, journal
		return api.Article{}, nil
	}}
	runner := &recordingArticleRunner{run: func(_ string, args []string) error {
		workDir := filepath.Dir(args[len(args)-1])
		if err := os.WriteFile(filepath.Join(workDir, "contents.md"), []byte("new article"), 0o600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(workDir, "id"), []byte("81\n"), 0o600)
	}}
	app, store, _, _ := newArticleLifecycleApp(t, client)
	app.Runner = runner
	if err := store.WriteCacheLines(journalCacheName, []string{"Daily"}); err != nil {
		t.Fatal(err)
	}

	if err := executeArticleTestCommand(app, "article", "new"); err != nil {
		t.Fatal(err)
	}
	if gotID != "81" || gotJournal != "Daily" {
		t.Fatalf("AssignArticleJournal() = (%q, %q), want (81, Daily)", gotID, gotJournal)
	}
}

func TestArticleNewWithoutSaveDoesNotAssign(t *testing.T) {
	client := &articleLifecycleAPI{stubAPI: &stubAPI{}, assign: func(context.Context, string, string) (api.Article, error) {
		t.Fatal("AssignArticleJournal() called without a save")
		return api.Article{}, nil
	}}
	app, _, _, stderr := newArticleLifecycleApp(t, client)
	app.Runner = &recordingArticleRunner{}

	if err := executeArticleTestCommand(app, "article", "new"); err != nil {
		t.Fatal(err)
	}
	if stderr.String() != "Nothing saved.\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCleanupWorkspacesRemovesOnlyOldEntries(t *testing.T) {
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

func TestArticleEditorPreservesExitCode(t *testing.T) {
	execErr := exec.Command("sh", "-c", "exit 6").Run()
	var processErr *exec.ExitError
	if !errors.As(execErr, &processErr) {
		t.Fatalf("test setup error = %v, want *exec.ExitError", execErr)
	}
	app, _, _, _ := newArticleLifecycleApp(t, &articleLifecycleAPI{stubAPI: &stubAPI{}})
	app.Runner = &recordingArticleRunner{err: execErr}

	err := executeArticleTestCommand(app, "article", "new")
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 6 {
		t.Fatalf("Execute() error = %#v, want ExitError code 6", err)
	}
	if !errors.Is(err, execErr) {
		t.Fatal("Execute() error does not wrap editor process error")
	}
}

func TestVimQuotingAndEditorCommand(t *testing.T) {
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
	got := editorCommand("/bin/sat", "/tmp/work")
	want := "set nospell | autocmd BufWritePost <buffer> let g:sat_out = trim(system(['/bin/sat', 'article', 'save', '--work-dir', '/tmp/work', '--hook'])) | if v:shell_error | echohl ErrorMsg | echomsg 'sat: save failed: ' . g:sat_out | echohl None | else | echo g:sat_out | endif"
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("editorCommand() = %q, want %q", got, want)
	}
}
