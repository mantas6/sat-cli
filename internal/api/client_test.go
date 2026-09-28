package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestClientMethods(t *testing.T) {
	t.Parallel()
	type call func(ctx context.Context, client *Client) (any, error)
	noContent := func(writer http.ResponseWriter) { writer.WriteHeader(http.StatusNoContent) }
	respondJSON := func(value any) func(http.ResponseWriter) {
		return func(writer http.ResponseWriter) { writeJSON(t, writer, value) }
	}
	control := func(action PlaybackAction) call {
		return func(ctx context.Context, client *Client) (any, error) {
			return nil, client.ControlPlayback(ctx, action)
		}
	}
	tests := []struct {
		name    string
		request string // method and escaped path
		query   string
		accept  string
		body    map[string]string // expected JSON request body; nil for none
		respond func(http.ResponseWriter)
		call    call
		want    any
	}{
		{
			name: "ListArticles all", request: "GET /api/journals/articles", query: "all=1", accept: "application/json",
			respond: respondJSON([]Article{{ID: 1, Title: "First", WordCount: 10, CreatedAt: "today"}}),
			call:    func(ctx context.Context, c *Client) (any, error) { return c.ListArticles(ctx, true) },
			want:    []Article{{ID: 1, Title: "First", WordCount: 10, CreatedAt: "today"}},
		},
		{
			name: "ListArticles recent", request: "GET /api/journals/articles", accept: "application/json",
			respond: respondJSON([]Article{{ID: 2, Journal: &Journal{ID: 3, Title: "Work"}}}),
			call:    func(ctx context.Context, c *Client) (any, error) { return c.ListArticles(ctx, false) },
			want:    []Article{{ID: 2, Journal: &Journal{ID: 3, Title: "Work"}}},
		},
		{
			name: "GetArticle", request: "GET /api/journals/articles/41", accept: "application/json",
			respond: respondJSON(ArticleContents{Contents: "# text"}),
			call:    func(ctx context.Context, c *Client) (any, error) { return c.GetArticle(ctx, 41) },
			want:    ArticleContents{Contents: "# text"},
		},
		{
			name: "CreateArticle", request: "POST /api/journals/articles", accept: "application/json",
			body:    map[string]string{"contents": "new text"},
			respond: respondJSON(Article{ID: 2, WordCount: 2}),
			call:    func(ctx context.Context, c *Client) (any, error) { return c.CreateArticle(ctx, "new text") },
			want:    Article{ID: 2, WordCount: 2},
		},
		{
			name: "UpdateArticleContents", request: "PUT /api/journals/articles/42", accept: "application/json",
			body:    map[string]string{"contents": "updated"},
			respond: respondJSON(Article{ID: 42}),
			call:    func(ctx context.Context, c *Client) (any, error) { return c.UpdateArticleContents(ctx, 42, "updated") },
			want:    Article{ID: 42},
		},
		{
			name: "AssignArticleJournal", request: "PUT /api/journals/articles/43", accept: "application/json",
			body:    map[string]string{"journal": "Work"},
			respond: respondJSON(Article{ID: 43, Journal: &Journal{Title: "Work"}}),
			call:    func(ctx context.Context, c *Client) (any, error) { return c.AssignArticleJournal(ctx, 43, "Work") },
			want:    Article{ID: 43, Journal: &Journal{Title: "Work"}},
		},
		{
			name: "ListJournals", request: "GET /api/journals", accept: "application/json",
			respond: respondJSON([]Journal{{ID: 3, Title: "Work"}}),
			call:    func(ctx context.Context, c *Client) (any, error) { return c.ListJournals(ctx) },
			want:    []Journal{{ID: 3, Title: "Work"}},
		},
		{
			name: "SavedTracks", request: "GET /api/albums/saved", accept: "application/json",
			respond: func(writer http.ResponseWriter) {
				_, _ = io.WriteString(writer, `[{"line":"track-id\tartist\t/album\t/01.\ttitle"}]`)
			},
			call: func(ctx context.Context, c *Client) (any, error) { return c.SavedTracks(ctx) },
			want: []string{"track-id\tartist\t/album\t/01.\ttitle"},
		},
		{
			name: "PlayTrack", request: "PUT /api/albums/play/track%2Fid", accept: textAccept, respond: noContent,
			call: func(ctx context.Context, c *Client) (any, error) { return nil, c.PlayTrack(ctx, "track/id") },
		},
		{name: "ControlPlayback pause", request: "PUT /api/albums/control/pause", accept: textAccept, respond: noContent, call: control(Pause)},
		{name: "ControlPlayback play", request: "PUT /api/albums/control/play", accept: textAccept, respond: noContent, call: control(Play)},
		{name: "ControlPlayback next", request: "PUT /api/albums/control/next", accept: textAccept, respond: noContent, call: control(Next)},
		{name: "ControlPlayback previous", request: "PUT /api/albums/control/previous", accept: textAccept, respond: noContent, call: control(Previous)},
		{
			name: "Dashboard", request: "GET /api/dash", accept: textAccept,
			respond: func(writer http.ResponseWriter) { _, _ = io.WriteString(writer, "dashboard\n") },
			call:    func(ctx context.Context, c *Client) (any, error) { return c.Dashboard(ctx) },
			want:    "dashboard\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				if got := request.Method + " " + request.URL.EscapedPath(); got != test.request {
					t.Errorf("request = %s, want %s", got, test.request)
				}
				if request.URL.RawQuery != test.query {
					t.Errorf("query = %q, want %q", request.URL.RawQuery, test.query)
				}
				if got := request.Header.Get("Authorization"); got != "Bearer test-token" {
					t.Errorf("authorization = %q", got)
				}
				if got := request.Header.Get("User-Agent"); got != "sat-cli" {
					t.Errorf("user agent = %q", got)
				}
				if got := request.Header.Get("Accept"); got != test.accept {
					t.Errorf("accept = %q, want %q", got, test.accept)
				}
				if test.body != nil {
					assertJSONBody(t, request, test.body)
				} else if got := request.Header.Get("Content-Type"); got != "" {
					t.Errorf("content type = %q, want none without a body", got)
				}
				test.respond(writer)
			}))
			t.Cleanup(server.Close)

			got, err := test.call(context.Background(), newTestClient(t, server.URL, "test-token"))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("result = %#v, want %#v", got, test.want)
			}
			if requests.Load() != 1 {
				t.Fatalf("server received %d requests, want 1", requests.Load())
			}
		})
	}
}

