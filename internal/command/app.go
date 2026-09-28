package command

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
	"github.com/mantas6/sat-cli/internal/ui"
	"golang.org/x/term"
)

// defaultTermGrace is how long ExecRunner waits after a context cancellation
// (which sends SIGTERM) before hard-killing the child.
const defaultTermGrace = 2 * time.Second

// ConfigStore is the state and configuration boundary used by commands.
type ConfigStore interface {
	BaseURL() (string, error)
	Token() (string, error)
	SetBaseURL(value string) error
	SetToken(value string) error
	HasBaseURL() bool
	HasToken() bool
	TmpDir() (string, error)
	Dir() string
	URLPath() string
	TokenPath() string
	ReadCacheLines(name string) (lines []string, exists bool, err error)
	WriteCacheLines(name string, lines []string) error
	RemoveCache(name string) error
}

// APIClient contains the Satellite operations used by CLI commands.
type APIClient interface {
	ListArticles(ctx context.Context, all bool) ([]api.Article, error)
	GetArticle(ctx context.Context, id int) (api.ArticleContents, error)
	CreateArticle(ctx context.Context, contents string) (api.Article, error)
	UpdateArticleContents(ctx context.Context, id int, contents string) (api.Article, error)
	AssignArticleJournal(ctx context.Context, id int, journal string) (api.Article, error)
	ListJournals(ctx context.Context) ([]api.Journal, error)
	SavedTracks(ctx context.Context) ([]string, error)
	PlayTrack(ctx context.Context, id string) error
	ControlPlayback(ctx context.Context, action api.PlaybackAction) error
	Dashboard(ctx context.Context) (string, error)
	Weather(ctx context.Context, place string) (string, error)
	Notify(ctx context.Context, message string) error
}

// APIClientFactory constructs an API client from persisted configuration.
type APIClientFactory func(baseURL, token string) (APIClient, error)

// ProcessRunner is the external process boundary used by editor and SSH commands.
type ProcessRunner interface {
	Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

// SelectFunc runs the interactive item selector.
type SelectFunc func(ctx context.Context, in io.Reader, out io.Writer, items []ui.Item, opts ui.SelectOptions) (ui.Item, error)

// PageFunc shows Markdown content in the full-screen pager.
type PageFunc func(ctx context.Context, in io.Reader, out io.Writer, content string, opts ui.PageOptions) error

// FollowFunc refreshes fetched content in the full-screen dashboard view.
type FollowFunc func(ctx context.Context, in io.Reader, out io.Writer, fetch ui.Fetcher, opts ui.DashboardOptions) error

// App contains the process, terminal and UI boundaries shared by subcommands.
// NewRootCommand fills every nil field with its operating-system default, so
// tests only set the boundaries they exercise.
type App struct {
	Config       ConfigStore
	NewAPIClient APIClientFactory
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	Now          func() time.Time
	Getenv       func(key string) string
	Runner       ProcessRunner
	Version      string

	// IsTTY reports whether stream (one of Stdin, Stdout, Stderr) is a
	// terminal.
	IsTTY func(stream any) bool
	// TermSize reports the size of the terminal on Stdout; ok is false when
	// it is unknown.
	TermSize func() (width, height int, ok bool)
	// ReadPassword reads a line from the terminal fd without echo.
	ReadPassword func(fd int) ([]byte, error)
	// TerminalState captures the terminal mode of fd.
	TerminalState func(fd int) (*term.State, error)
	// RestoreTerminal restores a mode captured by TerminalState.
	RestoreTerminal func(fd int, state *term.State) error
	// Executable returns the path of the running sat binary.
	Executable func() (string, error)
	// TermGrace is how long the default Runner waits after SIGTERM before
	// killing a child whose context was cancelled.
	TermGrace time.Duration

	Select SelectFunc
	Page   PageFunc
	Follow FollowFunc
}

// NewDefaultApp wires App to the operating system and the real API client. It
// fails when the state directory cannot be resolved.
func NewDefaultApp() (*App, error) {
	app := &App{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Getenv: os.Getenv,
	}
	stateDir, err := config.StateDir(app.Getenv)
	if err != nil {
		return nil, err
	}
	app.Config = config.NewStore(stateDir, app.Getenv)
	normalizeApp(app)
	return app, nil
}

// normalizeApp fills every nil boundary with its operating-system default.
// Config is left alone because resolving the state directory can fail; see
// NewDefaultApp.
func normalizeApp(app *App) {
	if app.NewAPIClient == nil {
		// Version is read when the client is built, after main has set it.
		app.NewAPIClient = func(baseURL, token string) (APIClient, error) {
			return api.NewClient(baseURL, token, api.WithUserAgent("sat-cli/"+app.Version))
		}
	}
	if app.Stdin == nil {
		app.Stdin = os.Stdin
	}
	if app.Stdout == nil {
		app.Stdout = os.Stdout
	}
	if app.Stderr == nil {
		app.Stderr = os.Stderr
	}
	if app.Now == nil {
		app.Now = time.Now
	}
	if app.Getenv == nil {
		app.Getenv = os.Getenv
	}
	if app.TermGrace <= 0 {
		app.TermGrace = defaultTermGrace
	}
	if app.Runner == nil {
		app.Runner = ExecRunner{Grace: app.TermGrace}
	}
	if app.Version == "" {
		app.Version = "dev"
	}
	if app.IsTTY == nil {
		app.IsTTY = isTerminal
	}
	if app.TermSize == nil {
		app.TermSize = func() (int, int, bool) { return terminalSize(app.Stdout) }
	}
	if app.ReadPassword == nil {
		app.ReadPassword = term.ReadPassword
	}
	if app.TerminalState == nil {
		app.TerminalState = term.GetState
	}
	if app.RestoreTerminal == nil {
		app.RestoreTerminal = term.Restore
	}
	if app.Executable == nil {
		app.Executable = os.Executable
	}
	if app.Select == nil {
		app.Select = ui.Select
	}
	if app.Page == nil {
		app.Page = ui.Page
	}
	if app.Follow == nil {
		app.Follow = ui.Follow
	}
}

// isTerminal reports whether stream is an *os.File attached to a terminal.
func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// terminalSize reports the size of the terminal behind stream, if any.
func terminalSize(stream any) (width, height int, ok bool) {
	file, isFile := stream.(*os.File)
	if !isFile {
		return 0, 0, false
	}
	width, height, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 0, 0, false
	}
	return width, height, true
}

// interactive reports whether both Stdin and Stdout are terminals.
func (a *App) interactive() bool {
	return a.IsTTY(a.Stdin) && a.IsTTY(a.Stdout)
}
