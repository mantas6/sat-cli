package command

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
	"golang.org/x/term"
)

// ConfigStore is the state and configuration boundary used by commands.
type ConfigStore interface {
	BaseURL() (string, error)
	Token() (string, error)
	SetBaseURL(string) error
	SetToken(string) error
	HasBaseURL() bool
	HasToken() bool
	TmpDir() (string, error)
	Dir() string
	URLPath() string
	TokenPath() string
	ReadCacheLines(string) ([]string, bool, error)
	WriteCacheLines(string, []string) error
	RemoveCache(string) error
}

// APIClient contains the Satellite operations used by CLI commands.
type APIClient interface {
	ListArticles(context.Context, bool) ([]api.Article, error)
	GetArticle(context.Context, int) (api.ArticleContents, error)
	CreateArticle(context.Context, string) (api.Article, error)
	UpdateArticleContents(context.Context, int, string) (api.Article, error)
	AssignArticleJournal(context.Context, int, string) (api.Article, error)
	ListJournals(context.Context) ([]api.Journal, error)
	SavedTracks(context.Context) ([]string, error)
	PlayTrack(context.Context, string) error
	ControlPlayback(context.Context, api.PlaybackAction) error
	Dashboard(context.Context) (string, error)
	Weather(context.Context, string) (string, error)
	Notify(context.Context, string) error
}

// APIClientFactory constructs an API client from persisted configuration.
type APIClientFactory func(baseURL, token string) (APIClient, error)

// ProcessRunner is the external process boundary used by editor and SSH commands.
type ProcessRunner interface {
	Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
}

// App contains the process boundaries shared by subcommands.
type App struct {
	Config       ConfigStore
	NewAPIClient APIClientFactory
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	IsTerminal   func(fd int) bool
	TerminalSize func(fd int) (width, height int, err error)
	Now          func() time.Time
	Getenv       func(string) string
	Runner       ProcessRunner
	Version      string
}

// NewDefaultApp wires App to the operating system and the real API client. It
// fails when the state directory cannot be resolved.
func NewDefaultApp() (*App, error) {
	getenv := os.Getenv
	stateDir, err := config.StateDir(getenv)
	if err != nil {
		return nil, err
	}
	return &App{
		Config: config.NewStore(stateDir, getenv),
		NewAPIClient: func(baseURL, token string) (APIClient, error) {
			return api.NewClient(baseURL, token)
		},
		Stdin:        os.Stdin,
		Stdout:       os.Stdout,
		Stderr:       os.Stderr,
		IsTerminal:   term.IsTerminal,
		TerminalSize: term.GetSize,
		Now:          time.Now,
		Getenv:       os.Getenv,
		Runner:       ExecRunner{},
		Version:      "dev",
	}, nil
}

func normalizeApp(app *App) {
	if app.Stdin == nil {
		app.Stdin = os.Stdin
	}
	if app.Stdout == nil {
		app.Stdout = os.Stdout
	}
	if app.Stderr == nil {
		app.Stderr = os.Stderr
	}
	if app.IsTerminal == nil {
		app.IsTerminal = term.IsTerminal
	}
	if app.TerminalSize == nil {
		app.TerminalSize = term.GetSize
	}
	if app.Now == nil {
		app.Now = time.Now
	}
	if app.Getenv == nil {
		app.Getenv = os.Getenv
	}
	if app.Runner == nil {
		app.Runner = ExecRunner{}
	}
	if app.Version == "" {
		app.Version = "dev"
	}
}
