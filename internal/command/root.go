// Package command defines sat's Cobra command tree and injectable boundaries.
package command

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
	"github.com/spf13/cobra"
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
	ReadCacheLines(string) ([]string, bool, error)
	WriteCacheLines(string, []string) error
	RemoveCache(string) error
}

// APIClient contains the Satellite operations used by CLI commands.
type APIClient interface {
	ListArticles(context.Context, bool) ([]api.Article, error)
	GetArticle(context.Context, string) (api.ArticleContents, error)
	CreateArticle(context.Context, string) (api.Article, error)
	UpdateArticleContents(context.Context, string, string) (api.Article, error)
	AssignArticleJournal(context.Context, string, string) (api.Article, error)
	ListJournals(context.Context) ([]api.Journal, error)
	SavedTracks(context.Context) ([]string, error)
	PlayTrack(context.Context, string) error
	ControlPlayback(context.Context, string) error
	Dashboard(context.Context) (string, error)
	Weather(context.Context, string) (string, error)
	Notify(context.Context, string, string) error
}

// APIClientFactory constructs an API client from persisted configuration.
type APIClientFactory func(baseURL, token string) (APIClient, error)

// ProcessRunner is the external process boundary used by editor and SSH commands.
type ProcessRunner interface {
	Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
}

// ExecRunner runs child processes with the standard os/exec package.
type ExecRunner struct{}

// Run executes one child process attached to the provided streams.
func (ExecRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	process := exec.CommandContext(ctx, name, args...)
	process.Stdin = stdin
	process.Stdout = stdout
	process.Stderr = stderr
	return process.Run()
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

// NewDefaultApp wires App to the operating system and the real API client.
func NewDefaultApp() *App {
	getenv := os.Getenv
	return &App{
		Config: config.NewStore(config.StateDir(getenv), getenv),
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
	}
}

// NewRootCommand builds the sat command tree. Register future commands with
// root.AddCommand(newXCommand(app)); each constructor remains easy to test.
func NewRootCommand(app *App) *cobra.Command {
	if app == nil {
		app = NewDefaultApp()
	}
	normalizeApp(app)

	root := &cobra.Command{
		Use:           "sat",
		Short:         "Command-line client for Satellite",
		Version:       app.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.SetIn(app.Stdin)
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetVersionTemplate("sat {{.Version}}\n")

	for _, constructor := range subcommands {
		root.AddCommand(constructor(app))
	}

	return root
}

// subcommands lists every top-level command constructor. Each command file
// registers itself from an init function via registerCommand so that files
// can be added without editing this one.
var subcommands []func(*App) *cobra.Command

func registerCommand(constructor func(*App) *cobra.Command) {
	subcommands = append(subcommands, constructor)
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