func TestNotifyEncodesForm(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/notify" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("content type = %q", got)
		}
		if err := request.ParseForm(); err != nil {
			t.Error(err)
		}
		if request.PostForm.Get("message") != "a message & more" {
			t.Errorf("form = %#v", request.PostForm)
		}
		if _, ok := request.PostForm["expire"]; ok {
			t.Errorf("form unexpectedly contains expire: %#v", request.PostForm)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	if err := client.Notify(context.Background(), "a message & more"); err != nil {
		t.Fatal(err)
	}
}

func TestWeatherEscapesPlaceAndOmitsAuthentication(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/api/wt/New%20York%2FUS" {
			t.Errorf("escaped path = %q", request.URL.EscapedPath())
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Errorf("public request has authorization %q", authorization)
		}
		_, _ = io.WriteString(writer, "sunny")
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "do-not-send")
	weather, err := client.Weather(context.Background(), "New York/US")
	if err != nil || weather != "sunny" {
		t.Fatalf("Weather() = %q, %v", weather, err)
	}
}

func TestWeatherWithoutPlaceOmitsSegmentAndAuthentication(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/api/wt" {
			t.Errorf("escaped path = %q", request.URL.EscapedPath())
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Errorf("public request has authorization %q", authorization)
		}
		_, _ = io.WriteString(writer, "sunny")
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "do-not-send")
	weather, err := client.Weather(context.Background(), "")
	if err != nil || weather != "sunny" {
		t.Fatalf("Weather() = %q, %v", weather, err)
	}
}

