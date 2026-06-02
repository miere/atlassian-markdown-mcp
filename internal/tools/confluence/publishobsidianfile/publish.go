// Package publishobsidianfile implements `confluence.publish-obsidian-file`:
// it reads an Obsidian markdown file from disk and pushes its content to a
// Confluence page, fully rewriting the page body. The page is identified by
// YAML frontmatter keys on the source file:
//
//   - confluence_space and confluence_title — required on first publish so
//     the tool can resolve or create the page.
//   - confluence_page_id — written back on first publish; once present, it
//     is the sole identifier and confluence_space / confluence_title may be
//     omitted. The page's existing Confluence title is preserved on update.
package publishobsidianfile

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/markdown"
)

// Frontmatter keys that drive the sync, kept out of the published property
// table because they are sync metadata, not page content.
const (
	KeySpace  = "confluence_space"
	KeyTitle  = "confluence_title"
	KeyPageID = "confluence_page_id"
)

// Tool is the publish-obsidian-file capability. Both Converter and NewClient
// can be overridden by tests; New() wires them to the real implementations.
type Tool struct {
	Converter markdown.Converter
	NewClient func() (atlassian.Client, error)
}

// New constructs the tool with the production Converter and an Atlassian
// client that loads its config from the environment on first Invoke.
func New() *Tool {
	return &Tool{
		Converter: markdown.Default(),
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
func (t *Tool) Name() string { return "confluence.publish-obsidian-file" }

// Description is a one-line hint for MCP clients.
func (t *Tool) Description() string {
	return "Publish a local Obsidian markdown file to a Confluence page, " +
		"fully rewriting its body and preserving the file's YAML frontmatter " +
		"as a property table at the top of the page."
}

// InputSchema declares the single required argument.
func (t *Tool) InputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"file_path": {
				Type:        "string",
				Description: "Path to the Obsidian markdown file to publish.",
			},
		},
		Required: []string{"file_path"},
	}
}

// Result is what Invoke returns. It JSON-marshals cleanly for MCP and has a
// String() so the CLI renders a useful single-line summary.
type Result struct {
	Action  string `json:"action"` // "created" or "updated"
	PageID  string `json:"page_id"`
	Title   string `json:"title"`
	SpaceID string `json:"space_id,omitempty"`
	WebURL  string `json:"web_url,omitempty"`
}

// String renders the result for the human-facing CLI.
func (r Result) String() string {
	if r.WebURL != "" {
		return fmt.Sprintf("%s page %s: %s — %s", r.Action, r.PageID, r.Title, r.WebURL)
	}
	return fmt.Sprintf("%s page %s: %s", r.Action, r.PageID, r.Title)
}

// splitFrontmatter separates the sync-metadata keys (confluence_*) from the
// rest of the frontmatter. The second return is the map fed into the
// published property table.
func splitFrontmatter(fm map[string]any) (sync, props map[string]any) {
	sync = map[string]any{}
	props = map[string]any{}
	for k, v := range fm {
		switch k {
		case KeySpace, KeyTitle, KeyPageID:
			sync[k] = v
		default:
			props[k] = v
		}
	}
	return sync, props
}

// requireString returns the value at key as a non-empty string, or an error
// describing what is missing. Numbers (e.g. YAML-parsed page IDs) are
// stringified so the same helper covers KeyPageID.
func requireString(m map[string]any, key string) (string, error) {
	raw, ok := m[key]
	if !ok {
		return "", fmt.Errorf("frontmatter key %q is required", key)
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return "", fmt.Errorf("frontmatter key %q is empty", key)
		}
		return v, nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case float64:
		return fmt.Sprintf("%d", int64(v)), nil
	default:
		return "", fmt.Errorf("frontmatter key %q must be a string, got %T", key, raw)
	}
}

// optionalString returns the value at key as a string or "" if absent. Like
// requireString it accepts ints/floats (so YAML-parsed numeric page IDs work).
func optionalString(m map[string]any, key string) (string, error) {
	if _, ok := m[key]; !ok {
		return "", nil
	}
	return requireString(m, key)
}
