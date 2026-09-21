package command

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"

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

func TestConfigValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "path", args: []string{"path"}, want: "/state/sat\n"},
		{name: "url", args: []string{"url"}, want: "https://sat.example\n"},
		{name: "token", args: []string{"token"}, want: "secret-token\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			app := &App{
				Config: &memoryConfig{dir: "/state/sat", baseURL: "https://sat.example", token: "secret-token"},
				Stdout: &output,
				Stderr: &output,
			}
			command := newConfigCommand(app)
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if got := output.String(); got != test.want {
				t.Fatalf("output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestConfigMissingValues(t *testing.T) {
	for _, subcommand := range []string{"url", "token"} {
		t.Run(subcommand, func(t *testing.T) {
			command := newConfigCommand(&App{Config: &memoryConfig{}})
			command.SetArgs([]string{subcommand})
			if err := command.Execute(); err == nil {
				t.Fatal("expected missing configuration error")
			}
		})
	}
}