func TestHTTPErrorMappingAndTruncation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   int
		body     string
		want     string
		sentinel error
	}{
		{http.StatusUnauthorized, `{"message":"bad credentials"}`, "bad credentials; token is invalid or expired", ErrUnauthorized},
		{http.StatusForbidden, `{"message":"ability denied"}`, "ability denied; token lacks the required ability", ErrForbidden},
		{http.StatusUnprocessableEntity, `{"message":"The contents field is required."}`, "The contents field is required.", nil},
		{http.StatusInternalServerError, "server exploded", "server exploded", nil},
	}

	for _, test := range tests {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body)
			}))
			t.Cleanup(server.Close)

			client := newTestClient(t, server.URL, "token")
			_, err := client.getText(context.Background(), "/failure", nil)
			var httpError *HTTPError
			if !errors.As(err, &httpError) {
				t.Fatalf("error type = %T (%v)", err, err)
			}
			if httpError.StatusCode != test.status || httpError.Method != http.MethodGet || httpError.Path != "/failure" {
				t.Fatalf("HTTPError = %#v", httpError)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want %q", err, test.want)
			}
			if strings.Contains(err.Error(), "sat auth login") {
				t.Fatalf("error = %q, want no CLI hint from the api package", err)
			}
			for _, sentinel := range []error{ErrUnauthorized, ErrForbidden} {
				if got, want := errors.Is(err, sentinel), sentinel == test.sentinel; got != want {
					t.Fatalf("errors.Is(err, %v) = %v, want %v", sentinel, got, want)
				}
			}
		})
	}

	secret := "secret-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(writer, secret+strings.Repeat("x", 2000))
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t, server.URL, secret)
	_, err := client.getText(context.Background(), "/large", nil)
	var httpError *HTTPError
	if !errors.As(err, &httpError) {
		t.Fatalf("error type = %T", err)
	}
	if !httpError.Truncated || len(httpError.Body) > maxErrorBody || !strings.Contains(err.Error(), "response body truncated") {
		t.Fatalf("truncated error = %#v (%v)", httpError, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
			_, _ = io.WriteString(writer, "late")
		case <-request.Context().Done():
		}
	}))
	t.Cleanup(server.Close)

	client, err := NewClient(server.URL, "", WithTimeout(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.getText(context.Background(), "/slow", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %T %v", err, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client = newTestClient(t, server.URL, "")
	if _, err := client.getText(ctx, "/canceled", nil); !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "GET /canceled: ") {
		t.Fatalf("cancellation error = %T %v, want context.Canceled prefixed with the request", err, err)
	}
}

func TestClientOptionsAreOrderIndependent(t *testing.T) {
	t.Parallel()
	base := &http.Client{Timeout: time.Minute}
	tests := []struct {
		name    string
		options []Option
		want    time.Duration
	}{
		{"default", nil, defaultTimeout},
		{"timeout only", []Option{WithTimeout(time.Second)}, time.Second},
		{"http client keeps its timeout", []Option{WithHTTPClient(base)}, time.Minute},
		{"http client without timeout gets default", []Option{WithHTTPClient(&http.Client{})}, defaultTimeout},
		{"timeout before http client", []Option{WithTimeout(time.Second), WithHTTPClient(base)}, time.Second},
		{"timeout after http client", []Option{WithHTTPClient(base), WithTimeout(time.Second)}, time.Second},
		{"nil http client ignored", []Option{WithHTTPClient(nil), WithTimeout(time.Second)}, time.Second},
		{"non-positive timeout ignored", []Option{WithTimeout(-1)}, defaultTimeout},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client, err := NewClient("https://sat.example", "token", test.options...)
			if err != nil {
				t.Fatal(err)
			}
			if client.authHTTP.Timeout != test.want || client.publicHTTP.Timeout != test.want {
				t.Fatalf("timeouts = %v/%v, want %v", client.authHTTP.Timeout, client.publicHTTP.Timeout, test.want)
			}
			if base.Timeout != time.Minute || base.CheckRedirect != nil {
				t.Fatalf("WithHTTPClient mutated the caller's client: %#v", base)
			}
		})
	}
}

