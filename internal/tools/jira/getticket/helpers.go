package getticket

import (
	"fmt"
	"regexp"
	"strings"
)

// keyRE matches a Jira issue key: an uppercase project prefix (with
// optional digits/underscores after the first letter), a dash, and a
// numeric sequence. Mirrors Jira Cloud's own validator so we reject
// obvious typos before any HTTP call.
var keyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]+-\d+$`)

// browseURLRE extracts the issue key from a /browse/<KEY> path
// segment. The trailing alternation allows a slash, end-of-string, or
// the start of a query string after the key.
var browseURLRE = regexp.MustCompile(`/browse/([A-Z][A-Z0-9_]+-\d+)(?:/|$|\?)`)

// titleSanitiseRE collapses any run of path-separator-ish characters
// into a single space. Slash, backslash, colon, and NUL would break
// the eventual filename; the rest are kept verbatim because Obsidian
// is happy with them.
var titleSanitiseRE = regexp.MustCompile(`[\\/:\x00]+`)

// whitespaceRE collapses any run of whitespace (including tabs and
// newlines) into a single space.
var whitespaceRE = regexp.MustCompile(`\s+`)

// sanitiseTitle prepares a Jira summary for use as a filename. An
// empty result signals the caller to fall back to the bare key.
func sanitiseTitle(s string) string {
	s = titleSanitiseRE.ReplaceAllString(s, " ")
	s = whitespaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// requireString reads key from m as a non-empty string, or returns an
// error mentioning the key. The tool only accepts string-typed ticket
// and output_dir parameters; we deliberately do NOT coerce other types.
func requireString(m map[string]any, key string) (string, error) {
	raw, ok := m[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return s, nil
}

// optionalString reads key from m as a string, returning def when the
// key is absent or the explicit value is "".
func optionalString(m map[string]any, key, def string) (string, error) {
	raw, ok := m[key]
	if !ok {
		return def, nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	if s == "" {
		return def, nil
	}
	return s, nil
}
