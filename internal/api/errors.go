package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	// ErrUnauthorized matches an *HTTPError for an HTTP 401 response: the token
	// is missing, invalid or expired.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden matches an *HTTPError for an HTTP 403 response: the token
	// lacks the ability the endpoint requires.
	ErrForbidden = errors.New("forbidden")
)

// HTTPError describes a non-success HTTP response.
type HTTPError struct {
	Status    int
	Method    string
	Path      string
	Body      string
	Truncated bool
}

// Error returns a bounded description of an HTTP failure.
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
		message += "; token is invalid or expired"
	case http.StatusForbidden:
		message += "; token lacks the required ability"
	}

	return message
}

// Is reports whether target is the sentinel for this response's status, so
// errors.Is(err, ErrUnauthorized) matches any HTTP 401 response.
func (e *HTTPError) Is(target error) bool {
	switch target {
	case ErrUnauthorized:
		return e.Status == http.StatusUnauthorized
	case ErrForbidden:
		return e.Status == http.StatusForbidden
	}
	return false
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
