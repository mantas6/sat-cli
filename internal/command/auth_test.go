package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/config"
)

func executeAuthStatus(app *App) (string, error) {
	var out bytes.Buffer
	command := newAuthCommand(app)
	command.SetArgs(nil)
	command.SetOut(&out)
	err := command.Execute()
	return out.String(), err
}

func TestAuthStatusConfigured(t *testing.T) {
	cfg := &memoryConfig{
		dir:       "/state/sat",
		baseURL:   "https://sat.example",
		token:     "super-secret-token",
		urlPath:   "/state/sat/url",
		tokenPath: "/state/sat/token",
	}
	app := &App{Config: cfg, Getenv: func(string) string { return "" }}

	out, err := executeAuthStatus(app)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"State directory: /state/sat",
		"Base URL:        https://sat.example",
		"URL file:        /state/sat/url",
		"Token:           configured",
		"Token file:      /state/sat/token",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\n%s", want, out)
		}
	}

	if strings.Contains(out, "super-secret-token") {
		t.Fatalf("token value leaked into output:\n%s", out)
	}
	if strings.Contains(out, "(from SAT_") {
		t.Fatalf("unexpected env override suffix:\n%s", out)
	}
}

func TestAuthStatusUnconfigured(t *testing.T) {
	cfg := &memoryConfig{
		dir:       "/state/sat",
		urlPath:   "/state/sat/url",
		tokenPath: "/state/sat/token",
	}
	app := &App{Config: cfg, Getenv: func(string) string { return "" }}

	out, err := executeAuthStatus(app)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Base URL:        not configured") {
		t.Fatalf("expected unconfigured base URL:\n%s", out)
	}
	if !strings.Contains(out, "Token:           not configured") {
		t.Fatalf("expected unconfigured token:\n%s", out)
	}
}

func TestAuthStatusEnvOverride(t *testing.T) {
	dir := t.TempDir()
	urlFile := filepath.Join(dir, "custom-url")
	tokenFile := filepath.Join(dir, "custom-token")
	if err := os.WriteFile(urlFile, []byte("https://override.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SAT_URL_PATH", urlFile)
	t.Setenv("SAT_TOKEN_PATH", tokenFile)

	store := config.NewStore(filepath.Join(dir, "state"), os.Getenv)
	app := &App{Config: store, Getenv: os.Getenv}

	out, err := executeAuthStatus(app)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, urlFile+"  (from SAT_URL_PATH)") {
		t.Fatalf("expected SAT_URL_PATH override annotation:\n%s", out)
	}
	if !strings.Contains(out, tokenFile+"  (from SAT_TOKEN_PATH)") {
		t.Fatalf("expected SAT_TOKEN_PATH override annotation:\n%s", out)
	}
	if !strings.Contains(out, "Base URL:        https://override.example") {
		t.Fatalf("expected overridden base URL:\n%s", out)
	}
	if !strings.Contains(out, "Token:           configured") {
		t.Fatalf("expected configured token:\n%s", out)
	}
	if strings.Contains(out, "secret") && !strings.Contains(out, "custom-token") {
		t.Fatalf("token value leaked into output:\n%s", out)
	}
}

func TestAuthStatusInvalidURL(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "url"), []byte("not-a-url\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := config.NewStore(dir, func(string) string { return "" })
	app := &App{Config: store, Getenv: func(string) string { return "" }}

	out, err := executeAuthStatus(app)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out, "Base URL:        invalid (") {
		t.Fatalf("expected invalid base URL line:\n%s", out)
	}
}

func TestLoginMovedUnderAuth(t *testing.T) {
	app := &App{}
	root := NewRootCommand(app)

	if cmd, _, err := root.Find([]string{"login"}); err == nil && cmd.Name() == "login" {
		t.Fatalf("top-level login command still resolves to %q", cmd.Name())
	}

	cmd, _, err := root.Find([]string{"auth", "login"})
	if err != nil {
		t.Fatalf("Find(auth login) error: %v", err)
	}
	if cmd.Name() != "login" {
		t.Fatalf("auth login resolved to %q, want login", cmd.Name())
	}
}
