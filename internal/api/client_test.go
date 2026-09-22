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
		case "GET /api/journals/articles/id%2Fwith%20space":
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
	contents, err := client.GetArticle(context.Background(), "id/with space")
	if err != nil || contents.Contents != "# text" {
		t.Fatalf("GetArticle() = %#v, %v", contents, err)
	}
	created, err := client.CreateArticle(context.Background(), "new text")
	if err != nil || created.ID != 2 {
		t.Fatalf("CreateArticle() = %#v, %v", created, err)
	}
	if _, err := client.UpdateArticleContents(context.Background(), "42", "updated"); err != nil {
		t.Fatal(err)
	}
	assigned, err := client.AssignArticleJournal(context.Background(), "43", "Work")
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
	if err := client.ControlPlayback(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	dashboard, err := client.Dashboard(context.Background())
	if err != nil || dashboard != "dashboard\n" {
		t.Fatalf("Dashboard() = %q, %v", dashboard, err)
	}

	for _, request := range []string{
		"GET /api/journals/articles",
		"GET /api/journals/articles/id%2Fwith%20space",
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

func TestHTTPErrorMappingAndTruncation(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusUnauthorized, `{"message":"bad credentials"}`, "token is invalid or expired; run `sat auth login`"},
		{http.StatusForbidden, `{"message":"ability denied"}`, "token lacks the required ability"},
		{http.StatusUnprocessableEntity, `{"message":"The contents field is required."}`, "The contents field is required."},
		{http.StatusInternalServerError, "server exploded", "server exploded"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, "token")
			_, err := client.GetText(context.Background(), "/failure", nil)
			var httpError *HTTPError
			if !errors.As(err, &httpError) {
				t.Fatalf("error type = %T (%v)", err, err)
			}
			if httpError.Status != test.status || httpError.Method != http.MethodGet || httpError.Path != "/failure" {
				t.Fatalf("HTTPError = %#v", httpError)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want %q", err, test.want)
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
	_, err := client.GetText(context.Background(), "/large", nil)
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
	if _, err := client.GetText(context.Background(), "/slow", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error = %T %v", err, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client = newTestClient(t, server.URL, "")
	if _, err := client.GetText(ctx, "/canceled", nil); err != context.Canceled {
		t.Fatalf("cancellation error = %T %v", err, err)
	}
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

func TestRedirectAuthorizationHandling(t *testing.T) {
	var crossHostAuthorization string
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		crossHostAuthorization = request.Header.Get("Authorization")
		_, _ = io.WriteString(writer, "cross-host")
	}))
	defer target.Close()

	var sourceAuthorization, sameHostAuthorization string
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/same-start":
			sourceAuthorization = request.Header.Get("Authorization")
			http.Redirect(writer, request, "/same-end", http.StatusFound)
		case "/same-end":
			sameHostAuthorization = request.Header.Get("Authorization")
			_, _ = io.WriteString(writer, "same-host")
		case "/cross-start":
			sourceAuthorization = request.Header.Get("Authorization")
			http.Redirect(writer, request, target.URL+"/end", http.StatusFound)
		}
	}))
	defer source.Close()

	client := newTestClient(t, source.URL, "redirect-token")
	if body, err := client.GetText(context.Background(), "/same-start", nil); err != nil || string(body) != "same-host" {
		t.Fatalf("same-host redirect = %q, %v", body, err)
	}
	if sourceAuthorization != "Bearer redirect-token" || sameHostAuthorization != "Bearer redirect-token" {
		t.Fatalf("same-host authorization = %q / %q", sourceAuthorization, sameHostAuthorization)
	}
	if body, err := client.GetText(context.Background(), "/cross-start", nil); err != nil || string(body) != "cross-host" {
		t.Fatalf("cross-host redirect = %q, %v", body, err)
	}
	if crossHostAuthorization != "" {
		t.Fatalf("authorization forwarded to another host: %q", crossHostAuthorization)
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
	if got := JoinPath("api", "resource", "an/id", "New York"); got != "/api/resource/an%2Fid/New%20York" {
		t.Fatalf("JoinPath() = %q", got)
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
	if err := client.GetJSON(context.Background(), "/search", url.Values{"q": {"a & b"}}, &output); err != nil {
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
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("JSON body = %#v, want %#v", got, want)
	}
}
