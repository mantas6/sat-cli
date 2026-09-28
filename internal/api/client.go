// Package api provides the typed HTTP client for the Satellite API.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultTimeout = 30 * time.Second
	maxErrorBody   = 1024
	userAgent      = "sat-cli"
	redacted       = "[REDACTED]"
	textAccept     = "application/json, text/plain;q=0.9"
)

// Client calls a Satellite server.
type Client struct {
	baseURL   *url.URL
	token     string
	http      *http.Client
	userAgent string
}

// Option customizes a Client.
type Option func(*Client)

// WithHTTPClient makes a Client use the supplied HTTP transport and settings.
// A zero Timeout is replaced with the default timeout.
func WithHTTPClient(client *http.Client) Option {
	return func(target *Client) {
		if client == nil {
			return
		}
		clone := *client
		if clone.Timeout == 0 {
			clone.Timeout = defaultTimeout
		}
		target.http = &clone
	}
}

// WithTimeout sets the overall HTTP request timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(client *Client) {
		client.http.Timeout = timeout
	}
}

// NewClient returns an API client. Authenticated requests never follow
// redirects, so bearer credentials cannot leak to another host or scheme and a
// POST is never silently replayed as a GET; a 3xx response is returned as an
// *HTTPError. Unauthenticated requests use the configured redirect policy.
func NewClient(baseURL, token string, options ...Option) (*Client, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid base URL %q: use an absolute http or https URL", strings.TrimSpace(baseURL))
	}

	client := &Client{
		baseURL:   parsed,
		token:     strings.TrimSpace(token),
		http:      &http.Client{Timeout: defaultTimeout},
		userAgent: userAgent,
	}
	for _, option := range options {
		option(client)
	}
	if client.http == nil {
		client.http = &http.Client{Timeout: defaultTimeout}
	}

	return client, nil
}

// GetJSON performs an authenticated GET request and decodes its JSON response.
func (c *Client) GetJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out, true)
}

// SendJSON performs an authenticated JSON request and decodes its JSON response.
func (c *Client) SendJSON(ctx context.Context, method, path string, body, out any) error {
	return c.doJSON(ctx, method, path, nil, body, out, true)
}

// PostForm performs an authenticated form request and returns its response body.
func (c *Client) PostForm(ctx context.Context, path string, form url.Values) ([]byte, error) {
	request, err := c.newRequest(ctx, http.MethodPost, path, nil, strings.NewReader(form.Encode()), true)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	return c.doBytes(request, true)
}

// GetText performs an authenticated GET request and returns its response body.
func (c *Client) GetText(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.doText(ctx, http.MethodGet, path, query, true)
}

// GetTextUnauthenticated performs a public GET request without a bearer header.
func (c *Client) GetTextUnauthenticated(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.doText(ctx, http.MethodGet, path, query, false)
}

// doText performs a body-less request to a plain-text endpoint. JSON is still
// preferred in Accept so that framework error pages come back as JSON.
func (c *Client) doText(ctx context.Context, method, path string, query url.Values, authenticated bool) ([]byte, error) {
	request, err := c.newRequest(ctx, method, path, query, nil, authenticated)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", textAccept)

	return c.doBytes(request, authenticated)
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body, out any, authenticated bool) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode %s request body: %w", method, err)
		}
		reader = bytes.NewReader(data)
	}

	request, err := c.newRequest(ctx, method, path, query, reader, authenticated)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	response, err := c.do(request, authenticated)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if out == nil || response.StatusCode == http.StatusNoContent {
		_, err = io.Copy(io.Discard, response.Body)
		return err
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			// An empty success body leaves out at its zero value.
			return nil
		}
		return fmt.Errorf("decode %s %s response: %w", method, path, err)
	}
	// Drain trailing bytes so the connection can be reused.
	_, _ = io.Copy(io.Discard, response.Body)

	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader, authenticated bool) (*http.Request, error) {
	endpoint, err := c.resolve(path, query)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create %s request: %w", method, err)
	}
	request.Header.Set("User-Agent", c.userAgent)
	if authenticated && c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	return request, nil
}

func (c *Client) resolve(path string, query url.Values) (string, error) {
	resolved := *c.baseURL
	rawPath := strings.TrimSuffix(c.baseURL.EscapedPath(), "/") + "/" + strings.TrimPrefix(path, "/")
	unescapedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", fmt.Errorf("resolve API path %q: %w", path, err)
	}
	resolved.Path = unescapedPath
	resolved.RawPath = rawPath
	resolved.RawQuery = query.Encode()
	resolved.Fragment = ""

	return resolved.String(), nil
}

func (c *Client) doBytes(request *http.Request, authenticated bool) ([]byte, error) {
	response, err := c.do(request, authenticated)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s %s response: %w", request.Method, request.URL.EscapedPath(), err)
	}

	return data, nil
}

func (c *Client) do(request *http.Request, authenticated bool) (*http.Response, error) {
	httpClient := *c.http
	if authenticated {
		httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	response, err := httpClient.Do(request)
	if err != nil {
		if request.Context().Err() != nil {
			return nil, request.Context().Err()
		}
		return nil, err
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return response, nil
	}
	defer response.Body.Close()

	// Read enough to redact a token that straddles the maxErrorBody boundary
	// before truncating, so no fragment of it can survive the cut.
	limit := int64(maxErrorBody + len(c.token) + 1)
	body, readErr := io.ReadAll(io.LimitReader(response.Body, limit))
	if readErr != nil {
		return nil, fmt.Errorf("read error response from %s %s: %w", request.Method, request.URL.EscapedPath(), readErr)
	}
	responseBody, truncated := c.redactErrorBody(string(body), int64(len(body)) == limit)

	return nil, &HTTPError{
		Status:    response.StatusCode,
		Method:    request.Method,
		Path:      request.URL.EscapedPath(),
		Body:      responseBody,
		Truncated: truncated,
	}
}

// redactErrorBody removes the bearer token from an error body and bounds it to
// maxErrorBody bytes, cutting at a UTF-8 boundary. cut reports that the body
// was read up to its limit and may continue past it.
func (c *Client) redactErrorBody(body string, cut bool) (string, bool) {
	if c.token != "" {
		body = strings.ReplaceAll(body, c.token, redacted)
		if cut {
			// Drop a trailing partial token; the rest of it was never read.
			for size := min(len(c.token)-1, len(body)); size > 0; size-- {
				if strings.HasSuffix(body, c.token[:size]) {
					body = body[:len(body)-size]
					break
				}
			}
		}
	}
	body = strings.TrimSpace(body)

	truncated := cut
	if len(body) > maxErrorBody {
		end := maxErrorBody
		for end > 0 && !utf8.RuneStart(body[end]) {
			end--
		}
		body = body[:end]
		truncated = true
	}

	return body, truncated
}
