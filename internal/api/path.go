package api

import (
	"fmt"
	"net/url"
	"strings"
)

// JoinPath escapes and joins dynamic URL path segments with a leading slash.
// Empty, "." and ".." segments are rejected because they would change which
// endpoint the request reaches.
func JoinPath(segments ...string) (string, error) {
	escaped := make([]string, len(segments))
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid URL path segment %q", segment)
		}
		escaped[index] = url.PathEscape(segment)
	}

	return "/" + strings.Join(escaped, "/"), nil
}
