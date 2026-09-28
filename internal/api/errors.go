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
	StatusCode int
	Method     string
	// Path is the escaped request path.
	Path string
	// Body is the redacted and bounded response body.
	Body string
	// Message is the "message" field of a JSON error body, if any.
	Message   string
	Truncated bool
}

// Error returns a bounded description of an HTTP failure.
func (e *HTTPError) Error() string {
	message := fmt.Sprintf("%s %s: HTTP %d %s", e.Method, e.Path, e.StatusCode, http.StatusText(e.StatusCode))
	detail := e.Message
	if detail == "" {
		detail = e.Body
	}
	if detail != "" {
		message += ": " + detail
	}
	if e.Truncated {
		message += " (response body truncated)"
	}

	switch e.StatusCode {
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
		return e.StatusCode == http.StatusUnauthorized
	case ErrForbidden:
		return e.StatusCode == http.StatusForbidden
	}
	return false
}

// jsonMessage returns the trimmed "message" field of a JSON error body, or ""
// when body is not such a document.
func jsonMessage(body string) string {
	var payload struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.Message)
}

// SpotifyError describes an error returned with a successful Spotify response.
type SpotifyError struct {
	Message string
}

// Error returns the Spotify error message.
func (e *SpotifyError) Error() string {
	return e.Message
}
