// Package updateticket implements `jira.update-ticket`: it reads an
// Obsidian markdown file produced by `jira.get-ticket`, validates the
// frontmatter, pushes the title, body, and (when changed) issue type
// in a single PUT, and — only if the local status differs from the
// live one — performs a workflow transition. Every validation runs
// before any mutation, so a typed error never leaves Jira in a
// half-written state. The parent key is treated as immutable: any
// drift between the local frontmatter and the live parent aborts the
// operation without touching Jira.
package updateticket

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/markdown"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/jira/getticket"
)

// Re-export the frontmatter key names so callers (and the schema
// description below) reference one source of truth.
const (
	KeyTicketKey    = getticket.KeyTicketKey
	KeyTicketTitle  = getticket.KeyTicketTitle
	KeyTicketStatus = getticket.KeyTicketStatus
	KeyTicketType   = getticket.KeyTicketType
	KeyParentKey    = getticket.KeyParentKey
)

// Tool is the update-ticket capability. Both Converter and NewClient
// can be overridden by tests; New() wires them to the real defaults.
type Tool struct {
	Converter markdown.Converter
	NewClient func() (atlassian.IssueClient, error)
}

// New constructs the tool with the production Converter and a Jira
// REST client that loads its config from the environment on first
// Invoke.
func New() *Tool {
	return &Tool{
		Converter: markdown.Default(),
		NewClient: func() (atlassian.IssueClient, error) {
			cfg, err := atlassian.LoadConfig()
			if err != nil {
				return nil, err
			}
			return atlassian.NewHTTPClient(cfg), nil
		},
	}
}

// Name returns the registry identifier.
func (t *Tool) Name() string { return "jira.update-ticket" }

// Description is a one-line hint for MCP clients.
func (t *Tool) Description() string {
	return "Update a Jira ticket from a local Obsidian markdown file. " +
		"The frontmatter drives the sync (key/title/status/type); the " +
		"parent key is treated as immutable and any drift aborts the call."
}

// InputSchema declares the single required argument.
func (t *Tool) InputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"file_path": {
				Type:        "string",
				Description: "Path to the Obsidian markdown file to push.",
			},
		},
		Required: []string{"file_path"},
	}
}

// Result is what Invoke returns. JSON-marshals cleanly for MCP and
// carries a String() for the CLI single-line summary.
type Result struct {
	Action       string `json:"action"` // always "updated"
	Key          string `json:"key"`
	Title        string `json:"title"`
	FromStatus   string `json:"from_status"`
	ToStatus     string `json:"to_status"`
	Transitioned bool   `json:"transitioned"`
	WebURL       string `json:"web_url,omitempty"`
}

// String renders the result for the human-facing CLI. The status
// transition is only mentioned when it actually happened.
func (r Result) String() string {
	if r.Transitioned {
		return fmt.Sprintf("updated ticket %s: %s [%s -> %s]",
			r.Key, r.Title, r.FromStatus, r.ToStatus)
	}
	return fmt.Sprintf("updated ticket %s: %s", r.Key, r.Title)
}

// requireFrontmatterString returns the value at key as a non-empty
// string, or a typed error naming the missing key. Numeric YAML
// values (rare but possible when a user pastes a key like "1234")
// are stringified so the same helper covers all four required keys.
func requireFrontmatterString(m map[string]any, key string) (string, error) {
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

// optionalFrontmatterString returns the value at key as a string, or
// "" when the key is absent. Empty string and missing key are treated
// identically — both mean "no local parent" for KeyParentKey.
func optionalFrontmatterString(m map[string]any, key string) (string, error) {
	if _, ok := m[key]; !ok {
		return "", nil
	}
	s, err := requireFrontmatterString(m, key)
	if err != nil {
		// An empty value is a "no value" signal here, not an error.
		if fmt.Sprintf("frontmatter key %q is empty", key) == err.Error() {
			return "", nil
		}
		return "", err
	}
	return s, nil
}
