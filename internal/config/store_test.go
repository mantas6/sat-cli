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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := StateDir(mapEnv(test.env)); got != test.want {
				t.Fatalf("StateDir() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStoreReadsConfigurationAndEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "url"), []byte("  https://file.example/prefix  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("  secret-token\n\t"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewStore(dir, mapEnv(map[string]string{"SAT_BASE_URL": " https://env.example/base "}))
	baseURL, err := store.BaseURL()
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "https://env.example/base" {
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

	store = NewStore(dir, mapEnv(nil))
	baseURL, err = store.BaseURL()
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "https://file.example/prefix" {
		t.Fatalf("file BaseURL() = %q", baseURL)
	}
}

func TestStoreMissingConfiguration(t *testing.T) {
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

func TestStoreRejectsMalformedBaseURL(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir, mapEnv(nil))

	for _, value := range []string{"", "example.com", "ftp://example.com", "://bad"} {
		if err := store.SetBaseURL(value); err == nil {
			t.Fatalf("SetBaseURL(%q) succeeded", value)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "url")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid URL created a file: %v", err)
	}

	store = NewStore(dir, mapEnv(map[string]string{"SAT_BASE_URL": "not-a-url"}))
	if _, err := store.BaseURL(); err == nil || !strings.Contains(err.Error(), "absolute http or https URL") {
		t.Fatalf("BaseURL() error = %v", err)
	}
}

func TestStoreWritesPrivateAtomicFiles(t *testing.T) {
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

func TestCacheLifecycleAndSplitTabs(t *testing.T) {
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
	if fields := SplitTabs(want[0]); len(fields) != 5 || fields[1] != "artist" {
		t.Fatalf("SplitTabs() = %#v", fields)
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
