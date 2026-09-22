package command

import (
	"bytes"
	"strings"
	"testing"
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
		alias string
		want  string
	}{
		{"arl", "article"},
		{"dash", "dashboard"},
		{"wt", "weather"},
	}
	for _, tc := range cases {
		root := NewRootCommand(app)
		cmd, _, err := root.Find([]string{tc.alias})
		if err != nil {
			t.Fatalf("Find(%q) error: %v", tc.alias, err)
		}
		if got := cmd.Name(); got != tc.want {
			t.Fatalf("alias %q resolved to %q, want %q", tc.alias, got, tc.want)
		}
	}
}
