package api

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseBaseURL validates a Satellite base URL after trimming surrounding
// whitespace. It must be an absolute http or https URL with a host and may
// carry a path prefix, but no user information, query or fragment: those
// would be silently dropped or leak into every request.
func ParseBaseURL(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid base URL %q: use an absolute http or https URL", value)
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("invalid base URL %q: must not contain user information", parsed.Redacted())
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return nil, fmt.Errorf("invalid base URL %q: must not contain a query", value)
	}
	if parsed.Fragment != "" || strings.Contains(value, "#") {
		return nil, fmt.Errorf("invalid base URL %q: must not contain a fragment", value)
	}

	return parsed, nil
}
