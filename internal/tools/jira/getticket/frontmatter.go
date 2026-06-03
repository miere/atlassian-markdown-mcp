package getticket

import (
	"strings"
)

// frontmatter renders the fixed-order YAML block that prefaces every
// downloaded ticket. Keys are written in a deterministic sequence
// (see render) so file diffs across re-downloads only reflect actual
// content changes.
type frontmatter struct {
	Key         string
	Title       string
	Status      string
	Type        string
	ParentKey   string   // empty when the ticket has no parent on Jira
	Created     string
	Updated     string
	Labels      []string
	PRURLs      []string
	BranchNames []string
}

// render serialises the frontmatter as a YAML block delimited by
// `---` fences, ending with a single trailing newline. The output
// always starts with the four required scalar keys; the parent key,
// when present, slots in between status and dates. Sequences are
// emitted in YAML flow form `[a, b]` (empty as `[]`) so the update
// tool can distinguish "explicitly empty" from "absent".
func (f frontmatter) render() string {
	var b strings.Builder
	b.WriteString("---\n")
	writeScalar(&b, KeyTicketKey, f.Key)
	writeScalar(&b, KeyTicketTitle, f.Title)
	writeScalar(&b, KeyTicketStatus, f.Status)
	writeScalar(&b, KeyTicketType, f.Type)
	if f.ParentKey != "" {
		writeScalar(&b, KeyParentKey, f.ParentKey)
	}
	writeScalar(&b, KeyCreated, f.Created)
	writeScalar(&b, KeyUpdated, f.Updated)
	writeSequence(&b, KeyLabels, f.Labels)
	writeSequence(&b, KeyPullRequests, f.PRURLs)
	writeSequence(&b, KeyBranches, f.BranchNames)
	b.WriteString("---\n")
	return b.String()
}

// writeScalar emits `key: value\n` with quoting that matches the
// rules used by internal/obsidian so a download→update round-trip
// parses cleanly.
func writeScalar(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(scalarYAML(value))
	b.WriteByte('\n')
}

// writeSequence emits `key: [a, b]\n`, or `key: []\n` for an empty
// slice. Individual items are scalar-quoted with scalarYAML so
// commas, colons and other YAML-significant characters survive the
// round-trip.
func writeSequence(b *strings.Builder, key string, items []string) {
	b.WriteString(key)
	b.WriteString(": [")
	for i, item := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(scalarYAML(item))
	}
	b.WriteString("]\n")
}

// scalarYAML renders v as a YAML scalar suitable for the right-hand
// side of `key: ...`. Mirrors the predicate in internal/obsidian and
// in the Confluence download tool so files written by any of the
// download paths round-trip identically.
func scalarYAML(v string) string {
	if needsQuoting(v) {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}

// needsQuoting reproduces the predicate from internal/obsidian (kept
// private here to avoid a cross-package dependency on a helper that
// is otherwise an implementation detail of the frontmatter writer).
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	switch s {
	case "true", "false", "null", "yes", "no", "on", "off":
		return true
	}
	for _, r := range s {
		if r == ':' || r == '#' || r == '\n' || r == '"' || r == '\'' ||
			r == ',' || r == '[' || r == ']' || r == '{' || r == '}' {
			return true
		}
	}
	return false
}
