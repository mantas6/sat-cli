package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStateDirPrecedence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "journal state wins",
			env:  map[string]string{"SAT_JOURNAL_STATE": "/custom", "XDG_STATE_HOME": "/xdg", "HOME": "/home/test"},
			want: "/custom",
		},
		{
			name: "xdg state home",
			env:  map[string]string{"XDG_STATE_HOME": "/xdg", "HOME": "/home/test"},
			want: "/xdg/sat",
		},
		{
			name: "home fallback",
			env:  map[string]string{"HOME": "/home/test"},
			want: "/home/test/.local/state/sat",
		},
		{
			name: "relative xdg state home is ignored",
			env:  map[string]string{"XDG_STATE_HOME": "relative/state", "HOME": "/home/test"},
			want: "/home/test/.local/state/sat",
		},
		{
			name: "journal state without home",
			env:  map[string]string{"SAT_JOURNAL_STATE": "/custom"},
			want: "/custom",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := StateDir(mapEnv(test.env))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("StateDir() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStateDirRequiresAbsoluteHome(t *testing.T) {
	t.Parallel()
	tests := map[string]map[string]string{
		"empty home":                    {},
		"blank home":                    {"HOME": "  "},
		"relative home":                 {"HOME": "home/test"},
		"relative xdg and missing home": {"XDG_STATE_HOME": "relative"},
	}
	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := StateDir(mapEnv(env))
			if err == nil {
				t.Fatalf("StateDir() = %q, want error", got)
			}
			if got != "" || !strings.Contains(err.Error(), "HOME") {
				t.Fatalf("StateDir() = %q, %v; want empty path and HOME error", got, err)
			}
		})
	}
}

func TestStoreReadsConfiguration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "url"), []byte("  https://file.example/prefix  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("  secret-token\n\t"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(dir, mapEnv(nil))
	baseURL, err := store.BaseURL()
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "https://file.example/prefix" {
		t.Fatalf("BaseURL() = %q", baseURL)
	}
	token, err := store.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != "secret-token" {
		t.Fatalf("Token() = %q", token)
	}
	if !store.HasBaseURL() || !store.HasToken() {
		t.Fatal("expected existing configuration to be reported")
	}
}

func TestStoreURLPathPrecedenceOnRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "url"), []byte("https://file.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(t.TempDir(), "url-override")
	if err := os.WriteFile(override, []byte("https://override.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(dir, mapEnv(map[string]string{"SAT_URL_PATH": override}))
	baseURL, err := store.BaseURL()
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "https://override.example" {
		t.Fatalf("BaseURL() = %q, want override", baseURL)
	}
	if !store.HasBaseURL() {
		t.Fatal("override URL not reported as present")
	}
}

func TestStoreTokenPathPrecedenceOnRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(t.TempDir(), "token-override")
	if err := os.WriteFile(override, []byte("override-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(dir, mapEnv(map[string]string{"SAT_TOKEN_PATH": override}))
	token, err := store.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != "override-token" {
		t.Fatalf("Token() = %q, want override", token)
	}
	if !store.HasToken() {
		t.Fatal("override token not reported as present")
	}
}

func TestStoreWritesThroughPathOverrides(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	urlOverride := filepath.Join(t.TempDir(), "url-override")
	tokenOverride := filepath.Join(t.TempDir(), "token-override")

	store := NewStore(dir, mapEnv(map[string]string{
		"SAT_URL_PATH":   urlOverride,
		"SAT_TOKEN_PATH": tokenOverride,
	}))
	if err := store.SetBaseURL("https://override.example"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetToken("override-token"); err != nil {
		t.Fatal(err)
	}

	if data, err := os.ReadFile(urlOverride); err != nil || strings.TrimSpace(string(data)) != "https://override.example" {
		t.Fatalf("url override = %q, %v", data, err)
	}
	if data, err := os.ReadFile(tokenOverride); err != nil || strings.TrimSpace(string(data)) != "override-token" {
		t.Fatalf("token override = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "url")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default url file was written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "token")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default token file was written: %v", err)
	}
}

func TestStoreMissingPathOverride(t *testing.T) {
	t.Parallel()
	urlOverride := filepath.Join(t.TempDir(), "missing-url")
	tokenOverride := filepath.Join(t.TempDir(), "missing-token")

	store := NewStore(t.TempDir(), mapEnv(map[string]string{
		"SAT_URL_PATH":   urlOverride,
		"SAT_TOKEN_PATH": tokenOverride,
	}))
	if _, err := store.BaseURL(); !errors.Is(err, ErrBaseURLMissing) {
		t.Fatalf("BaseURL() error = %v", err)
	}
	if _, err := store.Token(); !errors.Is(err, ErrTokenMissing) {
		t.Fatalf("Token() error = %v", err)
	}
	if store.HasBaseURL() || store.HasToken() {
		t.Fatal("missing override reported as present")
	}
}

func TestStoreCreatesNestedOverrideDirectory(t *testing.T) {
	t.Parallel()
	urlOverride := filepath.Join(t.TempDir(), "nested", "deeper", "url")
	store := NewStore(t.TempDir(), mapEnv(map[string]string{"SAT_URL_PATH": urlOverride}))

	if err := store.SetBaseURL("https://override.example"); err != nil {
		t.Fatal(err)
	}
	assertPerm(t, filepath.Dir(urlOverride), 0o700)
	assertPerm(t, urlOverride, 0o600)
}

func TestStoreLeavesExistingOverrideParentPermissions(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	store := NewStore(stateDir, mapEnv(map[string]string{
		"SAT_URL_PATH":   filepath.Join(parent, "url"),
		"SAT_TOKEN_PATH": filepath.Join(parent, "token"),
	}))

	if err := store.SetBaseURL("https://override.example"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetToken("override-token"); err != nil {
		t.Fatal(err)
	}
	assertPerm(t, parent, 0o755)
	assertPerm(t, filepath.Join(parent, "url"), 0o600)
	assertPerm(t, filepath.Join(parent, "token"), 0o600)
	if _, err := os.Stat(stateDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state directory created for override-only writes: %v", err)
	}
}

func TestStoreMissingConfiguration(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir(), mapEnv(nil))

	if _, err := store.BaseURL(); !errors.Is(err, ErrBaseURLMissing) {
		t.Fatalf("BaseURL() error = %v", err)
	}
	if _, err := store.Token(); !errors.Is(err, ErrTokenMissing) {
		t.Fatalf("Token() error = %v", err)
	}
	if store.HasBaseURL() || store.HasToken() {
		t.Fatal("missing configuration was reported as present")
	}
}

func TestStoreMissingErrorsNameTheFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewStore(dir, mapEnv(nil))
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := store.BaseURL()
	if !errors.Is(err, ErrBaseURLMissing) || !strings.Contains(err.Error(), filepath.Join(dir, "url")) {
		t.Fatalf("BaseURL() error = %v, want ErrBaseURLMissing naming the url file", err)
	}
	_, err = store.Token()
	if !errors.Is(err, ErrTokenMissing) || !strings.Contains(err.Error(), filepath.Join(dir, "token")+" is empty") {
		t.Fatalf("Token() error = %v, want ErrTokenMissing naming the empty token file", err)
	}
}

func TestSetTokenRejectsBlankToken(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewStore(dir, mapEnv(nil))
	for _, value := range []string{"", "   ", "\n\t"} {
		if err := store.SetToken(value); !errors.Is(err, ErrEmptyToken) {
			t.Fatalf("SetToken(%q) = %v, want ErrEmptyToken", value, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "token")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("blank token created a file: %v", err)
	}
}

func TestStoreRejectsMalformedBaseURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewStore(dir, mapEnv(nil))

	for _, value := range []string{"", "example.com", "ftp://example.com", "://bad", "https://u:p@example.com", "https://example.com/?q=1", "https://example.com/#top"} {
		if err := store.SetBaseURL(value); err == nil {
			t.Fatalf("SetBaseURL(%q) succeeded", value)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "url")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid URL created a file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "url"), []byte("not-a-url\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BaseURL(); err == nil || !strings.Contains(err.Error(), "absolute http or https URL") {
		t.Fatalf("BaseURL() error = %v", err)
	}
}

func TestStoreWritesPrivateAtomicFiles(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	dir := filepath.Join(parent, "state")
	store := NewStore(dir, mapEnv(nil))

	if err := store.SetBaseURL("https://sat.example"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetToken(" token "); err != nil {
		t.Fatal(err)
	}
	tmp, err := store.TmpDir()
	if err != nil {
		t.Fatal(err)
	}
	if tmp != filepath.Join(dir, "tmp") || store.Dir() != dir {
		t.Fatalf("unexpected paths: tmp=%q dir=%q", tmp, store.Dir())
	}

	assertPerm(t, dir, 0o700)
	assertPerm(t, tmp, 0o700)
	assertPerm(t, filepath.Join(dir, "token"), 0o600)
	assertPerm(t, filepath.Join(dir, "url"), 0o600)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("atomic write left temporary file %q", entry.Name())
		}
	}
}

func TestCacheLifecycle(t *testing.T) {
	t.Parallel()
	store := NewStore(filepath.Join(t.TempDir(), "state"), mapEnv(nil))
	if lines, exists, err := store.ReadCacheLines("tracks"); err != nil || exists || lines != nil {
		t.Fatalf("missing cache = %#v, %v, %v", lines, exists, err)
	}

	want := []string{"1\tartist\t/album\t/01.\ttitle", "2\tother"}
	if err := store.WriteCacheLines("tracks", want); err != nil {
		t.Fatal(err)
	}
	got, exists, err := store.ReadCacheLines("tracks")
	if err != nil || !exists || !reflect.DeepEqual(got, want) {
		t.Fatalf("ReadCacheLines() = %#v, %v, %v", got, exists, err)
	}

	if err := store.RemoveCache("tracks"); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveCache("tracks"); err != nil {
		t.Fatalf("removing missing cache: %v", err)
	}
	if _, exists, err := store.ReadCacheLines("tracks"); err != nil || exists {
		t.Fatalf("removed cache still exists: %v, %v", exists, err)
	}
	if err := store.WriteCacheLines("../outside", nil); err == nil {
		t.Fatal("unsafe cache name was accepted")
	}
}

func TestWriteCacheLinesRejectsLineBreaks(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir(), mapEnv(nil))
	for _, line := range []string{"a\nb", "a\rb", "trailing\n"} {
		if err := store.WriteCacheLines("tracks", []string{"ok", line}); err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("WriteCacheLines(%q) = %v, want line break error for line 2", line, err)
		}
	}
	if _, exists, err := store.ReadCacheLines("tracks"); err != nil || exists {
		t.Fatalf("rejected cache was written: exists=%v, err=%v", exists, err)
	}
}

