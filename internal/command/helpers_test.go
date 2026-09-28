package command

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/sat-cli/internal/api"
	"github.com/mantas6/sat-cli/internal/config"
	"github.com/mantas6/sat-cli/internal/ui"
	"golang.org/x/term"
)

// fakeAPI implements APIClient with one optional func per method. A nil func
// returns zero values.
type fakeAPI struct {
	listArticles          func(ctx context.Context, all bool) ([]api.Article, error)
	getArticle            func(ctx context.Context, id int) (api.ArticleContents, error)
	createArticle         func(ctx context.Context, contents string) (api.Article, error)
	updateArticleContents func(ctx context.Context, id int, contents string) (api.Article, error)
	assignArticleJournal  func(ctx context.Context, id int, journal string) (api.Article, error)
	listJournals          func(ctx context.Context) ([]api.Journal, error)
	savedTracks           func(ctx context.Context) ([]string, error)
	playTrack             func(ctx context.Context, id string) error
	controlPlayback       func(ctx context.Context, action api.PlaybackAction) error
	dashboard             func(ctx context.Context) (string, error)
	weather               func(ctx context.Context, place string) (string, error)
	notify                func(ctx context.Context, message string) error
}

var _ APIClient = (*fakeAPI)(nil)

func (f *fakeAPI) ListArticles(ctx context.Context, all bool) ([]api.Article, error) {
	if f.listArticles == nil {
		return nil, nil
	}
	return f.listArticles(ctx, all)
}

func (f *fakeAPI) GetArticle(ctx context.Context, id int) (api.ArticleContents, error) {
	if f.getArticle == nil {
		return api.ArticleContents{}, nil
	}
	return f.getArticle(ctx, id)
}

func (f *fakeAPI) CreateArticle(ctx context.Context, contents string) (api.Article, error) {
	if f.createArticle == nil {
		return api.Article{}, nil
	}
	return f.createArticle(ctx, contents)
}

func (f *fakeAPI) UpdateArticleContents(ctx context.Context, id int, contents string) (api.Article, error) {
	if f.updateArticleContents == nil {
		return api.Article{}, nil
	}
	return f.updateArticleContents(ctx, id, contents)
}

func (f *fakeAPI) AssignArticleJournal(ctx context.Context, id int, journal string) (api.Article, error) {
	if f.assignArticleJournal == nil {
		return api.Article{}, nil
	}
	return f.assignArticleJournal(ctx, id, journal)
}

func (f *fakeAPI) ListJournals(ctx context.Context) ([]api.Journal, error) {
	if f.listJournals == nil {
		return nil, nil
	}
	return f.listJournals(ctx)
}

func (f *fakeAPI) SavedTracks(ctx context.Context) ([]string, error) {
	if f.savedTracks == nil {
		return nil, nil
	}
	return f.savedTracks(ctx)
}

func (f *fakeAPI) PlayTrack(ctx context.Context, id string) error {
	if f.playTrack == nil {
		return nil
	}
	return f.playTrack(ctx, id)
}

func (f *fakeAPI) ControlPlayback(ctx context.Context, action api.PlaybackAction) error {
	if f.controlPlayback == nil {
		return nil
	}
	return f.controlPlayback(ctx, action)
}

func (f *fakeAPI) Dashboard(ctx context.Context) (string, error) {
	if f.dashboard == nil {
		return "", nil
	}
	return f.dashboard(ctx)
}

func (f *fakeAPI) Weather(ctx context.Context, place string) (string, error) {
	if f.weather == nil {
		return "", nil
	}
	return f.weather(ctx, place)
}

func (f *fakeAPI) Notify(ctx context.Context, message string) error {
	if f.notify == nil {
		return nil
	}
	return f.notify(ctx, message)
}

// fakeConfig is an in-memory ConfigStore. The zero value is unconfigured;
// newFakeConfig returns one with a base URL, token and temporary directory.
type fakeConfig struct {
	dir       string
	urlPath   string
	tokenPath string
	tmpDir    string
	baseURL   string
	token     string
	// baseURLErr, when set, is returned by BaseURL.
	baseURLErr error
	caches     map[string][]string
}

var _ ConfigStore = (*fakeConfig)(nil)

func newFakeConfig(t *testing.T) *fakeConfig {
	t.Helper()
	dir := t.TempDir()
	return &fakeConfig{
		dir:       dir,
		urlPath:   filepath.Join(dir, "url"),
		tokenPath: filepath.Join(dir, "token"),
		tmpDir:    filepath.Join(dir, "tmp"),
		baseURL:   "https://satellite.test",
		token:     "secret",
	}
}

func (c *fakeConfig) BaseURL() (string, error) {
	if c.baseURLErr != nil {
		return "", c.baseURLErr
	}
	if c.baseURL == "" {
		return "", config.ErrBaseURLMissing
	}
	return c.baseURL, nil
}

func (c *fakeConfig) Token() (string, error) {
	if c.token == "" {
		return "", config.ErrTokenMissing
	}
	return c.token, nil
}

func (c *fakeConfig) SetBaseURL(value string) error {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("base URL must be an absolute http or https URL")
	}
	c.baseURL = value
	return nil
}

func (c *fakeConfig) SetToken(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return config.ErrTokenMissing
	}
	c.token = value
	return nil
}

func (c *fakeConfig) HasBaseURL() bool  { return c.baseURL != "" }
func (c *fakeConfig) HasToken() bool    { return c.token != "" }
func (c *fakeConfig) Dir() string       { return c.dir }
func (c *fakeConfig) URLPath() string   { return c.urlPath }
func (c *fakeConfig) TokenPath() string { return c.tokenPath }

