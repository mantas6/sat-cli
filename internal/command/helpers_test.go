package command

import (
	"errors"
	"net/url"
	"strings"

	configpkg "github.com/mantas6/sat-cli/internal/config"
)

type memoryConfig struct {
	ConfigStore
	dir     string
	baseURL string
	token   string
}

func (c *memoryConfig) BaseURL() (string, error) {
	if c.baseURL == "" {
		return "", configpkg.ErrBaseURLMissing
	}
	return c.baseURL, nil
}

func (c *memoryConfig) Token() (string, error) {
	if c.token == "" {
		return "", configpkg.ErrTokenMissing
	}
	return c.token, nil
}

func (c *memoryConfig) SetBaseURL(value string) error {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("base URL must be an absolute http or https URL")
	}
	c.baseURL = value
	return nil
}

func (c *memoryConfig) SetToken(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return configpkg.ErrTokenMissing
	}
	c.token = value
	return nil
}

func (c *memoryConfig) HasBaseURL() bool { return c.baseURL != "" }
func (c *memoryConfig) HasToken() bool   { return c.token != "" }
func (c *memoryConfig) Dir() string      { return c.dir }
