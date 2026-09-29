package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PlaybackAction is a Spotify playback control accepted by ControlPlayback.
type PlaybackAction string

// Playback actions supported by the Satellite API.
const (
	Pause    PlaybackAction = "pause"
	Play     PlaybackAction = "play"
	Next     PlaybackAction = "next"
	Previous PlaybackAction = "previous"
)

func articlePath(id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid article ID %d", id)
	}
	return joinPath("api", "journals", "articles", strconv.Itoa(id))
}

// ListArticles returns recent articles, or all articles when all is true.
func (c *Client) ListArticles(ctx context.Context, all bool) ([]Article, error) {
	query := url.Values{}
	if all {
		query.Set("all", "1")
	}

	var articles []Article
	err := c.getJSON(ctx, "/api/journals/articles", query, &articles)
	return articles, err
}

// GetArticle returns an article's Markdown contents.
func (c *Client) GetArticle(ctx context.Context, id int) (ArticleContents, error) {
	path, err := articlePath(id)
	if err != nil {
		return ArticleContents{}, err
	}
	var contents ArticleContents
	err = c.getJSON(ctx, path, nil, &contents)
	return contents, err
}

// CreateArticle creates an article from Markdown contents.
func (c *Client) CreateArticle(ctx context.Context, contents string) (Article, error) {
	var article Article
	err := c.sendJSON(ctx, http.MethodPost, "/api/journals/articles", articleContentsRequest{Contents: contents}, &article)
	return article, err
}

// UpdateArticleContents replaces an article's Markdown contents.
func (c *Client) UpdateArticleContents(ctx context.Context, id int, contents string) (Article, error) {
	path, err := articlePath(id)
	if err != nil {
		return Article{}, err
	}
	var article Article
	err = c.sendJSON(ctx, http.MethodPut, path, articleContentsRequest{Contents: contents}, &article)
	return article, err
}

// AssignArticleJournal assigns an article to a journal by title.
func (c *Client) AssignArticleJournal(ctx context.Context, id int, journalTitle string) (Article, error) {
	path, err := articlePath(id)
	if err != nil {
		return Article{}, err
	}
	var article Article
	err = c.sendJSON(ctx, http.MethodPut, path, articleJournalRequest{Journal: journalTitle}, &article)
	return article, err
}

// ListJournals returns all journals.
func (c *Client) ListJournals(ctx context.Context) ([]Journal, error) {
	var journals []Journal
	err := c.getJSON(ctx, "/api/journals", nil, &journals)
	return journals, err
}

// SavedTracks returns the legacy tab-delimited saved-track lines.
//
// Newer satellites wrap the collection in a {"data": [...]} envelope while
// older ones return a bare array, so both shapes are accepted.
func (c *Client) SavedTracks(ctx context.Context) ([]string, error) {
	var raw json.RawMessage
	if err := c.getJSON(ctx, "/api/albums/saved", nil, &raw); err != nil {
		return nil, err
	}

	var tracks []savedTrack
	body := bytes.TrimSpace(raw)
	switch {
	case len(body) == 0:
		// Empty success body: no tracks.
	case body[0] == '[':
		// Legacy bare array from a satellite without the wrapping change.
		if err := json.Unmarshal(body, &tracks); err != nil {
			return nil, fmt.Errorf("GET /api/albums/saved: decode response: %w", err)
		}
	default:
		var payload struct {
			Data []savedTrack `json:"data"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("GET /api/albums/saved: decode response: %w", err)
		}
		tracks = payload.Data
	}

	lines := make([]string, len(tracks))
	for index, track := range tracks {
		lines[index] = track.Line
	}

	return lines, nil
}

// PlayTrack starts playback for a Spotify track or album ID.
func (c *Client) PlayTrack(ctx context.Context, id string) error {
	path, err := joinPath("api", "albums", "play", id)
	if err != nil {
		return err
	}
	return c.spotifyRequest(ctx, path)
}

// ControlPlayback performs one of Pause, Play, Next or Previous.
func (c *Client) ControlPlayback(ctx context.Context, action PlaybackAction) error {
	switch action {
	case Pause, Play, Next, Previous:
	default:
		return fmt.Errorf("invalid playback action %q", action)
	}
	path, err := joinPath("api", "albums", "control", string(action))
	if err != nil {
		return err
	}
	return c.spotifyRequest(ctx, path)
}

func (c *Client) spotifyRequest(ctx context.Context, path string) error {
	data, err := c.doText(ctx, http.MethodPut, path, nil, true)
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
	data, err := c.getText(ctx, "/api/dash", nil)
	return string(data), err
}

// Weather returns a public plain-text forecast. Redirects follow the configured
// HTTP client's policy because this request never carries authentication.
func (c *Client) Weather(ctx context.Context, place string) (string, error) {
	segments := []string{"api", "wt"}
	if place != "" {
		segments = append(segments, place)
	}
	path, err := joinPath(segments...)
	if err != nil {
		return "", err
	}
	data, err := c.getPublicText(ctx, path, nil)
	return string(data), err
}

// Notify posts a notification message.
func (c *Client) Notify(ctx context.Context, message string) error {
	_, err := c.postForm(ctx, "/api/notify", url.Values{
		"message": {message},
	})
	return err
}
