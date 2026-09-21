package api

import (
	"net/url"
	"strings"
)

// JoinPath escapes and joins dynamic URL path segments with a leading slash.
func JoinPath(segments ...string) string {
	escaped := make([]string, len(segments))
	for index, segment := range segments {
		escaped[index] = url.PathEscape(segment)
	}

	return "/" + strings.Join(escaped, "/")
}