func TestWithHTTPClientTransportAndUserAgent(t *testing.T) {
	t.Parallel()
	var calls int
	var mu sync.Mutex
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if got := request.Header.Get("User-Agent"); got != "sat-cli/1.2.3" {
			t.Errorf("user agent = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("via transport")),
			Request:    request,
		}, nil
	})
	client, err := NewClient("https://sat.example", "token",
		WithHTTPClient(&http.Client{Transport: transport}), WithUserAgent("sat-cli/1.2.3"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := client.Dashboard(context.Background()); err != nil || got != "via transport" {
		t.Fatalf("Dashboard() = %q, %v", got, err)
	}
	if got, err := client.Weather(context.Background(), ""); err != nil || got != "via transport" {
		t.Fatalf("Weather() = %q, %v", got, err)
	}
	if calls != 2 {
		t.Fatalf("transport calls = %d, want 2", calls)
	}
}

func TestTransportErrorsNameTheRequest(t *testing.T) {
	t.Parallel()
	cause := errors.New("connection refused")
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause })
	client, err := NewClient("https://sat.example/prefix", "token", WithHTTPClient(&http.Client{Transport: transport}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Dashboard(context.Background())
	if !errors.Is(err, cause) || err.Error() != "GET /prefix/api/dash: connection refused" {
		t.Fatalf("Dashboard() error = %q, want the request and cause only", err)
	}
}

func TestTextResponsesAreBounded(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.Copy(writer, io.LimitReader(neverEnding('x'), maxTextBody+10))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	if got, err := client.Dashboard(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Dashboard() = %d bytes, %v; want size error", len(got), err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type neverEnding byte

func (b neverEnding) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

func TestSpotifyErrorError(t *testing.T) {
	t.Parallel()
	if got := (&SpotifyError{Message: "No active device"}).Error(); got != "No active device" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestSpotifyErrorOnSuccessfulResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "  Spotify device is unavailable  \n")
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	err := client.PlayTrack(context.Background(), "1")
	var spotifyError *SpotifyError
	if !errors.As(err, &spotifyError) || spotifyError.Message != "Spotify device is unavailable" {
		t.Fatalf("PlayTrack() error = %T %v", err, err)
	}
}

func TestAuthenticatedRequestsDoNotFollowRedirects(t *testing.T) {
	t.Parallel()
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		_, _ = io.WriteString(writer, "followed")
	}))
	t.Cleanup(target.Close)

	var endCalls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/same-start":
			http.Redirect(writer, request, "/same-end", http.StatusFound)
		case "/cross-start":
			http.Redirect(writer, request, target.URL+"/end", http.StatusFound)
		case "/post-start":
			http.Redirect(writer, request, "/same-end", http.StatusFound)
		case "/same-end":
			endCalls.Add(1)
			_, _ = io.WriteString(writer, "followed")
		}
	}))
	t.Cleanup(source.Close)

	client := newTestClient(t, source.URL, "redirect-token")
	for name, call := range map[string]func() error{
		"same host": func() error {
			_, err := client.getText(context.Background(), "/same-start", nil)
			return err
		},
		"cross host": func() error {
			_, err := client.getText(context.Background(), "/cross-start", nil)
			return err
		},
		"post": func() error {
			_, err := client.postForm(context.Background(), "/post-start", url.Values{"a": {"b"}})
			return err
		},
		"json": func() error {
			return client.sendJSON(context.Background(), http.MethodPut, "/same-start", map[string]string{"a": "b"}, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			var httpError *HTTPError
			if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusFound {
				t.Fatalf("error = %T %v, want HTTP 302 HTTPError", err, err)
			}
		})
	}
	if targetCalls.Load() != 0 || endCalls.Load() != 0 {
		t.Fatalf("redirect followed: target=%d same-host=%d", targetCalls.Load(), endCalls.Load())
	}
}

func TestAuthenticatedRequestsDoNotFollowHTTPSToHTTPDowngrade(t *testing.T) {
	t.Parallel()
	var leaked atomic.Value
	plain := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		leaked.Store(request.Header.Get("Authorization"))
		_, _ = io.WriteString(writer, "downgraded")
	}))
	t.Cleanup(plain.Close)
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, plain.URL+request.URL.Path, http.StatusMovedPermanently)
	}))
	t.Cleanup(secure.Close)

	client, err := NewClient(secure.URL, "downgrade-token", WithHTTPClient(secure.Client()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Dashboard(context.Background())
	var httpError *HTTPError
	if !errors.As(err, &httpError) || httpError.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("Dashboard() error = %T %v, want HTTP 301 HTTPError", err, err)
	}
	if value := leaked.Load(); value != nil {
		t.Fatalf("plain-HTTP server was reached with authorization %q", value)
	}
}

func TestUnauthenticatedRedirectLimit(t *testing.T) {
	t.Parallel()
	var hops atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hops.Add(1)
		http.Redirect(writer, request, fmt.Sprintf("/api/wt/hop%d", hops.Load()), http.StatusFound)
	}))
	t.Cleanup(server.Close)

	_, err := newTestClient(t, server.URL, "token").Weather(context.Background(), "loop")
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") || !strings.HasPrefix(err.Error(), "GET /api/wt/loop: ") {
		t.Fatalf("Weather() error = %v, want redirect limit error naming the request", err)
	}
	if got := hops.Load(); got != 10 {
		t.Fatalf("server saw %d requests, want 10", got)
	}
}

