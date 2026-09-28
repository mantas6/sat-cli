package command

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
)

func TestRootHelpAndVersion(t *testing.T) {
	t.Parallel()

	t.Run("help lists the environment", func(t *testing.T) {
		t.Parallel()
		app, stdout, _ := newTestApp(t)
		if err := run(t, app, "--help"); err != nil {
			t.Fatal(err)
		}
		help := stdout.String()
		for _, want := range []string{
			"Command-line client for Satellite",
			"Environment:",
			"SAT_JOURNAL_STATE",
			"XDG_STATE_HOME",
			"SAT_URL_PATH",
			"SAT_TOKEN_PATH",
			"REMOTE_HOST",
			"REMOTE_USER",
			"REMOTE_ROOT",
		} {
			if !strings.Contains(help, want) {
				t.Fatalf("help output missing %q: %q", want, help)
			}
		}
	})

	t.Run("subcommand help omits the environment", func(t *testing.T) {
		t.Parallel()
		app, stdout, _ := newTestApp(t)
		if err := run(t, app, "weather", "--help"); err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); !strings.Contains(got, "weather [place]") || strings.Contains(got, "Environment:") {
			t.Fatalf("weather help = %q, want usage without Environment block", got)
		}
	})

	t.Run("version", func(t *testing.T) {
		t.Parallel()
		app, stdout, _ := newTestApp(t)
		app.Version = "1.2.3"
		if err := run(t, app, "--version"); err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); got != "sat 1.2.3\n" {
			t.Fatalf("version output = %q", got)
		}
	})

	t.Run("default version", func(t *testing.T) {
		t.Parallel()
		app, stdout, _ := newTestApp(t)
		if err := run(t, app, "--version"); err != nil {
			t.Fatal(err)
		}
		if got := stdout.String(); got != "sat dev\n" {
			t.Fatalf("version output = %q", got)
		}
	})
}

func TestRootCommandAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"arl"}, "arl"},
		{[]string{"article", "arl"}, "read"},
		{[]string{"dash"}, "dashboard"},
		{[]string{"wt"}, "weather"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			t.Parallel()
			app, _, _ := newTestApp(t)
			cmd, _, err := NewRootCommand(app).Find(tc.args)
			if err != nil {
				t.Fatalf("Find(%q) error: %v", tc.args, err)
			}
			if got := cmd.Name(); got != tc.want {
				t.Fatalf("alias %q resolved to %q, want %q", tc.args, got, tc.want)
			}
		})
	}

	t.Run("arl shim", func(t *testing.T) {
		t.Parallel()
		app, _, _ := newTestApp(t)
		arl, _, err := NewRootCommand(app).Find([]string{"arl"})
		if err != nil || !arl.Hidden || arl.Flags().Lookup("id") == nil || arl.Flags().Lookup("raw") == nil {
			t.Fatalf("arl shim = hidden %v, err %v; want hidden read command with --id and --raw", arl.Hidden, err)
		}
	})
}

func TestArlRunsArticleRead(t *testing.T) {
	t.Parallel()
	var gotID int
	client := &fakeAPI{getArticle: func(_ context.Context, id int) (api.ArticleContents, error) {
		gotID = id
		return api.ArticleContents{Contents: "contents"}, nil
	}}
	app, stdout, _ := newTestApp(t, withAPI(client))
	if err := run(t, app, "arl", "--id", "5", "--raw"); err != nil {
		t.Fatal(err)
	}
	if gotID != 5 || stdout.String() != "contents\n" {
		t.Fatalf("GetArticle() ID = %d, stdout = %q", gotID, stdout.String())
	}
}

func TestGroupCommandsRejectUnknownSubcommands(t *testing.T) {
	t.Parallel()
	for _, group := range []string{"article", "music"} {
		t.Run(group, func(t *testing.T) {
			t.Parallel()
			app, _, _ := newTestApp(t)
			err := run(t, app, group, "typo")
			if err == nil || !strings.Contains(err.Error(), `unknown command "typo" for "sat `+group+`"`) {
				t.Fatalf("Execute(%s typo) error = %v, want unknown command", group, err)
			}

			app, stdout, _ := newTestApp(t)
			if err := run(t, app, group); err != nil {
				t.Fatalf("Execute(%s) error = %v, want help", group, err)
			}
			if !strings.Contains(stdout.String(), "Available Commands:") {
				t.Fatalf("Execute(%s) output = %q, want help", group, stdout.String())
			}
		})
	}
}

func TestRootWithoutArgsPrintsHelpAndRejectsUnknownCommands(t *testing.T) {
	t.Parallel()
	app, stdout, _ := newTestApp(t)
	if err := run(t, app); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "Available Commands:") {
		t.Fatalf("root output = %q, want help", stdout.String())
	}

	app, _, _ = newTestApp(t)
	if err := run(t, app, "typo"); err == nil || !strings.Contains(err.Error(), `unknown command "typo" for "sat"`) {
		t.Fatalf("Execute(typo) error = %v, want unknown command", err)
	}
}

func TestNormalizeAppFillsEveryBoundary(t *testing.T) {
	t.Parallel()
	app := &App{}
	normalizeApp(app)
	if app.NewAPIClient == nil || app.Stdin == nil || app.Stdout == nil || app.Stderr == nil ||
		app.Now == nil || app.Getenv == nil || app.Runner == nil || app.IsTTY == nil ||
		app.TermSize == nil || app.ReadPassword == nil || app.TerminalState == nil ||
		app.RestoreTerminal == nil || app.Executable == nil || app.Select == nil ||
		app.Page == nil || app.Follow == nil {
		t.Fatalf("normalizeApp() left a nil boundary: %#v", app)
	}
	if app.Version != "dev" || app.TermGrace != defaultTermGrace {
		t.Fatalf("normalizeApp() Version = %q, TermGrace = %v", app.Version, app.TermGrace)
	}
	if runner, ok := app.Runner.(ExecRunner); !ok || runner.Grace != defaultTermGrace {
		t.Fatalf("normalizeApp() Runner = %#v, want ExecRunner with the default grace", app.Runner)
	}
}

func TestDefaultAPIClientSendsVersionedUserAgent(t *testing.T) {
	t.Parallel()
	agents := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		agents <- request.Header.Get("User-Agent")
		_, _ = io.WriteString(writer, "sunny")
	}))
	defer server.Close()

	app := &App{}
	normalizeApp(app)
	// main sets Version after NewDefaultApp has normalized the App.
	app.Version = "1.2.3"
	client, err := app.NewAPIClient(server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Weather(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	if got := <-agents; got != "sat-cli/1.2.3" {
		t.Fatalf("User-Agent = %q, want sat-cli/1.2.3", got)
	}
}

func TestDefaultTerminalDetectionRejectsNonTerminals(t *testing.T) {
	t.Parallel()
	reader, writer := pipe(t)
	for name, stream := range map[string]any{
		"buffer":     &strings.Builder{},
		"pipe read":  reader,
		"pipe write": writer,
		"nil":        nil,
	} {
		if isTerminal(stream) {
			t.Errorf("isTerminal(%s) = true", name)
		}
		if _, _, ok := terminalSize(stream); ok {
			t.Errorf("terminalSize(%s) ok = true", name)
		}
	}
}
