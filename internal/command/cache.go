package command

import (
	"strings"

	"github.com/mantas6/sat-cli/internal/ui"
)

// Cache names of the tab-delimited picker caches in the state directory.
const (
	articleCacheName = "list"
	trackCacheName   = "tracks"
)

// cacheFieldReplacer neutralises the separators of the tab-delimited,
// newline-terminated cache format inside a single field.
var cacheFieldReplacer = strings.NewReplacer("\r\n", " ", "\t", " ", "\n", " ", "\r", " ")

// cacheLineReplacer neutralises line breaks inside an already tab-delimited
// cache line so it stays one record.
var cacheLineReplacer = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// cacheLine formats item as one cache record: the ID followed by its columns.
func cacheLine(item ui.Item) string {
	return strings.Join(append([]string{item.ID}, item.Columns...), "\t")
}

// splitTabs splits one cache record into its tab-delimited fields.
func splitTabs(line string) []string {
	return strings.Split(line, "\t")
}

// parseCacheLines turns cache records into picker items. The first field is
// the item ID and the rest are its columns; blank lines and records without
// an ID are skipped.
func parseCacheLines(lines []string) []ui.Item {
	items := make([]ui.Item, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := splitTabs(line)
		if fields[0] == "" {
			continue
		}
		items = append(items, ui.Item{ID: fields[0], Columns: fields[1:]})
	}
	return items
}
