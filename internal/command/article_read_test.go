package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

func newArticleTestApp(client APIClient, store *cacheConfig) (*App, *bytes.Buffer) {
	if store == nil {
		store = &cacheConfig{stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"}}
	}
	stdout := &bytes.Buffer{}
	return &App{
		Config: store,
		NewAPIClient: func(string, string) (APIClient, error) {
			return client, nil
		},
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: &bytes.Buffer{},
	}, stdout
}

func executeArticleTestCommand(app *App, args ...string) error {
	command := NewRootCommand(app)
	command.SetArgs(args)
	return command.Execute()
}

func TestArticleReadWithIDSkipsSelector(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	runSelector = func(context.Context, io.Reader, io.Writer, []ui.Item, ui.SelectOptions) (ui.Item, error) {
		t.Fatal("selector called with --id")
		return ui.Item{}, nil
	}

	var gotID string
	client := &articleAPI{stubAPI: &stubAPI{}, get: func(_ context.Context, id string) (api.ArticleContents, error) {
		gotID = id
		return api.ArticleContents{Contents: "contents"}, nil
	}}
	app, _ := newArticleTestApp(client, nil)
	if err := executeArticleTestCommand(app, "article", "read", "--id", "27"); err != nil {
		t.Fatal(err)
	}
	if gotID != "27" {
		t.Fatalf("GetArticle() ID = %q, want 27", gotID)
	}
}

func TestArticleReadPassesJoinedQueryToSelector(t *testing.T) {
	previous := runSelector
	t.Cleanup(func() { runSelector = previous })
	var gotQuery string
	runSelector = func(_ context.Context, _ io.Reader, _ io.Writer, items []ui.Item, opts ui.SelectOptions) (ui.Item, error) {
		gotQuery = opts.Query
		return items[0], nil
	}

	client := &articleAPI{stubAPI: &stubAPI{}, get: func(context.Context, string) (api.ArticleContents, error) {
		return api.ArticleContents{Contents: "contents"}, nil
	}}
	store := &cacheConfig{
		stubConfig: &stubConfig{baseURL: "https://satellite.test", token: "secret"},
		exists:     true,
		lines:      []string{"1\tShared article first", "2\tShared article second"},
	}
	app, _ := newArticleTestApp(client, store)
	if err := executeArticleTestCommand(app, "article", "read", "Shared", "article"); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "Shared article" {
		t.Fatalf("selector query = %q, want %q", gotQuery, "Shared article")
	}
}

func TestArticleReadWritesExactRawContentForNonTerminal(t *testing.T) {
	client := &articleAPI{stubAPI: &stubAPI{}, get: func(context.Context, string) (api.ArticleContents, error) {
		return api.ArticleContents{Contents: "# Heading\n\nBody"}, nil
	}}
	app, stdout := newArticleTestApp(client, nil)
	if err := executeArticleTestCommand(app, "article", "read", "--id", "1"); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "# Heading\n\nBody\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestArticleReadRawFlagBypassesPagerOnTerminal(t *testing.T) {
	previous := runPager
	t.Cleanup(func() { runPager = previous })
	runPager = func(context.Context, io.Reader, io.Writer, string, ui.PageOptions) error {
		t.Fatal("pager called with --raw")
		return nil
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	client := &articleAPI{stubAPI: &stubAPI{}, get: func(context.Context, string) (api.ArticleContents, error) {
		return api.ArticleContents{Contents: "raw\n"}, nil
	}}
	app, _ := newArticleTestApp(client, nil)
	app.Stdout = writer
	app.IsTerminal = func(int) bool { return true }
	if err := executeArticleTestCommand(app, "article", "read", "--id", "1", "--raw"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "raw\n" {
		t.Fatalf("stdout = %q, want raw content", data)
	}
}

func TestArticleReadInvokesPagerForTerminal(t *testing.T) {
	previous := runPager
	t.Cleanup(func() { runPager = previous })
	var gotContent string
	var gotOptions ui.PageOptions
	runPager = func(_ context.Context, _ io.Reader, _ io.Writer, content string, opts ui.PageOptions) error {
		gotContent = content
		gotOptions = opts
		return nil
	}

	client := &articleAPI{stubAPI: &stubAPI{}, get: func(context.Context, string) (api.ArticleContents, error) {
		return api.ArticleContents{Contents: "# Markdown"}, nil
	}}
	app, _ := newArticleTestApp(client, nil)
	app.Stdout = os.Stdout
	app.IsTerminal = func(int) bool { return true }
	app.TerminalSize = func(int) (int, int, error) { return 100, 35, nil }
	if err := executeArticleTestCommand(app, "article", "read", "--id", "1"); err != nil {
		t.Fatal(err)
	}
	if gotContent != "# Markdown" {
		t.Fatalf("pager content = %q, want raw Markdown", gotContent)
	}
	if gotOptions.Width != 100 || gotOptions.Height != 35 || gotOptions.Title != "Article" {
		t.Fatalf("pager options = %#v", gotOptions)
	}
}

func TestArticleReadAPIErrorPropagates(t *testing.T) {
	wantErr := errors.New("get article failed")
	client := &articleAPI{stubAPI: &stubAPI{}, get: func(context.Context, string) (api.ArticleContents, error) {
		return api.ArticleContents{}, wantErr
	}}
	app, _ := newArticleTestApp(client, nil)
	err := executeArticleTestCommand(app, "article", "read", "--id", "1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
