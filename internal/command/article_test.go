package command

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/ui"
)

func TestArticleItemFormatsLegacyCacheLine(t *testing.T) {
	t.Parallel()
	article := api.Article{
		ID:        42,
		Title:     "A title",
		WordCount: 123,
		CreatedAt: "2026-09-21",
		Journal:   &api.Journal{Title: "Notes"},
	}
	if got, want := cacheLine(articleItem(article)), "42\tA title\t123w\t2026-09-21\tNotes"; got != want {
		t.Fatalf("cacheLine(articleItem()) = %q, want %q", got, want)
	}
	article.Journal = nil
	if got, want := cacheLine(articleItem(article)), "42\tA title\t123w\t2026-09-21\t"; got != want {
		t.Fatalf("cacheLine(articleItem()) with nil journal = %q, want %q", got, want)
	}
}

func TestArticleItemNeutralisesSeparatorsInFields(t *testing.T) {
	t.Parallel()
	article := api.Article{
		ID:        7,
		Title:     "Tabs\tand\nnew\r\nlines\r",
		WordCount: 1,
		CreatedAt: "today\n",
		Journal:   &api.Journal{Title: "Work\tLog"},
	}
	item := articleItem(article)
	line := cacheLine(item)
	if got, want := line, "7\tTabs and new lines \t1w\ttoday \tWork Log"; got != want {
		t.Fatalf("cacheLine(articleItem()) = %q, want %q", got, want)
	}
	items := parseCacheLines([]string{line})
	if !reflect.DeepEqual(items, []ui.Item{item}) {
		t.Fatalf("parseCacheLines(cacheLine(item)) = %#v, want %#v", items, []ui.Item{item})
	}
}

func TestParseCacheLinesToleratesLegacyShortLines(t *testing.T) {
	t.Parallel()
	got := parseCacheLines([]string{
		"",
		" \t ",
		"\tno ID",
		"7",
		"8\tTitle\t20w\t2026-09-21\tJournal",
		"track-id\tArtist\t/Album\t/03.\tTitle",
	})
	want := []ui.Item{
		{ID: "7", Columns: []string{}},
		{ID: "8", Columns: []string{"Title", "20w", "2026-09-21", "Journal"}},
		{ID: "track-id", Columns: []string{"Artist", "/Album", "/03.", "Title"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCacheLines() = %#v, want %#v", got, want)
	}
}

func TestCachedArticleItemsCacheHitSkipsAPI(t *testing.T) {
	t.Parallel()
	client := &fakeAPI{listArticles: func(context.Context, bool) ([]api.Article, error) {
		t.Error("ListArticles() called on a cache hit")
		return nil, nil
	}}
	cfg := newFakeConfig(t)
	cfg.caches = map[string][]string{articleCacheName: {"1\tOld", "2\tNew"}}
	app, _, stderr := newTestApp(t, withConfig(cfg))

	items, err := cachedArticleItems(t.Context(), app, client, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []ui.Item{{ID: "2", Columns: []string{"New"}}, {ID: "1", Columns: []string{"Old"}}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("cachedArticleItems() = %#v, want reversed cache %#v", items, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want nothing on a cache hit", stderr.String())
	}
}

func TestCachedArticleItemsCacheMissFetchesAllAndWritesCache(t *testing.T) {
	t.Parallel()
	var gotAll bool
	client := &fakeAPI{listArticles: func(_ context.Context, all bool) ([]api.Article, error) {
		gotAll = all
		return []api.Article{
			{ID: 1, Title: "Old", WordCount: 10, CreatedAt: "yesterday"},
			{ID: 2, Title: "New", WordCount: 20, CreatedAt: "today", Journal: &api.Journal{Title: "Daily"}},
		}, nil
	}}
	cfg := newFakeConfig(t)
	app, _, stderr := newTestApp(t, withConfig(cfg))

	items, err := cachedArticleItems(t.Context(), app, client, true)
	if err != nil {
		t.Fatal(err)
	}
	if !gotAll {
		t.Fatal("ListArticles() all = false, want true")
	}
	wantLines := []string{"1\tOld\t10w\tyesterday\t", "2\tNew\t20w\ttoday\tDaily"}
	if got := cfg.caches[articleCacheName]; !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("article cache = %#v, want %#v", got, wantLines)
	}
	if want := parseCacheLines([]string{wantLines[1], wantLines[0]}); !reflect.DeepEqual(items, want) {
		t.Fatalf("items = %#v, want newest first %#v", items, want)
	}
	if stderr.String() != "Fetching articles list...\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseArticleID(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]int{"1": 1, " 42\n": 42} {
		if got, err := parseArticleID(input); err != nil || got != want {
			t.Fatalf("parseArticleID(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "0", "-3", "..", "new", "12a"} {
		if got, err := parseArticleID(input); err == nil || !strings.Contains(err.Error(), "invalid article ID") {
			t.Fatalf("parseArticleID(%q) = %d, %v; want invalid article ID", input, got, err)
		}
	}
}

func TestArticleCommandsRejectInvalidIDs(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"article", "read", "--id", ".."},
		{"article", "read", "--id", "0"},
		{"article", "edit", "--id", "new"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			client := &fakeAPI{getArticle: func(context.Context, int) (api.ArticleContents, error) {
				t.Error("GetArticle() called with an invalid ID")
				return api.ArticleContents{}, nil
			}}
			app, _, _ := newTestApp(t, withAPI(client))
			err := run(t, app, args...)
			if err == nil || !strings.Contains(err.Error(), "invalid article ID") {
				t.Fatalf("Execute() error = %v, want invalid article ID", err)
			}
		})
	}
}

func TestArticleCommandsRejectEmptyIDFlag(t *testing.T) {
	t.Parallel()
	for _, sub := range []string{"read", "edit"} {
		t.Run(sub, func(t *testing.T) {
			t.Parallel()
			app, _, _ := newTestApp(t)
			err := run(t, app, "article", sub, "--id", " ")
			if err == nil || err.Error() != "article ID must not be empty" {
				t.Fatalf("Execute() error = %v, want empty ID error", err)
			}
		})
	}
}
