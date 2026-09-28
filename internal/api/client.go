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
	defaultTimeout   = 30 * time.Second
	defaultUserAgent = "sat-cli"
	maxErrorBody     = 1024
	// maxTextBody bounds plain-text and form responses read into memory.
	maxTextBody = 8 << 20
	redacted    = "[REDACTED]"
	textAccept  = "application/json, text/plain;q=0.9"
)

// Client calls a Satellite server.
type Client struct {
	baseURL   *url.URL
	token     string
	userAgent string
	// authHTTP carries the bearer token and never follows redirects.
	authHTTP *http.Client
	// publicHTTP is used for unauthenticated requests and keeps the
	// configured redirect policy.
	publicHTTP *http.Client
}

type clientOptions struct {
	http      *http.Client
	timeout   time.Duration
	userAgent string
}

// Option customizes a Client. Options may be given in any order.
type Option func(*clientOptions)

// WithHTTPClient makes a Client use the supplied HTTP transport and settings.
// A nil client is ignored. When neither the client nor WithTimeout sets a
// timeout, the default timeout applies.
func WithHTTPClient(client *http.Client) Option {
	return func(options *clientOptions) {
		options.http = client
	}
}

// WithTimeout sets the overall HTTP request timeout, overriding the timeout
// of a client given to WithHTTPClient. A non-positive timeout is ignored.
func WithTimeout(timeout time.Duration) Option {
	return func(options *clientOptions) {
		options.timeout = timeout
	}
}

// WithUserAgent sets the User-Agent header sent with every request, for
// example "sat-cli/1.2.3". An empty value keeps the default "sat-cli".
func WithUserAgent(userAgent string) Option {
	return func(options *clientOptions) {
		options.userAgent = strings.TrimSpace(userAgent)
	}
}

// NewClient returns an API client. Authenticated requests never follow
// redirects, so bearer credentials cannot leak to another host or scheme and a
// POST is never silently replayed as a GET; a 3xx response is returned as an
// *HTTPError. Unauthenticated requests use the configured redirect policy.
func NewClient(baseURL, token string, options ...Option) (*Client, error) {
	parsed, err := ParseBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	settings := clientOptions{}
	for _, option := range options {
		option(&settings)
	}

	public := &http.Client{}
	if settings.http != nil {
		clone := *settings.http
		public = &clone
	}
	switch {
	case settings.timeout > 0:
		public.Timeout = settings.timeout
	case public.Timeout <= 0:
		public.Timeout = defaultTimeout
	}
	authenticated := *public
	authenticated.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	userAgent := settings.userAgent
	if userAgent == "" {
		userAgent = defaultUserAgent
	}

	return &Client{
		baseURL:    parsed,
		token:      strings.TrimSpace(token),
		userAgent:  userAgent,
		authHTTP:   &authenticated,
		publicHTTP: public,
	}, nil
}

// getJSON performs an authenticated GET request and decodes its JSON response.
func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, query, nil, out, true)
}

// sendJSON performs an authenticated JSON request and decodes its JSON response.
func (c *Client) sendJSON(ctx context.Context, method, path string, body, out any) error {
	return c.doJSON(ctx, method, path, nil, body, out, true)
}

// postForm performs an authenticated form request and returns its response body.
func (c *Client) postForm(ctx context.Context, path string, form url.Values) ([]byte, error) {
	request, err := c.newRequest(ctx, http.MethodPost, path, nil, strings.NewReader(form.Encode()), true)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	return c.doBytes(request, true)
}

// getText performs an authenticated GET request and returns its response body.
func (c *Client) getText(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.doText(ctx, http.MethodGet, path, query, true)
}

// getPublicText performs a public GET request without a bearer header.
func (c *Client) getPublicText(ctx context.Context, path string, query url.Values) ([]byte, error) {
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
			return fmt.Errorf("%s %s: encode request body: %w", method, path, err)
		}
		reader = bytes.NewReader(data)
	}

	request, err := c.newRequest(ctx, method, path, query, reader, authenticated)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.do(request, authenticated)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if out == nil || response.StatusCode == http.StatusNoContent {
		if _, err := io.Copy(io.Discard, response.Body); err != nil {
			return requestError(request, "read response", err)
		}
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			// An empty success body leaves out at its zero value.
			return nil
		}
		return requestError(request, "decode response", err)
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
		return nil, fmt.Errorf("%s %s: create request: %w", method, path, err)
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

	return resolved.String(), nil
}

func (c *Client) doBytes(request *http.Request, authenticated bool) ([]byte, error) {
	response, err := c.do(request, authenticated)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	data, err := io.ReadAll(io.LimitReader(response.Body, maxTextBody+1))
	if err != nil {
		return nil, requestError(request, "read response", err)
	}
	if len(data) > maxTextBody {
		return nil, requestError(request, "read response", fmt.Errorf("body exceeds %d bytes", maxTextBody))
	}

	return data, nil
}

func (c *Client) do(request *http.Request, authenticated bool) (*http.Response, error) {
	httpClient := c.publicHTTP
	if authenticated {
		httpClient = c.authHTTP
	}

	response, err := httpClient.Do(request)
	if err != nil {
		if ctxErr := request.Context().Err(); ctxErr != nil {
			return nil, requestError(request, "", ctxErr)
		}
		// *url.Error repeats the method and full URL; keep only the cause.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, requestError(request, "", err)
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
		return nil, requestError(request, "read error response", readErr)
	}
	responseBody, truncated := c.redactErrorBody(string(body), int64(len(body)) == limit)

	return nil, &HTTPError{
		StatusCode: response.StatusCode,
		Method:     request.Method,
		Path:       request.URL.EscapedPath(),
		Body:       responseBody,
		Message:    jsonMessage(responseBody),
		Truncated:  truncated,
	}
}

// requestError formats err as "METHOD /escaped/path: action: err", omitting
// the action when it is empty.
func requestError(request *http.Request, action string, err error) error {
	if action == "" {
		return fmt.Errorf("%s %s: %w", request.Method, request.URL.EscapedPath(), err)
	}
	return fmt.Errorf("%s %s: %s: %w", request.Method, request.URL.EscapedPath(), action, err)
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
