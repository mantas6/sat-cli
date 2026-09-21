package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// ListArticles returns recent articles, or all articles when all is true.
func (c *Client) ListArticles(ctx context.Context, all bool) ([]Article, error) {
	query := url.Values{}
	if all {
		query.Set("all", "1")
	}

	var articles []Article
	err := c.GetJSON(ctx, "/api/journals/articles", query, &articles)
	return articles, err
}

// GetArticle returns an article's Markdown contents.
func (c *Client) GetArticle(ctx context.Context, id string) (ArticleContents, error) {
	var contents ArticleContents
	err := c.GetJSON(ctx, JoinPath("api", "journals", "articles", id), nil, &contents)
	return contents, err
}

// CreateArticle creates an article from Markdown contents.
func (c *Client) CreateArticle(ctx context.Context, contents string) (Article, error) {
	var article Article
	err := c.SendJSON(ctx, http.MethodPost, "/api/journals/articles", map[string]string{"contents": contents}, &article)
	return article, err
}

// UpdateArticleContents replaces an article's Markdown contents.
func (c *Client) UpdateArticleContents(ctx context.Context, id, contents string) (Article, error) {
	var article Article
	err := c.SendJSON(ctx, http.MethodPut, JoinPath("api", "journals", "articles", id), map[string]string{"contents": contents}, &article)
	return article, err
}

// AssignArticleJournal assigns an article to a journal by title.
func (c *Client) AssignArticleJournal(ctx context.Context, id, journalTitle string) (Article, error) {
	var article Article
	err := c.SendJSON(ctx, http.MethodPut, JoinPath("api", "journals", "articles", id), map[string]string{"journal": journalTitle}, &article)
	return article, err
}

// ListJournals returns all journals.
func (c *Client) ListJournals(ctx context.Context) ([]Journal, error) {
	var journals []Journal
	err := c.GetJSON(ctx, "/api/journals", nil, &journals)
	return journals, err
}

// SavedTracks returns the legacy tab-delimited saved-track lines.
func (c *Client) SavedTracks(ctx context.Context) ([]string, error) {
	var response SavedTracksResponse
	if err := c.GetJSON(ctx, "/api/albums/saved", nil, &response); err != nil {
		return nil, err
	}

	lines := make([]string, len(response.Data))
	for index, item := range response.Data {
		lines[index] = item.Line
	}

	return lines, nil
}

// PlayTrack starts playback for a Spotify track or album ID.
func (c *Client) PlayTrack(ctx context.Context, id string) error {
	return c.spotifyRequest(ctx, JoinPath("api", "albums", "play", id))
}

// ControlPlayback performs one of pause, play, next, or previous.
func (c *Client) ControlPlayback(ctx context.Context, action string) error {
	return c.spotifyRequest(ctx, JoinPath("api", "albums", "control", action))
}

func (c *Client) spotifyRequest(ctx context.Context, path string) error {
	request, err := c.newRequest(ctx, http.MethodPut, path, nil, nil, true)
	if err != nil {
		return err
	}
	data, err := c.doBytes(request, true)
	if err != nil {
		return err
	}
	if message := strings.TrimSpace(string(data)); message != "" {
		return &SpotifyError{Message: message}
	}

	return nil
}

// Dashboard returns the plain-text dashboard.
func (c *Client) Dashboard(ctx context.Context) (string, error) {
	data, err := c.GetText(ctx, "/api/dash", nil)
	return string(data), err
}

// Weather returns a public plain-text forecast. Redirects follow the configured
// HTTP client's policy because this request never carries authentication.
func (c *Client) Weather(ctx context.Context, place string) (string, error) {
	data, err := c.GetTextUnauthenticated(ctx, JoinPath("api", "wt", place), nil)
	return string(data), err
}

// Notify posts a notification message.
func (c *Client) Notify(ctx context.Context, message string) error {
	_, err := c.PostForm(ctx, "/api/notify", url.Values{
		"message": {message},
	})
	return err
}
