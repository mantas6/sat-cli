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
	"testing"
	"time"
	"unicode/utf8"
)

func TestClientMethods(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("%s authorization = %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		if request.Header.Get("User-Agent") != "sat-cli" {
			t.Errorf("%s user agent = %q", request.URL.Path, request.Header.Get("User-Agent"))
		}
		mu.Lock()
		seen[request.Method+" "+request.URL.EscapedPath()] = true
		mu.Unlock()

		switch request.Method + " " + request.URL.EscapedPath() {
		case "GET /api/journals/articles":
			if request.URL.Query().Get("all") != "1" || request.Header.Get("Accept") != "application/json" {
				t.Errorf("article list query/accept = %q/%q", request.URL.RawQuery, request.Header.Get("Accept"))
			}
			writeJSON(t, writer, []Article{{ID: 1, Title: "First", WordCount: 10, CreatedAt: "today"}})
		case "GET /api/journals/articles/41":
			writeJSON(t, writer, ArticleContents{Contents: "# text"})
		case "POST /api/journals/articles":
			assertJSONBody(t, request, map[string]string{"contents": "new text"})
			writeJSON(t, writer, Article{ID: 2, WordCount: 2})
		case "PUT /api/journals/articles/42":
			assertJSONBody(t, request, map[string]string{"contents": "updated"})
			writeJSON(t, writer, Article{ID: 42})
		case "PUT /api/journals/articles/43":
			assertJSONBody(t, request, map[string]string{"journal": "Work"})
			writeJSON(t, writer, Article{ID: 43, Journal: &Journal{Title: "Work"}})
		case "GET /api/journals":
			writeJSON(t, writer, []Journal{{ID: 3, Title: "Work"}})
		case "GET /api/albums/saved":
			_, _ = io.WriteString(writer, `[{"line":"track-id\tartist\t/album\t/01.\ttitle"}]`)
		case "PUT /api/albums/play/track%2Fid", "PUT /api/albums/control/next":
			writer.WriteHeader(http.StatusNoContent)
		case "GET /api/dash":
			if got := request.Header.Get("Accept"); got != "application/json, text/plain;q=0.9" {
				t.Errorf("dashboard accept = %q", got)
			}
			_, _ = io.WriteString(writer, "dashboard\n")
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "test-token")
	articles, err := client.ListArticles(context.Background(), true)
	if err != nil || len(articles) != 1 || articles[0].Title != "First" {
		t.Fatalf("ListArticles() = %#v, %v", articles, err)
	}
	contents, err := client.GetArticle(context.Background(), 41)
	if err != nil || contents.Contents != "# text" {
		t.Fatalf("GetArticle() = %#v, %v", contents, err)
	}
	created, err := client.CreateArticle(context.Background(), "new text")
	if err != nil || created.ID != 2 {
		t.Fatalf("CreateArticle() = %#v, %v", created, err)
	}
	if _, err := client.UpdateArticleContents(context.Background(), 42, "updated"); err != nil {
		t.Fatal(err)
	}
	assigned, err := client.AssignArticleJournal(context.Background(), 43, "Work")
	if err != nil || assigned.Journal == nil || assigned.Journal.Title != "Work" {
		t.Fatalf("AssignArticleJournal() = %#v, %v", assigned, err)
	}
	journals, err := client.ListJournals(context.Background())
	if err != nil || len(journals) != 1 || journals[0].ID != 3 {
		t.Fatalf("ListJournals() = %#v, %v", journals, err)
	}
	tracks, err := client.SavedTracks(context.Background())
	if err != nil || !reflect.DeepEqual(tracks, []string{"track-id\tartist\t/album\t/01.\ttitle"}) {
		t.Fatalf("SavedTracks() = %#v, %v", tracks, err)
	}
	if err := client.PlayTrack(context.Background(), "track/id"); err != nil {
		t.Fatal(err)
	}
	if err := client.ControlPlayback(context.Background(), Next); err != nil {
		t.Fatal(err)
	}
	dashboard, err := client.Dashboard(context.Background())
	if err != nil || dashboard != "dashboard\n" {
		t.Fatalf("Dashboard() = %q, %v", dashboard, err)
	}

	for _, request := range []string{
		"GET /api/journals/articles",
		"GET /api/journals/articles/41",
		"POST /api/journals/articles",
		"PUT /api/journals/articles/42",
		"PUT /api/journals/articles/43",
		"GET /api/journals",
		"GET /api/albums/saved",
		"PUT /api/albums/play/track%2Fid",
		"PUT /api/albums/control/next",
		"GET /api/dash",
	} {
		if !seen[request] {
			t.Errorf("did not receive %s", request)
		}
	}
}

func TestNotifyEncodesForm(t *testing.T) {
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
	defer server.Close()

	client := newTestClient(t, server.URL, "token")
	if err := client.Notify(context.Background(), "a message & more"); err != nil {
		t.Fatal(err)
	}
}

