package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/mantas6/sat-cli/internal/command"
	"github.com/mantas6/sat-cli/internal/ui"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := command.NewDefaultApp()
	if err != nil {
		exit(err)
	}
	app.Version = version
	root := command.NewRootCommand(app)

	if err := root.ExecuteContext(ctx); err != nil {
		exit(err)
	}
}

func exit(err error) {
	if !silent(err) {
		fmt.Fprintf(os.Stderr, "sat: %v\n", err)
	}
	os.Exit(exitCode(err))
}

// silent reports whether err only carries an exit code: the failure was
// already reported by a child process (editor, ssh), the user cancelled a
// picker, or there is no underlying error at all.
func silent(err error) bool {
	var exitErr command.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	if exitErr.Err == nil {
		return true
	}
	var processErr *exec.ExitError
	return errors.As(exitErr.Err, &processErr) || errors.Is(exitErr.Err, ui.ErrCancelled)
}

func exitCode(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}

	var exitErr command.ExitError
	if errors.As(err, &exitErr) && exitErr.Code != 0 {
		return exitErr.Code
	}

	return 1
}