func TestWriteThroughSymlinkKeepsLink(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	linkTarget := filepath.Join(dir, "dotfiles", "sat-token")
	if err := os.MkdirAll(filepath.Dir(linkTarget), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linkTarget, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A relative, two-level chain: token -> link -> ../dotfiles/sat-token.
	links := filepath.Join(dir, "links")
	if err := os.MkdirAll(links, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../dotfiles/sat-token", filepath.Join(links, "link")); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(links, "token")
	if err := os.Symlink("link", tokenPath); err != nil {
		t.Fatal(err)
	}

	store := NewStore(stateDir, mapEnv(map[string]string{"SAT_TOKEN_PATH": tokenPath}))
	if err := store.SetToken("new"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(tokenPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("token path is no longer a symlink: %v, %v", info, err)
	}
	if data, err := os.ReadFile(linkTarget); err != nil || string(data) != "new\n" {
		t.Fatalf("link target = %q, %v; want new token", data, err)
	}
	assertPerm(t, linkTarget, 0o600)
	assertPerm(t, filepath.Dir(linkTarget), 0o755)
	if token, err := store.Token(); err != nil || token != "new" {
		t.Fatalf("Token() = %q, %v", token, err)
	}
}

func TestWriteThroughDanglingSymlinkCreatesTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "target-url")
	urlPath := filepath.Join(dir, "url")
	if err := os.Symlink(target, urlPath); err != nil {
		t.Fatal(err)
	}

	store := NewStore(filepath.Join(dir, "state"), mapEnv(map[string]string{"SAT_URL_PATH": urlPath}))
	if err := store.SetBaseURL("https://sat.example"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(urlPath); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("url path is no longer a symlink: %v, %v", info, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "https://sat.example\n" {
		t.Fatalf("link target = %q, %v", data, err)
	}
}

func TestWriteThroughSymlinkLoopFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	loop := filepath.Join(dir, "loop")
	if err := os.Symlink("loop", loop); err != nil {
		t.Fatal(err)
	}

	store := NewStore(filepath.Join(dir, "state"), mapEnv(map[string]string{"SAT_TOKEN_PATH": loop}))
	if err := store.SetToken("token"); err == nil || !strings.Contains(err.Error(), "symbolic links") {
		t.Fatalf("SetToken() = %v, want symlink loop error", err)
	}
}

func TestTmpDirResecuresExistingDirectory(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, "tmp"), 0o755); err != nil {
		t.Fatal(err)
	}

	tmp, err := NewStore(dir, mapEnv(nil)).TmpDir()
	if err != nil {
		t.Fatal(err)
	}
	assertPerm(t, dir, 0o700)
	assertPerm(t, tmp, 0o700)
}

func TestReadEmptyCacheFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewStore(dir, mapEnv(nil))
	for name, contents := range map[string]string{"empty": "", "newline": "\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		lines, exists, err := store.ReadCacheLines(name)
		if err != nil || !exists || lines == nil || len(lines) != 0 {
			t.Fatalf("ReadCacheLines(%s) = %#v, %v, %v; want empty existing cache", name, lines, exists, err)
		}
	}

	if err := store.WriteCacheLines("written", nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "written")); err != nil || len(data) != 0 {
		t.Fatalf("WriteCacheLines(nil) wrote %q, %v; want an empty file", data, err)
	}
}

func TestURLAndTokenPaths(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewStore(dir, mapEnv(nil))
	if store.URLPath() != filepath.Join(dir, "url") || store.TokenPath() != filepath.Join(dir, "token") {
		t.Fatalf("default paths = %q, %q", store.URLPath(), store.TokenPath())
	}

	store = NewStore(dir, mapEnv(map[string]string{"SAT_URL_PATH": "  /etc/sat/url \n", "SAT_TOKEN_PATH": "/run/sat/token"}))
	if store.URLPath() != "/etc/sat/url" || store.TokenPath() != "/run/sat/token" {
		t.Fatalf("override paths = %q, %q", store.URLPath(), store.TokenPath())
	}

	store = NewStore(dir, mapEnv(map[string]string{"SAT_URL_PATH": "   ", "SAT_TOKEN_PATH": "\t"}))
	if store.URLPath() != filepath.Join(dir, "url") || store.TokenPath() != filepath.Join(dir, "token") {
		t.Fatalf("blank overrides = %q, %q; want defaults", store.URLPath(), store.TokenPath())
	}
}

