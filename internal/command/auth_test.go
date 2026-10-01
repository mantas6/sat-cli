package command

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mantas6/sat-cli/internal/config"
)

func TestAuthStatusConfigured(t *testing.T) {
	t.Parallel()
	cfg := &fakeConfig{
		dir:       "/state/sat",
		baseURL:   "https://sat.example",
		token:     "super-secret-token",
		urlPath:   "/state/sat/url",
		tokenPath: "/state/sat/token",
	}
	app, stdout, _ := newTestApp(t, withConfig(cfg))

	if err := run(t, app, "auth"); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{
		"State directory: /state/sat\n",
		"Base URL:        https://sat.example\n",
		"URL file:        /state/sat/url\n",
		"Token:           configured\n",
		"Token file:      /state/sat/token\n",
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
	t.Parallel()
	cfg := &fakeConfig{
		dir:       "/state/sat",
		urlPath:   "/state/sat/url",
		tokenPath: "/state/sat/token",
	}
	app, stdout, _ := newTestApp(t, withConfig(cfg))

	if err := run(t, app, "auth"); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Base URL:        not configured\n") {
		t.Fatalf("expected unconfigured base URL:\n%s", out)
	}
	if !strings.Contains(out, "Token:           not configured\n") {
		t.Fatalf("expected unconfigured token:\n%s", out)
	}
}

func TestAuthStatusEnvOverride(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	urlFile := filepath.Join(dir, "custom-url")
	tokenFile := filepath.Join(dir, "custom-token")
	if err := os.WriteFile(urlFile, []byte("https://override.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const tokenValue = "tok-7f3e9c1d-never-print"
	if err := os.WriteFile(tokenFile, []byte(tokenValue+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	env := map[string]string{"SAT_URL_PATH": urlFile, "SAT_TOKEN_PATH": tokenFile}
	getenv := func(key string) string { return env[key] }
	store := config.NewStore(filepath.Join(dir, "state"), getenv)
	app, stdout, _ := newTestApp(t, withConfig(store), withEnv(env))

	if err := run(t, app, "auth"); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, urlFile+"  (from SAT_URL_PATH)") {
		t.Fatalf("expected SAT_URL_PATH override annotation:\n%s", out)
	}
	if !strings.Contains(out, tokenFile+"  (from SAT_TOKEN_PATH)") {
		t.Fatalf("expected SAT_TOKEN_PATH override annotation:\n%s", out)
	}
	if !strings.Contains(out, "Base URL:        https://override.example\n") {
		t.Fatalf("expected overridden base URL:\n%s", out)
	}
	if !strings.Contains(out, "Token:           configured\n") {
		t.Fatalf("expected configured token:\n%s", out)
	}
	if strings.Contains(out, tokenValue) {
		t.Fatalf("token value leaked into output:\n%s", out)
	}
}

func TestAuthStatusInvalidURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "url"), []byte("not-a-url\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(dir, func(string) string { return "" })
	app, stdout, _ := newTestApp(t, withConfig(store))

	if err := run(t, app, "auth"); err != nil {
		t.Fatal(err)
	}
	if out := stdout.String(); !strings.Contains(out, "Base URL:        invalid (") {
		t.Fatalf("expected invalid base URL line:\n%s", out)
	}
}

func TestLoginMovedUnderAuth(t *testing.T) {
	t.Parallel()
	app, _, _ := newTestApp(t)
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

func TestAuthGetURLConfigured(t *testing.T) {
	t.Parallel()
	cfg := newFakeConfig(t)
	cfg.baseURL = "https://sat.example"
	app, stdout, stderr := newTestApp(t, withConfig(cfg))

	if err := run(t, app, "auth", "get-url"); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "https://sat.example\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestAuthGetURLUnconfigured(t *testing.T) {
	t.Parallel()
	cfg := newFakeConfig(t)
	cfg.baseURL = ""
	app, stdout, _ := newTestApp(t, withConfig(cfg))

	err := run(t, app, "auth", "get-url")
	if !errors.Is(err, config.ErrBaseURLMissing) {
		t.Fatalf("error = %v, want ErrBaseURLMissing", err)
	}
	if !strings.HasSuffix(err.Error(), "; "+loginHint) {
		t.Fatalf("error = %q, want login hint", err)
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}

func TestAuthGetURLRejectsArgs(t *testing.T) {
	t.Parallel()
	app, stdout, _ := newTestApp(t)

	if err := run(t, app, "auth", "get-url", "extra"); err == nil {
		t.Fatal("expected an error for an unexpected argument")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
}
