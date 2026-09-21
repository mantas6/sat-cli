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
