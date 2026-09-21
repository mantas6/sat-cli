package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mantas6/sat-cli/internal/command"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := command.NewDefaultApp()
	app.Version = version
	root := command.NewRootCommand(app)

	if err := root.ExecuteContext(ctx); err != nil {
		exit(err)
	}
}

func exit(err error) {
	fmt.Fprintf(os.Stderr, "sat: %v\n", err)
	os.Exit(exitCode(err))
}

func exitCode(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}

	var pointer *command.ExitError
	if errors.As(err, &pointer) && pointer.Code != 0 {
		return pointer.Code
	}
	var value command.ExitError
	if errors.As(err, &value) && value.Code != 0 {
		return value.Code
	}

	return 1
}
