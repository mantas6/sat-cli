package api

import (
	"strings"
	"testing"
)

func TestParseBaseURL(t *testing.T) {
	t.Parallel()
	valid := map[string]string{
		"https://sat.example":            "https://sat.example",
		"  http://sat.example:8080/  \n": "http://sat.example:8080/",
		"https://sat.example/prefix":     "https://sat.example/prefix",
		"HTTPS://sat.example":            "https://sat.example",
	}
	for raw, want := range valid {
		t.Run("valid "+raw, func(t *testing.T) {
			t.Parallel()
			parsed, err := ParseBaseURL(raw)
			if err != nil || parsed.String() != want {
				t.Fatalf("ParseBaseURL(%q) = %v, %v; want %q", raw, parsed, err, want)
			}
		})
	}

	invalid := map[string]string{
		"":                               "absolute http or https URL",
		"sat.example":                    "absolute http or https URL",
		"/relative":                      "absolute http or https URL",
		"ftp://sat.example":              "absolute http or https URL",
		"https://":                       "absolute http or https URL",
		"http:sat.example":               "absolute http or https URL",
		"://bad":                         "absolute http or https URL",
		"https://user:hunter2@sat.test":  "user information",
		"https://user@sat.test":          "user information",
		"https://sat.test/?token=secret": "query",
		"https://sat.test/?":             "query",
		"https://sat.test/#frag":         "fragment",
		"https://sat.test/#":             "fragment",
	}
	for raw, want := range invalid {
		t.Run("invalid "+raw, func(t *testing.T) {
			t.Parallel()
			parsed, err := ParseBaseURL(raw)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("ParseBaseURL(%q) = %v, %v; want error containing %q", raw, parsed, err, want)
			}
			if strings.Contains(err.Error(), "hunter2") {
				t.Fatalf("error leaks the password: %v", err)
			}
		})
	}
}

func TestNewClientRejectsInvalidBaseURLs(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "sat.example", "ftp://sat.example", "https://u:p@sat.test", "https://sat.test/?a=b", "https://sat.test/#x"} {
		if client, err := NewClient(raw, "token"); err == nil {
			t.Fatalf("NewClient(%q) = %#v, want error", raw, client)
		}
	}
}
