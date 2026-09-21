package command

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoginPromptsForMissingURLAndToken(t *testing.T) {
	config := &memoryConfig{}
	stderr, err := executeLogin(config, "https://sat.example\nnew-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://sat.example" || config.token != "new-token" {
		t.Fatalf("config = URL %q, token %q", config.baseURL, config.token)
	}
	if !strings.Contains(stderr, "Base URL: ") || !strings.Contains(stderr, "Token: ") {
		t.Fatalf("prompts = %q", stderr)
	}
}

func TestLoginSkipsConfiguredURL(t *testing.T) {
	config := &memoryConfig{baseURL: "https://existing.example"}
	stderr, err := executeLogin(config, "new-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://existing.example" {
		t.Fatalf("base URL = %q", config.baseURL)
	}
	if strings.Contains(stderr, "Base URL: ") {
		t.Fatalf("unexpected URL prompt: %q", stderr)
	}
}

func TestLoginReplacesURLWithFlag(t *testing.T) {
	config := &memoryConfig{baseURL: "https://old.example"}
	_, err := executeLogin(config, "https://new.example\nnew-token\n", "--replace-url")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://new.example" {
		t.Fatalf("base URL = %q", config.baseURL)
	}
}

func TestLoginURLOnlyReplacesURLWithoutToken(t *testing.T) {
	config := &memoryConfig{baseURL: "https://old.example", token: "existing-token"}
	_, err := executeLogin(config, "https://new.example\n", "--url-only")
	if err != nil {
		t.Fatal(err)
	}
	if config.baseURL != "https://new.example" || config.token != "existing-token" {
		t.Fatalf("config = URL %q, token %q", config.baseURL, config.token)
	}
}

func TestLoginWarnsWhenReplacingToken(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example", token: "old-token"}
	stderr, err := executeLogin(config, "new-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, "Token is already defined; it will be replaced") {
		t.Fatalf("stderr = %q", stderr)
	}
	if config.token != "new-token" {
		t.Fatalf("token = %q", config.token)
	}
}

func TestLoginRejectsEmptyToken(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example"}
	if _, err := executeLogin(config, "\n"); err == nil {
		t.Fatal("expected an empty token error")
	}
}

func TestLoginRejectsInvalidURLNonInteractively(t *testing.T) {
	config := &memoryConfig{}
	if _, err := executeLogin(config, "not-a-url\ntoken\n"); err == nil {
		t.Fatal("expected an invalid URL error")
	}
	if config.baseURL != "" || config.token != "" {
		t.Fatalf("config changed to URL %q, token %q", config.baseURL, config.token)
	}
}

func TestLoginReadsNonInteractiveTokenFromStdin(t *testing.T) {
	config := &memoryConfig{baseURL: "https://sat.example"}
	stderr, err := executeLogin(config, "piped-token\n")
	if err != nil {
		t.Fatal(err)
	}
	if config.token != "piped-token" {
		t.Fatalf("token = %q", config.token)
	}
	if !strings.Contains(stderr, "Token saved.") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func executeLogin(config *memoryConfig, input string, args ...string) (string, error) {
	var stderr bytes.Buffer
	app := &App{
		Config:     config,
		Stdin:      strings.NewReader(input),
		Stdout:     &bytes.Buffer{},
		Stderr:     &stderr,
		IsTerminal: func(int) bool { return false },
	}
	command := newLoginCommand(app)
	command.SetArgs(args)
	err := command.Execute()
	return stderr.String(), err
}
