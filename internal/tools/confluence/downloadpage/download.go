// Package downloadpage implements `confluence.download-page`: it fetches a
// Confluence page by ID or URL, converts its ADF body to markdown, and
// writes the result to disk as an Obsidian-compatible note. The property
// table that `confluence.publish-obsidian-file` prepends to each published
// page is detected and lifted back into the YAML frontmatter, so a
// download → publish round-trip preserves the metadata that the publish
// tool itself controls.
package downloadpage

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
)

// KeyPageID is the frontmatter key written into every downloaded note. It
// matches the key the publish tool uses, so the downloaded file is
// publish-ready without any manual edit.
const KeyPageID = "confluence_page_id"

// DefaultOutputDir is the directory used when the caller does not pass
// `output_dir`. Kept identical to the documentation in the tool's
// Description so MCP clients see one canonical value.
const DefaultOutputDir = "/tmp/"

// Tool is the download-page capability. NewClient is overridden by tests
// to substitute a fake atlassian.Client.
type Tool struct {
	NewClient func() (atlassian.Client, error)
}

// New constructs the tool with an Atlassian HTTP client that loads its
// config from the environment on first Invoke.
func New() *Tool {
	return &Tool{
		NewClient: func() (atlassian.Client, error) {
			cfg, err := atlassian.LoadConfig()
			if err != nil {
				return nil, err
			}
			return atlassian.NewHTTPClient(cfg), nil
		},
	}
}

// Name returns the registry identifier.
func (t *Tool) Name() string { return "confluence.download-page" }

// Description is a one-line hint for MCP clients.
func (t *Tool) Description() string {
	return "Download a Confluence page (by ID or URL) and write it to disk " +
		"as an Obsidian-friendly markdown file. The page's property table, " +
		"if present, is restored as YAML frontmatter."
}

// InputSchema declares the tool's two arguments.
func (t *Tool) InputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"page": {
				Type: "string",
				Description: "Confluence page identifier: either a numeric ID " +
					"or a full page URL on the workspace configured via " +
					"ATLASSIAN_BASE_URL.",
			},
			"output_dir": {
				Type: "string",
				Description: "Directory in which to write the markdown file. " +
					"Defaults to " + DefaultOutputDir + " and must already exist.",
			},
		},
		Required: []string{"page"},
	}
}

// Result is what Invoke returns. JSON-marshals cleanly for MCP; the
// String() method drives the CLI single-line summary.
type Result struct {
	Action   string `json:"action"` // always "downloaded"
	PageID   string `json:"page_id"`
	Title    string `json:"title"`
	SpaceID  string `json:"space_id,omitempty"`
	WebURL   string `json:"web_url,omitempty"`
	FilePath string `json:"file_path"`
}

// String renders the result for the human-facing CLI.
func (r Result) String() string {
	return fmt.Sprintf("downloaded page %s: %s -> %s", r.PageID, r.Title, r.FilePath)
}

// pageURLRE extracts the numeric page ID from a Confluence v2 page URL.
// Matches both `/wiki/spaces/.../pages/<id>/...` and the rarer
// `/wiki/pages/<id>/...` shape.
var pageURLRE = regexp.MustCompile(`/pages/(\d+)(?:/|$)`)

// numericRE recognises a bare positive integer ID.
var numericRE = regexp.MustCompile(`^\d+$`)

// slugRE captures the runs of non-slug characters that get collapsed to a
// single dash by slugify.
var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lowercases s, collapses any run of non-`[a-z0-9]+` runes into a
// single `-`, and trims leading/trailing dashes. An empty result (e.g. a
// title made entirely of CJK characters) is signalled by returning "" so
// callers can substitute a fallback like the page ID.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugRE.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// requireString reads key from m as a non-empty string, or returns an
// error mentioning the key. The tool only accepts string-typed page and
// output_dir parameters; we deliberately do NOT coerce other types.
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

// optionalString reads key from m as a string, returning def when the key
// is absent. An explicit empty string is treated as "use the default" so
// MCP clients can leave the parameter out without special-casing.
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
