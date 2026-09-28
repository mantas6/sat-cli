package command

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

func TestArticleReadWithIDSkipsSelector(t *testing.T) {
	t.Parallel()
	var gotID int
	client := &fakeAPI{getArticle: func(_ context.Context, id int) (api.ArticleContents, error) {
		gotID = id
		return api.ArticleContents{Contents: "contents"}, nil
	}}
	// newTestApp fails the test if the selector runs.
	app, stdout, _ := newTestApp(t, withAPI(client))
	if err := run(t, app, "article", "read", "--id", "27"); err != nil {
		t.Fatal(err)
	}
	if gotID != 27 || stdout.String() != "contents\n" {
		t.Fatalf("GetArticle() ID = %d, stdout = %q; want 27 and contents", gotID, stdout.String())
	}
}

func TestArticleReadPassesJoinedQueryToSelector(t *testing.T) {
	t.Parallel()
	var gotID int
	client := &fakeAPI{getArticle: func(_ context.Context, id int) (api.ArticleContents, error) {
		gotID = id
		return api.ArticleContents{Contents: "contents"}, nil
	}}
	cfg := newFakeConfig(t)
	cfg.caches = map[string][]string{articleCacheName: {"1\tShared article first", "2\tShared article second"}}
	app, stdout, _ := newTestApp(t, withAPI(client), withConfig(cfg), withTTY())
	var gotOpts ui.SelectOptions
	app.Select = selectIndex(0, nil, &gotOpts)
	app.TermSize = func() (int, int, bool) { return 90, 30, true }
	var paged string
	app.Page = func(_ context.Context, _ io.Reader, _ io.Writer, content string, _ ui.PageOptions) error {
		paged = content
		return nil
	}

	if err := run(t, app, "article", "read", "Shared", "article"); err != nil {
		t.Fatal(err)
	}
	if gotOpts.Query != "Shared article" || gotOpts.Title != "Articles" || gotOpts.Width != 90 || gotOpts.Height != 30 {
		t.Fatalf("selector options = %#v", gotOpts)
	}
	if gotID != 2 || paged != "contents" || stdout.Len() != 0 {
		t.Fatalf("GetArticle() ID = %d, paged %q, stdout %q; want newest article paged", gotID, paged, stdout.String())
	}
}

func TestArticleReadWritesExactRawContentForNonTerminal(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{getArticle: func(context.Context, int) (api.ArticleContents, error) {
		return api.ArticleContents{Contents: "# Heading\n\nBody"}, nil
	}}
	app, stdout, _ := newTestApp(t, withAPI(client))
	if err := run(t, app, "article", "read", "--id", "1"); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "# Heading\n\nBody\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestArticleReadRawFlagBypassesPagerOnTerminal(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{getArticle: func(context.Context, int) (api.ArticleContents, error) {
		return api.ArticleContents{Contents: "raw\n"}, nil
	}}
	// newTestApp fails the test if the pager runs.
	app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
	if err := run(t, app, "article", "read", "--id", "1", "--raw"); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "raw\n" {
		t.Fatalf("stdout = %q, want raw content", stdout.String())
	}
}

func TestArticleReadInvokesPagerForTerminal(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{getArticle: func(context.Context, int) (api.ArticleContents, error) {
		return api.ArticleContents{Contents: "# Markdown"}, nil
	}}
	app, stdout, _ := newTestApp(t, withAPI(client), withTTY())
	app.TermSize = func() (int, int, bool) { return 100, 35, true }
	var gotContent string
	var gotOptions ui.PageOptions
	var gotOut io.Writer
	app.Page = func(_ context.Context, _ io.Reader, out io.Writer, content string, opts ui.PageOptions) error {
		gotOut, gotContent, gotOptions = out, content, opts
		return nil
	}

	if err := run(t, app, "article", "read", "--id", "1"); err != nil {
		t.Fatal(err)
	}
	if gotContent != "# Markdown" {
		t.Fatalf("pager content = %q, want raw Markdown", gotContent)
	}
	if gotOptions != (ui.PageOptions{Title: "Article", Size: ui.Size{Width: 100, Height: 35}}) {
		t.Fatalf("pager options = %#v", gotOptions)
	}
	if gotOut != app.Stdout || stdout.Len() != 0 {
		t.Fatalf("pager output = %v, stdout = %q; want the pager to own stdout", gotOut, stdout.String())
	}
}

func TestArticleReadPagerSizeFallsBackWhenUnknown(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t, withTTY())
	var gotOptions ui.PageOptions
	app.Page = func(_ context.Context, _ io.Reader, _ io.Writer, _ string, opts ui.PageOptions) error {
		gotOptions = opts
		return nil
	}
	if err := run(t, app, "article", "read", "--id", "1"); err != nil {
		t.Fatal(err)
	}
	// Zero sizes make the pager use its 80x24 default.
	if gotOptions != (ui.PageOptions{Title: "Article"}) {
		t.Fatalf("pager options = %#v, want no size", gotOptions)
	}
}

func TestArticleReadAPIErrorPropagates(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("get article failed")
	client := &fakeAPI{getArticle: func(context.Context, int) (api.ArticleContents, error) {
		return api.ArticleContents{}, wantErr
	}}
	app, _, _ := newTestApp(t, withAPI(client))
	if err := run(t, app, "article", "read", "--id", "1"); !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
