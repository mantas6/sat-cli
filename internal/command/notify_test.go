package command

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
)

type stubAPI struct {
	weather func(context.Context, string) (string, error)
	notify  func(context.Context, string) error
}

func (s *stubAPI) ListArticles(context.Context, bool) ([]api.Article, error) {
	return nil, nil
}

func (s *stubAPI) GetArticle(context.Context, string) (api.ArticleContents, error) {
	return api.ArticleContents{}, nil
}

func (s *stubAPI) CreateArticle(context.Context, string) (api.Article, error) {
	return api.Article{}, nil
}

func (s *stubAPI) UpdateArticleContents(context.Context, string, string) (api.Article, error) {
	return api.Article{}, nil
}

func (s *stubAPI) AssignArticleJournal(context.Context, string, string) (api.Article, error) {
	return api.Article{}, nil
}

func (s *stubAPI) ListJournals(context.Context) ([]api.Journal, error) {
	return nil, nil
}

func (s *stubAPI) SavedTracks(context.Context) ([]string, error) {
	return nil, nil
}

func (s *stubAPI) PlayTrack(context.Context, string) error {
	return nil
}

func (s *stubAPI) ControlPlayback(context.Context, string) error {
	return nil
}

func (s *stubAPI) Dashboard(context.Context) (string, error) {
	return "", nil
}

func (s *stubAPI) Weather(ctx context.Context, place string) (string, error) {
	if s.weather == nil {
		return "", nil
	}
	return s.weather(ctx, place)
}

func (s *stubAPI) Notify(ctx context.Context, message string) error {
	if s.notify == nil {
		return nil
	}
	return s.notify(ctx, message)
}

type stubConfig struct {
	baseURL  string
	token    string
	baseErr  error
	tokenErr error
}

func (s *stubConfig) BaseURL() (string, error)                      { return s.baseURL, s.baseErr }
func (s *stubConfig) Token() (string, error)                        { return s.token, s.tokenErr }
func (s *stubConfig) SetBaseURL(value string) error                 { s.baseURL = value; return nil }
func (s *stubConfig) SetToken(value string) error                   { s.token = value; return nil }
func (s *stubConfig) HasBaseURL() bool                              { return s.baseURL != "" }
func (s *stubConfig) HasToken() bool                                { return s.token != "" }
func (s *stubConfig) TmpDir() (string, error)                       { return "", nil }
func (s *stubConfig) Dir() string                                   { return "" }
func (s *stubConfig) ReadCacheLines(string) ([]string, bool, error) { return nil, false, nil }
func (s *stubConfig) WriteCacheLines(string, []string) error        { return nil }
func (s *stubConfig) RemoveCache(string) error                      { return nil }

func newWeatherNotifyTestApp(client APIClient) (*App, *bytes.Buffer) {
	output := &bytes.Buffer{}
	return &App{
		Config: &stubConfig{baseURL: "https://satellite.test", token: "secret"},
		NewAPIClient: func(string, string) (APIClient, error) {
			return client, nil
		},
		Stdout: output,
		Stderr: output,
	}, output
}

func executeWeatherNotifyTestCommand(app *App, args ...string) error {
	command := NewRootCommand(app)
	command.SetArgs(args)
	return command.Execute()
}

func TestNotifyForwardsMessage(t *testing.T) {
	var gotMessage string
	client := &stubAPI{notify: func(_ context.Context, message string) error {
		gotMessage = message
		return nil
	}}
	app, output := newWeatherNotifyTestApp(client)

	if err := executeWeatherNotifyTestCommand(app, "notify", "deploy complete"); err != nil {
		t.Fatal(err)
	}
	if gotMessage != "deploy complete" {
		t.Fatalf("Notify() = %q, want %q", gotMessage, "deploy complete")
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want no output", output.String())
	}
}

func TestNotifyRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing", args: []string{"notify"}},
		{name: "too many", args: []string{"notify", "one", "two"}},
		{name: "empty", args: []string{"notify", ""}},
		{name: "blank", args: []string{"notify", "  "}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, _ := newWeatherNotifyTestApp(&stubAPI{})
			if err := executeWeatherNotifyTestCommand(app, test.args...); err == nil {
				t.Fatal("Execute() error = nil, want nonzero result")
			}
		})
	}
}

func TestNotifyAPIErrorPropagates(t *testing.T) {
	wantErr := errors.New("notification failed")
	client := &stubAPI{notify: func(context.Context, string) error {
		return wantErr
	}}
	app, _ := newWeatherNotifyTestApp(client)

	err := executeWeatherNotifyTestCommand(app, "notify", "message")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
