// Package config manages sat's configuration and on-disk state.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	stateDirMode  = 0o700
	stateFileMode = 0o600
)

var (
	// ErrBaseURLMissing indicates that no server URL has been configured.
	ErrBaseURLMissing = errors.New("base URL is not configured")
	// ErrTokenMissing indicates that no API token has been configured.
	ErrTokenMissing = errors.New("token is not configured")
	// ErrEmptyToken indicates an attempt to save a blank API token.
	ErrEmptyToken = errors.New("token must not be empty")
)

// Store reads and writes configuration and caches under one state directory.
type Store struct {
	dir    string
	getenv func(string) string
}

// NewStore returns a Store rooted at stateDir with an injectable environment lookup.
func NewStore(stateDir string, getenv func(string) string) *Store {
	if getenv == nil {
		getenv = os.Getenv
	}

	return &Store{dir: stateDir, getenv: getenv}
}

// SplitTabs splits one legacy cache line into its tab-delimited fields.
func SplitTabs(line string) []string {
	return strings.Split(line, "\t")
}

// Dir returns the Store's state directory.
func (s *Store) Dir() string {
	return s.dir
}

// BaseURL returns the validated server URL read from the configured URL path.
func (s *Store) BaseURL() (string, error) {
	value, err := s.readTrimmed(s.urlPath(), ErrBaseURLMissing)
	if err != nil {
		return "", err
	}

	return validateBaseURL(value)
}

// Token returns the configured bearer token with surrounding whitespace removed.
func (s *Store) Token() (string, error) {
	return s.readTrimmed(s.tokenPath(), ErrTokenMissing)
}

// SetBaseURL validates and atomically persists a server URL.
func (s *Store) SetBaseURL(value string) error {
	value, err := validateBaseURL(value)
	if err != nil {
		return err
	}

	return s.writeAtomic(s.urlPath(), []byte(value+"\n"))
}

// SetToken atomically persists a bearer token. A blank value is rejected with
// ErrEmptyToken.
func (s *Store) SetToken(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return ErrEmptyToken
	}

	return s.writeAtomic(s.tokenPath(), []byte(value+"\n"))
}

// HasBaseURL reports whether a non-empty URL file exists.
func (s *Store) HasBaseURL() bool {
	value, err := os.ReadFile(s.urlPath())
	return err == nil && strings.TrimSpace(string(value)) != ""
}

// HasToken reports whether a non-empty token file exists.
func (s *Store) HasToken() bool {
	value, err := os.ReadFile(s.tokenPath())
	return err == nil && strings.TrimSpace(string(value)) != ""
}

// URLPath returns the file the base URL is read from and written to, honouring
// the SAT_URL_PATH override.
func (s *Store) URLPath() string {
	return s.urlPath()
}

// TokenPath returns the file the token is read from and written to, honouring
// the SAT_TOKEN_PATH override.
func (s *Store) TokenPath() string {
	return s.tokenPath()
}

// urlPath returns the file the base URL is read from and written to. It honours
// the SAT_URL_PATH override, falling back to the state directory's url file.
func (s *Store) urlPath() string {
	if value := strings.TrimSpace(s.getenv("SAT_URL_PATH")); value != "" {
		return value
	}

	return s.path("url")
}

// tokenPath returns the file the token is read from and written to. It honours
// the SAT_TOKEN_PATH override, falling back to the state directory's token file.
func (s *Store) tokenPath() string {
	if value := strings.TrimSpace(s.getenv("SAT_TOKEN_PATH")); value != "" {
		return value
	}

	return s.path("token")
}

// TmpDir creates, if necessary, and returns the private temporary directory.
func (s *Store) TmpDir() (string, error) {
	if err := ensureDir(s.dir); err != nil {
		return "", err
	}

	path := s.path("tmp")
	if err := os.MkdirAll(path, stateDirMode); err != nil {
		return "", fmt.Errorf("create temporary state directory: %w", err)
	}
	if err := os.Chmod(path, stateDirMode); err != nil {
		return "", fmt.Errorf("secure temporary state directory: %w", err)
	}

	return path, nil
}

// ReadCacheLines reads a newline-delimited cache. The boolean is false when
// the cache does not exist.
func (s *Store) ReadCacheLines(name string) ([]string, bool, error) {
	if err := validateCacheName(name); err != nil {
		return nil, false, err
	}

	data, err := os.ReadFile(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read cache %q: %w", name, err)
	}

	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return []string{}, true, nil
	}

	return strings.Split(text, "\n"), true, nil
}

// WriteCacheLines atomically writes a newline-delimited cache.
func (s *Store) WriteCacheLines(name string, lines []string) error {
	if err := validateCacheName(name); err != nil {
		return err
	}

	data := []byte(strings.Join(lines, "\n"))
	if len(lines) > 0 {
		data = append(data, '\n')
	}

	return s.writeAtomic(s.path(name), data)
}

// RemoveCache removes a cache, treating a missing cache as success.
func (s *Store) RemoveCache(name string) error {
	if err := validateCacheName(name); err != nil {
		return err
	}

	err := os.Remove(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove cache %q: %w", name, err)
	}

	return nil
}

// readTrimmed returns the whitespace-trimmed contents of path. A missing or
// blank file yields missing wrapped with the path, so errors.Is still matches
// the sentinel while the message names the file that was consulted.
func (s *Store) readTrimmed(path string, missing error) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w (%s)", missing, path)
	}
	if err != nil {
		// The *fs.PathError already names the path.
		return "", fmt.Errorf("read configuration: %w", err)
	}

	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("%w (%s is empty)", missing, path)
	}

	return value, nil
}

func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(dir, stateDirMode); err != nil {
		return fmt.Errorf("secure state directory: %w", err)
	}

	return nil
}

// writeAtomic replaces target via a private temporary file in the same
// directory. Only the state directory itself is re-secured to 0700; parents of
// SAT_URL_PATH/SAT_TOKEN_PATH overrides (for example $HOME or /tmp) are created
// when missing but their existing permissions are left alone.
func (s *Store) writeAtomic(target string, data []byte) (err error) {
	dir := filepath.Dir(target)
	if filepath.Clean(dir) == filepath.Clean(s.dir) {
		if err := ensureDir(dir); err != nil {
			return err
		}
	} else if err := os.MkdirAll(dir, stateDirMode); err != nil {
		return fmt.Errorf("create directory for %s: %w", filepath.Base(target), err)
	}

	base := filepath.Base(target)
	temporary, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary %s file: %w", base, err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()

	if err := temporary.Chmod(stateFileMode); err != nil {
		return fmt.Errorf("secure temporary %s file: %w", base, err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary %s file: %w", base, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary %s file: %w", base, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary %s file: %w", base, err)
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("replace %s file: %w", base, err)
	}

	return nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name)
}

// reservedStateNames are state-directory entries that are not caches.
var reservedStateNames = map[string]bool{
	"token": true,
	"url":   true,
	"tmp":   true,
}

func validateCacheName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("invalid cache name %q", name)
	}
	if reservedStateNames[name] {
		return fmt.Errorf("invalid cache name %q: reserved for sat state", name)
	}

	return nil
}