func TestUnauthenticatedRequestsFollowRedirects(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/wt/Vilnius":
			http.Redirect(writer, request, "/weather-end", http.StatusFound)
		case "/weather-end":
			_, _ = io.WriteString(writer, "sunny")
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	weather, err := client.Weather(context.Background(), "Vilnius")
	if err != nil || weather != "sunny" {
		t.Fatalf("Weather() = %q, %v", weather, err)
	}
}

func TestMalformedJSONResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `[{"id":`)
	}))
	t.Cleanup(server.Close)

	articles, err := newTestClient(t, server.URL, "token").ListArticles(context.Background(), false)
	if err == nil || !strings.HasPrefix(err.Error(), "GET /api/journals/articles: decode response: ") {
		t.Fatalf("ListArticles() = %#v, %v; want decode error naming the request", articles, err)
	}
}

func TestJSONWithoutOutputDiscardsBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertJSONBody(t, request, map[string]string{"a": "b"})
		// Not valid JSON: with out == nil the body must not be decoded.
		_, _ = io.WriteString(writer, "{ignored")
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	if err := client.sendJSON(context.Background(), http.MethodPut, "/thing", map[string]string{"a": "b"}, nil); err != nil {
		t.Fatalf("sendJSON(out = nil) = %v", err)
	}
}

func TestCreateArticleValidationError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(writer, `{"message":"The contents field is required.","errors":{"contents":["The contents field is required."]}}`)
	}))
	t.Cleanup(server.Close)

	article, err := newTestClient(t, server.URL, "token").CreateArticle(context.Background(), "")
	var httpError *HTTPError
	if !errors.As(err, &httpError) {
		t.Fatalf("CreateArticle() = %#v, %T %v; want *HTTPError", article, err, err)
	}
	if httpError.StatusCode != http.StatusUnprocessableEntity || httpError.Method != http.MethodPost ||
		httpError.Path != "/api/journals/articles" || httpError.Message != "The contents field is required." {
		t.Fatalf("HTTPError = %#v", httpError)
	}
	if want := "POST /api/journals/articles: HTTP 422 Unprocessable Entity: The contents field is required."; err.Error() != want {
		t.Fatalf("Error() = %q, want %q", err, want)
	}
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrForbidden) {
		t.Fatalf("422 matched a credential sentinel: %v", err)
	}
}

func TestHTTPErrorMessage(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		`{"message":"  padded  "}`: "padded",
		`{"message":""}`:           "",
		`{"error":"other"}`:        "",
		`["message"]`:              "",
		"plain text":               "",
		"":                         "",
	}
	for body, want := range tests {
		if got := jsonMessage(body); got != want {
			t.Errorf("jsonMessage(%q) = %q, want %q", body, got, want)
		}
	}
	plain := &HTTPError{StatusCode: http.StatusBadGateway, Method: http.MethodGet, Path: "/x", Body: "upstream down"}
	if got, want := plain.Error(), "GET /x: HTTP 502 Bad Gateway: upstream down"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestJSONEmptySuccessResponses(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/no-content":
			writer.WriteHeader(http.StatusNoContent)
		case "/empty":
			writer.WriteHeader(http.StatusOK)
		case "/trailing":
			_, _ = io.WriteString(writer, `{"result":"ok"}`+"\n\n")
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	for _, path := range []string{"/no-content", "/empty", "/trailing"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			var output struct {
				Result string `json:"result"`
			}
			if err := client.getJSON(context.Background(), path, nil, &output); err != nil {
				t.Fatalf("getJSON(%s) = %v", path, err)
			}
			if path == "/trailing" && output.Result != "ok" {
				t.Fatalf("decoded output = %#v", output)
			}
		})
	}
}

