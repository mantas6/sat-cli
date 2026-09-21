package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// StateDir resolves sat's state directory with an injectable environment lookup.
func StateDir(getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	if state := strings.TrimSpace(getenv("SAT_JOURNAL_STATE")); state != "" {
		return state
	}
	if stateHome := strings.TrimSpace(getenv("XDG_STATE_HOME")); stateHome != "" {
		return filepath.Join(stateHome, "sat")
	}

	return filepath.Join(strings.TrimSpace(getenv("HOME")), ".local", "state", "sat")
}

func validateBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("invalid base URL %q: use an absolute http or https URL", value)
	}

	return value, nil
}
