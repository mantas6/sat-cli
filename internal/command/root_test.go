package command

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/api"
)

func TestRootHelpAndVersion(t *testing.T) {
	var output bytes.Buffer
	app := &App{Stdout: &output, Stderr: &output, Version: "1.2.3"}
	root := NewRootCommand(app)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Command-line client for Satellite") {
		t.Fatalf("help output = %q", got)
	}
	help := output.String()
	if !strings.Contains(help, "Environment:") {
		t.Fatalf("help output missing Environment block: %q", help)
	}
	for _, name := range []string{
		"SAT_JOURNAL_STATE",
		"XDG_STATE_HOME",
		"SAT_URL_PATH",
		"SAT_TOKEN_PATH",
		"REMOTE_HOST",
		"REMOTE_USER",
		"REMOTE_ROOT",
	} {
		if !strings.Contains(help, name) {
			t.Fatalf("help output missing %s: %q", name, help)
		}
	}

	output.Reset()
	root = NewRootCommand(app)
	root.SetArgs([]string{"weather", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); strings.Contains(got, "Environment:") {
		t.Fatalf("weather help should not contain Environment block: %q", got)
	}

	output.Reset()
	root = NewRootCommand(app)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "sat 1.2.3\n" {
		t.Fatalf("version output = %q", got)
	}
}

func TestRootCommandAliases(t *testing.T) {
	app := &App{}
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
		root := NewRootCommand(app)
		cmd, _, err := root.Find(tc.args)
		if err != nil {
			t.Fatalf("Find(%q) error: %v", tc.args, err)
		}
		if got := cmd.Name(); got != tc.want {
			t.Fatalf("alias %q resolved to %q, want %q", tc.args, got, tc.want)
		}
	}

	root := NewRootCommand(app)
	arl, _, err := root.Find([]string{"arl"})
	if err != nil || !arl.Hidden || arl.Flags().Lookup("id") == nil || arl.Flags().Lookup("raw") == nil {
		t.Fatalf("arl shim = hidden %v, err %v; want hidden read command with --id and --raw", arl.Hidden, err)
	}
}

func TestArlRunsArticleRead(t *testing.T) {
	var gotID int
	client := &articleAPI{stubAPI: &stubAPI{}, get: func(_ context.Context, id int) (api.ArticleContents, error) {
		gotID = id
		return api.ArticleContents{Contents: "contents"}, nil
	}}
	app, stdout := newArticleTestApp(client, nil)
	if err := executeArticleTestCommand(app, "arl", "--id", "5", "--raw"); err != nil {
		t.Fatal(err)
	}
	if gotID != 5 || stdout.String() != "contents\n" {
		t.Fatalf("GetArticle() ID = %d, stdout = %q", gotID, stdout.String())
	}
}

func TestGroupCommandsRejectUnknownSubcommands(t *testing.T) {
	for _, group := range []string{"article", "music"} {
		t.Run(group, func(t *testing.T) {
			var output bytes.Buffer
			app := &App{Stdout: &output, Stderr: &output}

			root := NewRootCommand(app)
			root.SetArgs([]string{group, "typo"})
			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), `unknown command "typo" for "sat `+group+`"`) {
				t.Fatalf("Execute(%s typo) error = %v, want unknown command", group, err)
			}

			output.Reset()
			root = NewRootCommand(app)
			root.SetArgs([]string{group})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute(%s) error = %v, want help", group, err)
			}
			if !strings.Contains(output.String(), "Available Commands:") {
				t.Fatalf("Execute(%s) output = %q, want help", group, output.String())
			}
		})
	}
}

func TestRootWithoutArgsPrintsHelpAndRejectsUnknownCommands(t *testing.T) {
	var output bytes.Buffer
	app := &App{Stdout: &output, Stderr: &output}
	root := NewRootCommand(app)
	root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Available Commands:") {
		t.Fatalf("root output = %q, want help", output.String())
	}

	root = NewRootCommand(app)
	root.SetArgs([]string{"typo"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), `unknown command "typo" for "sat"`) {
		t.Fatalf("Execute(typo) error = %v, want unknown command", err)
	}
}
