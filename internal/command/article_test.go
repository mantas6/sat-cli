package command

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

type articleAPI struct {
	*stubAPI
	list func(context.Context, bool) ([]api.Article, error)
	get  func(context.Context, string) (api.ArticleContents, error)
}

func (a *articleAPI) ListArticles(ctx context.Context, all bool) ([]api.Article, error) {
	if a.list == nil {
		return nil, nil
	}
	return a.list(ctx, all)
}

func (a *articleAPI) GetArticle(ctx context.Context, id string) (api.ArticleContents, error) {
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