func TestHTTPErrorRedactsTokenStraddlingTruncation(t *testing.T) {
	t.Parallel()
	secret := "straddling-secret-token"
	tests := map[string]string{
		// The token starts just before the limit and ends past it.
		"straddles limit": strings.Repeat("x", maxErrorBody-5) + secret + strings.Repeat("y", 2000),
		// Earlier redactions shrink the body and pull a token that was only
		// partially read into the kept window.
		"partial after shrink": strings.Repeat(secret, 20) + strings.Repeat("z", maxErrorBody-20*len(secret)+2) + secret + strings.Repeat("y", 2000),
		// Multi-byte runes must not be split by the cut.
		"utf8 boundary": strings.Repeat("ž", 2000),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(writer, body)
			}))
			t.Cleanup(server.Close)

			client := newTestClient(t, server.URL, secret)
			_, err := client.getText(context.Background(), "/large", nil)
			var httpError *HTTPError
			if !errors.As(err, &httpError) {
				t.Fatalf("error type = %T", err)
			}
			if !httpError.Truncated || len(httpError.Body) > maxErrorBody {
				t.Fatalf("Truncated = %v, len(Body) = %d", httpError.Truncated, len(httpError.Body))
			}
			if !utf8.ValidString(httpError.Body) {
				t.Fatalf("body is not valid UTF-8: %q", httpError.Body[len(httpError.Body)-8:])
			}
			for size := 4; size <= len(secret); size++ {
				if strings.Contains(httpError.Body, secret[:size]) {
					t.Fatalf("body leaks token prefix %q", secret[:size])
				}
			}
		})
	}
}

func TestBaseURLPathPrefixAndJoinPath(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sat/prefix/api/dash" {
			t.Errorf("path = %q", request.URL.Path)
		}
		_, _ = io.WriteString(writer, "ok")
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL+"/sat/prefix/", "token")
	if _, err := client.Dashboard(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := joinPath("api", "resource", "an/id", "New York"); err != nil || got != "/api/resource/an%2Fid/New%20York" {
		t.Fatalf("joinPath() = %q, %v", got, err)
	}
	for _, segment := range []string{"", ".", ".."} {
		if got, err := joinPath("api", segment); err == nil {
			t.Fatalf("joinPath(%q) = %q, want error", segment, got)
		}
	}
}

func TestInvalidIDsAndActionsNeverReachTheServer(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Errorf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
		writer.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	ctx := context.Background()
	calls := map[string]func() error{
		"GetArticle zero":     func() error { _, err := client.GetArticle(ctx, 0); return err },
		"GetArticle negative": func() error { _, err := client.GetArticle(ctx, -1); return err },
		"UpdateArticleContents zero": func() error {
			_, err := client.UpdateArticleContents(ctx, 0, "x")
			return err
		},
		"AssignArticleJournal zero": func() error {
			_, err := client.AssignArticleJournal(ctx, 0, "Work")
			return err
		},
		"PlayTrack empty":       func() error { return client.PlayTrack(ctx, "") },
		"PlayTrack dot":         func() error { return client.PlayTrack(ctx, ".") },
		"PlayTrack dot dot":     func() error { return client.PlayTrack(ctx, "..") },
		"ControlPlayback bogus": func() error { return client.ControlPlayback(ctx, PlaybackAction("..")) },
		"Weather dot dot":       func() error { _, err := client.Weather(ctx, ".."); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := call(); err == nil {
				t.Fatal("call succeeded, want validation error")
			}
		})
	}
}

func TestGenericHelpersIncludeQueryAndDecodeJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("q") != "a & b" {
			t.Errorf("query = %q", request.URL.RawQuery)
		}
		writeJSON(t, writer, map[string]string{"result": "ok"})
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL, "token")
	var output struct {
		Result string `json:"result"`
	}
	if err := client.getJSON(context.Background(), "/search", url.Values{"q": {"a & b"}}, &output); err != nil {
		t.Fatal(err)
	}
	if output.Result != "ok" {
		t.Fatalf("decoded output = %#v", output)
	}
}

func newTestClient(t *testing.T, baseURL, token string) *Client {
	t.Helper()
	client, err := NewClient(baseURL, token)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Error(err)
	}
}

func assertJSONBody(t *testing.T, request *http.Request, want map[string]string) {
	t.Helper()
	if request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
		t.Errorf("JSON headers = %q, %q", request.Header.Get("Content-Type"), request.Header.Get("Accept"))
	}
	var got map[string]string
	if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
		// Called from the server's handler goroutine, where t.Fatal is not allowed.
		t.Errorf("decode JSON body: %v", err)
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("JSON body = %#v, want %#v", got, want)
	}
}
