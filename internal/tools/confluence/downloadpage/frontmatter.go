package downloadpage

import (
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
)

// frontmatter holds the YAML keys we will write to the head of the
// downloaded markdown file. Keys preserves insertion order so the
// rendered block matches the order they appeared in the property table
// (with confluence_page_id always first).
type frontmatter struct {
	Keys   []string
	Values map[string]string
}

// render serialises the frontmatter as a YAML block delimited by `---`
// fences, ending with a single blank line so the body that follows
// starts on its own line. Returns "" when there are no keys at all,
// which never happens in practice because Invoke always seeds
// confluence_page_id.
func (f frontmatter) render() string {
	if len(f.Keys) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range f.Keys {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(scalarYAML(f.Values[k]))
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	return b.String()
}

// scalarYAML renders v as a YAML scalar suitable for the right-hand side
// of `key: ...`. The rules mirror the ones in internal/obsidian so files
// produced by download → publish round-trip without YAML drift.
func scalarYAML(v string) string {
	if needsQuoting(v) {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}

// needsQuoting reproduces the predicate from internal/obsidian (kept
// private to this package to avoid a cross-package dependency on a
// helper that is otherwise an implementation detail of the frontmatter
// writer).
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	switch s {
	case "true", "false", "null", "yes", "no", "on", "off":
		return true
	}
	for _, r := range s {
		if r == ':' || r == '#' || r == '\n' || r == '"' || r == '\'' {
			return true
		}
	}
	return false
}

// baseURLOf returns the configured Confluence base URL for clients that
// expose one (the production HTTPClient), or "" otherwise. The download
// tool uses it both for URL host validation and for assembling the web
// URL in the Result.
func baseURLOf(client atlassian.Client) string {
	if hc, ok := client.(interface{ BaseURL() string }); ok {
		return hc.BaseURL()
	}
	return ""
}

// resultFrom converts an atlassian.Page plus the destination path into
// the tool's Result. The web URL is best-effort — only the production
// HTTPClient exposes the base URL needed to build it.
func resultFrom(p atlassian.Page, dest string, client atlassian.Client) Result {
	r := Result{
		Action:   "downloaded",
		PageID:   p.ID,
		Title:    p.Title,
		SpaceID:  p.SpaceID,
		FilePath: dest,
	}
	if base := baseURLOf(client); base != "" {
		r.WebURL = p.WebURL(base)
	}
	return r
}