func TestWeatherEscapesPlaceAndOmitsAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/api/wt/New%20York%2FUS" {
			t.Errorf("escaped path = %q", request.URL.EscapedPath())
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Errorf("public request has authorization %q", authorization)
		}
		_, _ = io.WriteString(writer, "sunny")
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "do-not-send")
	weather, err := client.Weather(context.Background(), "New York/US")
	if err != nil || weather != "sunny" {
		t.Fatalf("Weather() = %q, %v", weather, err)
	}
}

func TestWeatherWithoutPlaceOmitsSegmentAndAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.EscapedPath() != "/api/wt" {
			t.Errorf("escaped path = %q", request.URL.EscapedPath())
		}
		if authorization := request.Header.Get("Authorization"); authorization != "" {
			t.Errorf("public request has authorization %q", authorization)
		}
		_, _ = io.WriteString(writer, "sunny")
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "do-not-send")
	weather, err := client.Weather(context.Background(), "")
	if err != nil || weather != "sunny" {
		t.Fatalf("Weather() = %q, %v", weather, err)
	}
}

func TestHTTPErrorMappingAndTruncation(t *testing.T) {
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
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()

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
	defer server.Close()
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
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
			_, _ = io.WriteString(writer, "late")
		case <-request.Context().Done():
		}
	}))
	defer server.Close()

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
		})
	}
	if base.Timeout != time.Minute || base.CheckRedirect != nil {
		t.Fatalf("WithHTTPClient mutated the caller's client: %#v", base)
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
	defer server.Close()

	client := newTestClient(t, server.URL, "token")
	if got, err := client.Dashboard(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Dashboard() = %d bytes, %v; want size error", len(got), err)
	}
}

func TestJSONContentTypeOnlyWithBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Content-Type"); got != "" {
			t.Errorf("%s content type = %q, want none without a body", request.Method, got)
		}
		writeJSON(t, writer, []Journal{})
	}))
	defer server.Close()

	if _, err := newTestClient(t, server.URL, "token").ListJournals(context.Background()); err != nil {
		t.Fatal(err)
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

func TestSpotifyErrorOnSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "  Spotify device is unavailable  \n")
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "token")
	err := client.PlayTrack(context.Background(), "1")
	var spotifyError *SpotifyError
	if !errors.As(err, &spotifyError) || spotifyError.Message != "Spotify device is unavailable" {
		t.Fatalf("PlayTrack() error = %T %v", err, err)
	}
}

func TestAuthenticatedRequestsDoNotFollowRedirects(t *testing.T) {
	var targetCalls int
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		targetCalls++
		_, _ = io.WriteString(writer, "followed")
	}))
	defer target.Close()

	var endCalls int
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/same-start":
			http.Redirect(writer, request, "/same-end", http.StatusFound)
		case "/cross-start":
			http.Redirect(writer, request, target.URL+"/end", http.StatusFound)
		case "/post-start":
			http.Redirect(writer, request, "/same-end", http.StatusFound)
		case "/same-end":
			endCalls++
			_, _ = io.WriteString(writer, "followed")
		}
	}))
	defer source.Close()

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
	if targetCalls != 0 || endCalls != 0 {
		t.Fatalf("redirect followed: target=%d same-host=%d", targetCalls, endCalls)
	}
}

func TestUnauthenticatedRequestsFollowRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/wt/Vilnius":
			http.Redirect(writer, request, "/weather-end", http.StatusFound)
		case "/weather-end":
			_, _ = io.WriteString(writer, "sunny")
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "token")
	weather, err := client.Weather(context.Background(), "Vilnius")
	if err != nil || weather != "sunny" {
		t.Fatalf("Weather() = %q, %v", weather, err)
	}
}

func TestJSONEmptySuccessResponses(t *testing.T) {
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
	defer server.Close()

	client := newTestClient(t, server.URL, "token")
	for _, path := range []string{"/no-content", "/empty", "/trailing"} {
		t.Run(path, func(t *testing.T) {
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
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(writer, body)
			}))
			defer server.Close()

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
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sat/prefix/api/dash" {
			t.Errorf("path = %q", request.URL.Path)
		}
		_, _ = io.WriteString(writer, "ok")
	}))
	defer server.Close()

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
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		t.Errorf("unexpected request %s %s", request.Method, request.URL.EscapedPath())
		writer.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

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
			if err := call(); err == nil {
				t.Fatal("call succeeded, want validation error")
			}
		})
	}
}

func TestGenericHelpersIncludeQueryAndDecodeJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("q") != "a & b" {
			t.Errorf("query = %q", request.URL.RawQuery)
		}
		writeJSON(t, writer, map[string]string{"result": "ok"})
	}))
	defer server.Close()

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
