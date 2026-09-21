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
	ErrBaseURLMissing = errors.New("URL is not configured. Run `sat login`.")
	// ErrTokenMissing indicates that no API token has been configured.
	ErrTokenMissing = errors.New("Token is not configured. Run `sat login`.")
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

// BaseURL returns the validated server URL. SAT_BASE_URL takes precedence over
// the persisted URL.
func (s *Store) BaseURL() (string, error) {
	if value := strings.TrimSpace(s.getenv("SAT_BASE_URL")); value != "" {
		return validateBaseURL(value)
	}

	value, err := s.readTrimmed("url", ErrBaseURLMissing)
	if err != nil {
		return "", err
	}

	return validateBaseURL(value)
}

// Token returns the configured bearer token with surrounding whitespace removed.
func (s *Store) Token() (string, error) {
	return s.readTrimmed("token", ErrTokenMissing)
}

// SetBaseURL validates and atomically persists a server URL.
func (s *Store) SetBaseURL(value string) error {
	value, err := validateBaseURL(value)
	if err != nil {
		return err
	}

	return s.writeAtomic("url", []byte(value+"\n"))
}

// SetToken atomically persists a bearer token.
func (s *Store) SetToken(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return ErrTokenMissing
	}

	return s.writeAtomic("token", []byte(value+"\n"))
}

// HasBaseURL reports whether a non-empty URL override or URL file exists.
func (s *Store) HasBaseURL() bool {
	if strings.TrimSpace(s.getenv("SAT_BASE_URL")) != "" {
		return true
	}

	value, err := os.ReadFile(s.path("url"))
	return err == nil && strings.TrimSpace(string(value)) != ""
}

// HasToken reports whether a non-empty token file exists.
func (s *Store) HasToken() bool {
	value, err := os.ReadFile(s.path("token"))
	return err == nil && strings.TrimSpace(string(value)) != ""
}

// TmpDir creates, if necessary, and returns the private temporary directory.
func (s *Store) TmpDir() (string, error) {
	if err := s.ensureDir(); err != nil {
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

	return s.writeAtomic(name, data)
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

func (s *Store) readTrimmed(name string, missing error) (string, error) {
	data, err := os.ReadFile(s.path(name))
	if errors.Is(err, os.ErrNotExist) {
		return "", missing
	}
	if err != nil {
		return "", fmt.Errorf("read %s configuration: %w", name, err)
	}

	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", missing
	}

	return value, nil
}

func (s *Store) ensureDir() error {
	if err := os.MkdirAll(s.dir, stateDirMode); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(s.dir, stateDirMode); err != nil {
		return fmt.Errorf("secure state directory: %w", err)
	}

	return nil
}

func (s *Store) writeAtomic(name string, data []byte) (err error) {
	if err := s.ensureDir(); err != nil {
		return err
	}

	temporary, err := os.CreateTemp(s.dir, "."+name+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary %s file: %w", name, err)
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
	}()

	if err := temporary.Chmod(stateFileMode); err != nil {
		return fmt.Errorf("secure temporary %s file: %w", name, err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary %s file: %w", name, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary %s file: %w", name, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary %s file: %w", name, err)
	}
	if err := os.Rename(temporaryName, s.path(name)); err != nil {
		return fmt.Errorf("replace %s file: %w", name, err)
	}

	return nil
}

func (s *Store) path(name string) string {
	return filepath.Join(s.dir, name)
}

func validateCacheName(name string) error {
	if name == "" || name == "." || filepath.Base(name) != name {
		return fmt.Errorf("invalid cache name %q", name)
	}

	return nil
}
