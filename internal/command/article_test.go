package command

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

type articleAPI struct {
	*stubAPI
	list func(context.Context, bool) ([]api.Article, error)
	get  func(context.Context, int) (api.ArticleContents, error)
}

func (a *articleAPI) ListArticles(ctx context.Context, all bool) ([]api.Article, error) {
	if a.list == nil {
		return nil, nil
	}
	return a.list(ctx, all)
}

func (a *articleAPI) GetArticle(ctx context.Context, id int) (api.ArticleContents, error) {
	if a.get == nil {
		return api.ArticleContents{}, nil
	}
	return a.get(ctx, id)
}

func TestArticleLineFormatsLegacyCacheLine(t *testing.T) {
	article := api.Article{
		ID:        42,
		Title:     "A title",
		WordCount: 123,
		CreatedAt: "2026-09-21",
		Journal:   &api.Journal{Title: "Notes"},
	}
	if got, want := articleLine(article), "42\tA title\t123w\t2026-09-21\tNotes"; got != want {
		t.Fatalf("articleLine() = %q, want %q", got, want)
	}
	article.Journal = nil
	if got, want := articleLine(article), "42\tA title\t123w\t2026-09-21\t"; got != want {
		t.Fatalf("articleLine() with nil journal = %q, want %q", got, want)
	}
}

func TestArticleLineNeutralisesSeparatorsInFields(t *testing.T) {
	article := api.Article{
		ID:        7,
		Title:     "Tabs\tand\nnew\r\nlines\r",
		WordCount: 1,
		CreatedAt: "today\n",
		Journal:   &api.Journal{Title: "Work\tLog"},
	}
	line := articleLine(article)
	if got, want := line, "7\tTabs and new lines \t1w\ttoday \tWork Log"; got != want {
		t.Fatalf("articleLine() = %q, want %q", got, want)
	}
	items := parseArticleLines([]string{line})
	if len(items) != 1 || items[0].ID != "7" || len(items[0].Columns) != 4 {
		t.Fatalf("parseArticleLines(articleLine()) = %#v, want one item with 4 columns", items)
	}
}

func TestParseArticleLinesToleratesLegacyShortLines(t *testing.T) {
	got := parseArticleLines([]string{
		"",
		" \t ",
		"7",
		"8\tTitle\t20w\t2026-09-21\tJournal",
	})
	want := []ui.Item{
		{ID: "7", Columns: []string{}},
		{ID: "8", Columns: []string{"Title", "20w", "2026-09-21", "Journal"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseArticleLines() = %#v, want %#v", got, want)
	}
}

func TestReverseItemsReturnsNewestFirstWithoutMutatingInput(t *testing.T) {
	items := []ui.Item{{ID: "oldest"}, {ID: "middle"}, {ID: "newest"}}
	got := reverseItems(items)
	if got[0].ID != "newest" || got[2].ID != "oldest" {
		t.Fatalf("reverseItems() = %#v", got)
	}
	if items[0].ID != "oldest" {
		t.Fatalf("reverseItems() mutated input: %#v", items)
	}
}

func TestCachedArticleItemsCacheHitSkipsAPI(t *testing.T) {
	called := false
	client := &articleAPI{stubAPI: &stubAPI{}, list: func(context.Context, bool) ([]api.Article, error) {
		called = true
		return nil, nil
	}}
	store := &cacheConfig{
		stubConfig: &stubConfig{},
		exists:     true,
		lines:      []string{"1\tOld", "2\tNew"},
	}
	app := &App{Config: store, Stderr: &bytes.Buffer{}}

	items, err := cachedArticleItems(context.Background(), app, client, true)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("ListArticles() called on a cache hit")
	}
	if len(items) != 2 || items[0].ID != "2" {
		t.Fatalf("cachedArticleItems() = %#v, want reversed cache", items)
	}
}

func TestCachedArticleItemsCacheMissFetchesAllAndWritesCache(t *testing.T) {
	var gotAll bool
	client := &articleAPI{stubAPI: &stubAPI{}, list: func(_ context.Context, all bool) ([]api.Article, error) {
		gotAll = all
		return []api.Article{
			{ID: 1, Title: "Old", WordCount: 10, CreatedAt: "yesterday"},
			{ID: 2, Title: "New", WordCount: 20, CreatedAt: "today", Journal: &api.Journal{Title: "Daily"}},
		}, nil
	}}
	store := &cacheConfig{stubConfig: &stubConfig{}}
	stderr := &bytes.Buffer{}
	app := &App{Config: store, Stderr: stderr}

	items, err := cachedArticleItems(context.Background(), app, client, true)
	if err != nil {
		t.Fatal(err)
	}
	if !gotAll {
		t.Fatal("ListArticles() all = false, want true")
	}
	wantLines := []string{"1\tOld\t10w\tyesterday\t", "2\tNew\t20w\ttoday\tDaily"}
	if store.writtenKey != articleCacheName || !reflect.DeepEqual(store.written, wantLines) {
		t.Fatalf("cache write = (%q, %#v), want (%q, %#v)", store.writtenKey, store.written, articleCacheName, wantLines)
	}
	if len(items) != 2 || items[0].ID != "2" {
		t.Fatalf("items = %#v, want newest first", items)
	}
	if stderr.String() != "Fetching articles list...\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseArticleID(t *testing.T) {
	for input, want := range map[string]int{"1": 1, " 42\n": 42} {
		if got, err := parseArticleID(input); err != nil || got != want {
			t.Fatalf("parseArticleID(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "0", "-3", "..", "new", "12a"} {
		if got, err := parseArticleID(input); err == nil {
			t.Fatalf("parseArticleID(%q) = %d, want error", input, got)
		}
	}
}

func TestArticleCommandsRejectInvalidIDs(t *testing.T) {
	for _, args := range [][]string{
		{"article", "read", "--id", ".."},
		{"article", "read", "--id", "0"},
		{"article", "edit", "--id", "new"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			client := &articleAPI{stubAPI: &stubAPI{}, get: func(context.Context, int) (api.ArticleContents, error) {
				t.Fatal("GetArticle() called with an invalid ID")
				return api.ArticleContents{}, nil
			}}
			app, _ := newArticleTestApp(client, nil)
			err := executeArticleTestCommand(app, args...)
			if err == nil || !strings.Contains(err.Error(), "invalid article ID") {
				t.Fatalf("Execute() error = %v, want invalid article ID", err)
			}
		})
	}
}
