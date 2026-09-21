package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// HTTPError describes a non-success HTTP response.
type HTTPError struct {
	Status    int
	Method    string
	Path      string
	Body      string
	Truncated bool
}

// Error returns a bounded, actionable description of an HTTP failure.
func (e *HTTPError) Error() string {
	message := fmt.Sprintf("%s %s: HTTP %d %s", e.Method, e.Path, e.Status, http.StatusText(e.Status))
	if detail := e.detail(); detail != "" {
		message += ": " + detail
	}
	if e.Truncated {
		message += " (response body truncated)"
	}

	switch e.Status {
	case http.StatusUnauthorized:
		message += "; token is invalid or expired; run `sat login`"
	case http.StatusForbidden:
		message += "; token lacks the required ability"
	}

	return message
}

func (e *HTTPError) detail() string {
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &payload) == nil && strings.TrimSpace(payload.Message) != "" {
		return strings.TrimSpace(payload.Message)
	}

	return e.Body
}

// SpotifyError describes an error returned with a successful Spotify response.
type SpotifyError struct {
	Message string
}

// Error returns the Spotify error message.
func (e *SpotifyError) Error() string {
	return e.Message
}