func (c *fakeConfig) TmpDir() (string, error) {
	if c.tmpDir == "" {
		return "", errors.New("fakeConfig has no temporary directory")
	}
	return c.tmpDir, os.MkdirAll(c.tmpDir, 0o700)
}

func (c *fakeConfig) ReadCacheLines(name string) ([]string, bool, error) {
	lines, exists := c.caches[name]
	return slices.Clone(lines), exists, nil
}

func (c *fakeConfig) WriteCacheLines(name string, lines []string) error {
	if c.caches == nil {
		c.caches = make(map[string][]string)
	}
	c.caches[name] = slices.Clone(lines)
	return nil
}

func (c *fakeConfig) RemoveCache(name string) error {
	delete(c.caches, name)
	return nil
}

// fakeRunner records the last process it was asked to run. run, when set,
// decides the result; otherwise err is returned.
type fakeRunner struct {
	calls  int
	name   string
	args   []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	err    error
	run    func(name string, args []string) error
}

var _ ProcessRunner = (*fakeRunner)(nil)

func (r *fakeRunner) Run(_ context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	r.calls++
	r.name = name
	r.args = slices.Clone(args)
	r.stdin, r.stdout, r.stderr = stdin, stdout, stderr
	if r.run != nil {
		return r.run(name, args)
	}
	return r.err
}

type testOption func(*App)

// withAPI makes every API client the app builds return client.
func withAPI(client APIClient) testOption {
	return func(app *App) {
		app.NewAPIClient = func(string, string) (APIClient, error) { return client, nil }
	}
}

func withConfig(store ConfigStore) testOption {
	return func(app *App) { app.Config = store }
}

func withRunner(runner ProcessRunner) testOption {
	return func(app *App) { app.Runner = runner }
}

func withStdin(stdin io.Reader) testOption {
	return func(app *App) { app.Stdin = stdin }
}

// withEnv replaces the environment with env.
func withEnv(env map[string]string) testOption {
	return func(app *App) {
		app.Getenv = func(key string) string { return env[key] }
	}
}

// withTTY reports every stream as a terminal.
func withTTY() testOption {
	return func(app *App) { app.IsTTY = func(any) bool { return true } }
}

// newTestApp returns a hermetic App: a configured fakeConfig, an empty
// fakeAPI, no environment, buffered streams that are not terminals, and UI,
// terminal and process boundaries that fail the test when used unexpectedly.
func newTestApp(t *testing.T, opts ...testOption) (app *App, stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	unexpected := func(name string) error {
		t.Errorf("unexpected call to App.%s", name)
		return errors.New("unexpected call to App." + name)
	}
	app = &App{
		Config: newFakeConfig(t),
		Stdin:  strings.NewReader(""),
		Stdout: stdout,
		Stderr: stderr,
		Now:    time.Now,
		Getenv: func(string) string { return "" },
		Runner: runnerFunc(func(name string, _ []string) error {
			return unexpected("Runner.Run(" + name + ")")
		}),
		IsTTY:    func(any) bool { return false },
		TermSize: func() (int, int, bool) { return 0, 0, false },
		ReadPassword: func(int) ([]byte, error) {
			return nil, unexpected("ReadPassword")
		},
		TerminalState: func(int) (*term.State, error) {
			return nil, unexpected("TerminalState")
		},
		RestoreTerminal: func(int, *term.State) error {
			return unexpected("RestoreTerminal")
		},
		Executable: func() (string, error) { return "/usr/local/bin/sat", nil },
		Select: func(context.Context, io.Reader, io.Writer, []ui.Item, ui.SelectOptions) (ui.Item, error) {
			return ui.Item{}, unexpected("Select")
		},
		Page: func(context.Context, io.Reader, io.Writer, string, ui.PageOptions) error {
			return unexpected("Page")
		},
		Follow: func(context.Context, io.Reader, io.Writer, ui.Fetcher, ui.DashboardOptions) error {
			return unexpected("Follow")
		},
	}
	withAPI(&fakeAPI{})(app)
	for _, opt := range opts {
		opt(app)
	}
	normalizeApp(app)
	return app, stdout, stderr
}

type runnerFunc func(name string, args []string) error

func (f runnerFunc) Run(_ context.Context, name string, args []string, _ io.Reader, _, _ io.Writer) error {
	return f(name, args)
}

// run executes the sat command tree with args under the test's context.
func run(t *testing.T, app *App, args ...string) error {
	t.Helper()
	return runContext(t.Context(), app, args...)
}

// runContext executes the sat command tree with args under ctx.
func runContext(ctx context.Context, app *App, args ...string) error {
	root := NewRootCommand(app)
	// A nil slice would make cobra fall back to os.Args.
	root.SetArgs(append([]string{}, args...))
	return root.ExecuteContext(ctx)
}

// selectIndex returns a Select stub that records its arguments and picks the
// item at index.
func selectIndex(index int, gotItems *[]ui.Item, gotOpts *ui.SelectOptions) SelectFunc {
	return func(_ context.Context, _ io.Reader, _ io.Writer, items []ui.Item, opts ui.SelectOptions) (ui.Item, error) {
		if gotItems != nil {
			*gotItems = slices.Clone(items)
		}
		if gotOpts != nil {
			*gotOpts = opts
		}
		return items[index], nil
	}
}

// pipe returns both ends of an os.Pipe, closed when the test ends. The read
// end is a real *os.File that is not a terminal.
func pipe(t *testing.T) (reader, writer *os.File) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})
	return reader, writer
}