// Not parallel: t.Setenv changes the process environment.
func TestNilGetenvUsesProcessEnvironment(t *testing.T) {
	override := filepath.Join(t.TempDir(), "token")
	t.Setenv("SAT_TOKEN_PATH", override)
	state := t.TempDir()
	t.Setenv("SAT_JOURNAL_STATE", state)

	store := NewStore(t.TempDir(), nil)
	if got := store.TokenPath(); got != override {
		t.Fatalf("TokenPath() = %q, want %q from the environment", got, override)
	}
	if got, err := StateDir(nil); err != nil || got != state {
		t.Fatalf("StateDir(nil) = %q, %v; want %q", got, err, state)
	}
}

func TestCacheNameValidation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewStore(dir, mapEnv(nil))
	if err := store.SetToken("keep-me"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"", ".", "..", "../outside", "nested/name", "token", "url", "tmp"} {
		// Sequential: the token check below must run after every subtest.
		t.Run(name, func(t *testing.T) {
			if err := store.WriteCacheLines(name, []string{"x"}); err == nil {
				t.Fatalf("WriteCacheLines(%q) succeeded", name)
			}
			if _, _, err := store.ReadCacheLines(name); err == nil {
				t.Fatalf("ReadCacheLines(%q) succeeded", name)
			}
			if err := store.RemoveCache(name); err == nil {
				t.Fatalf("RemoveCache(%q) succeeded", name)
			}
		})
	}

	if token, err := store.Token(); err != nil || token != "keep-me" {
		t.Fatalf("Token() = %q, %v; want token untouched", token, err)
	}
	for _, name := range []string{"list", "tracks", "journals"} {
		if err := validateCacheName(name); err != nil {
			t.Fatalf("validateCacheName(%q) = %v", name, err)
		}
	}
}

func TestSetTokenUnwritableDirectoryKeepsOriginal(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}

	// Only the state directory is re-secured by writeAtomic, so a read-only
	// SAT_TOKEN_PATH parent stays read-only and creating the temporary file
	// fails.
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	store := NewStore(filepath.Join(t.TempDir(), "state"), mapEnv(map[string]string{"SAT_TOKEN_PATH": tokenPath}))
	if err := store.SetToken("new"); err == nil || !strings.Contains(err.Error(), "create temporary token file") {
		t.Fatalf("SetToken() = %v, want temporary file creation error", err)
	}
	if token, err := store.Token(); err != nil || token != "original" {
		t.Fatalf("Token() = %q, %v; want original", token, err)
	}
	assertOnlyEntries(t, dir, "token")
}

func TestSetTokenFailedRenameRemovesTemporaryFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// A non-empty directory where the token file should be makes the final
	// rename fail after the temporary file was written.
	tokenPath := filepath.Join(dir, "token")
	if err := os.MkdirAll(filepath.Join(tokenPath, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}

	store := NewStore(filepath.Join(t.TempDir(), "state"), mapEnv(map[string]string{"SAT_TOKEN_PATH": tokenPath}))
	if err := store.SetToken("new"); err == nil || !strings.Contains(err.Error(), "replace token file") {
		t.Fatalf("SetToken() = %v, want rename error", err)
	}
	assertOnlyEntries(t, dir, "token")
}

func TestBaseURLUnreadableFile(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}

	dir := t.TempDir()
	urlPath := filepath.Join(dir, "url")
	if err := os.WriteFile(urlPath, []byte("https://sat.example\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(urlPath, 0o600) })

	store := NewStore(dir, mapEnv(nil))
	_, err := store.BaseURL()
	if err == nil {
		t.Fatal("BaseURL succeeded on unreadable file")
	}
	if errors.Is(err, ErrBaseURLMissing) {
		t.Fatalf("BaseURL returned missing sentinel: %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "read") || !strings.Contains(msg, "permission denied") {
		t.Fatalf("BaseURL error = %v, want read/permission-denied failure", err)
	}
}

// assertOnlyEntries fails unless dir contains exactly the named entries.
func assertOnlyEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s contains %q, want %q", dir, got, want)
	}
}

func mapEnv(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s permissions = %o, want %o", path, got, want)
	}
}
