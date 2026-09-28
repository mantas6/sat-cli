package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mantas6/sat-cli/internal/api"
)

// StateDir resolves sat's state directory with an injectable environment
// lookup. Following the XDG Base Directory specification, a relative
// XDG_STATE_HOME is ignored. It fails when the fallback needs $HOME and $HOME
// is empty or relative, rather than resolving a path relative to the working
// directory.
func StateDir(getenv func(string) string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if state := strings.TrimSpace(getenv("SAT_JOURNAL_STATE")); state != "" {
		return state, nil
	}
	if stateHome := strings.TrimSpace(getenv("XDG_STATE_HOME")); stateHome != "" && filepath.IsAbs(stateHome) {
		return filepath.Join(stateHome, "sat"), nil
	}

	home := strings.TrimSpace(getenv("HOME"))
	if home == "" {
		return "", errors.New("cannot resolve state directory: HOME is not set; set SAT_JOURNAL_STATE or XDG_STATE_HOME")
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("cannot resolve state directory: HOME %q is not an absolute path; set SAT_JOURNAL_STATE or XDG_STATE_HOME", home)
	}

	return filepath.Join(home, ".local", "state", "sat"), nil
}

// validateBaseURL checks value with api.ParseBaseURL, the same validator the
// client uses, and returns it trimmed.
func validateBaseURL(value string) (string, error) {
	if _, err := api.ParseBaseURL(value); err != nil {
		return "", err
	}

	return strings.TrimSpace(value), nil
}
