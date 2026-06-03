// Package getticket implements `jira.get-ticket`: it fetches a Jira
// issue (by key or browse URL), converts its ADF description to
// markdown, and writes the result to disk as an Obsidian-friendly note.
// Sync metadata — key, title, status, type, optional parent, dates,
// labels, and a best-effort DevTools PR/branch list — lives in the
// YAML frontmatter at the top of the file. The file name is
// `<KEY> - <title>.md`; see download.go's slug helpers for the
// sanitisation rules. The companion `jira.update-ticket` tool reads
// the same frontmatter to push edits back.
package getticket

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
)

// Frontmatter keys are exported so the update tool can reference the
// exact same names without re-typing the strings.
const (
	KeyTicketKey    = "jira_ticket_key"
	KeyTicketTitle  = "jira_ticket_title"
	KeyTicketStatus = "jira_ticket_status"
	KeyTicketType   = "jira_ticket_type"
	KeyParentKey    = "jira_parent_ticket_key"
	KeyCreated      = "jira_created"
	KeyUpdated      = "jira_updated"
	KeyLabels       = "jira_labels"
	KeyPullRequests = "jira_pull_requests"
	KeyBranches     = "jira_branches"
)

// DefaultOutputDir matches the Confluence download tool so MCP clients
// see one consistent default across the namespace.
const DefaultOutputDir = "/tmp/"

// Tool is the get-ticket capability. NewClient is overridden by tests
// to substitute a fake atlassian.IssueClient.
type Tool struct {
	NewClient func() (atlassian.IssueClient, error)
}

// New constructs the tool with a Jira REST client that loads its
// config from the environment on first Invoke.
func New() *Tool {
	return &Tool{
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
func (t *Tool) Name() string { return "jira.get-ticket" }

// Description is a one-line hint for MCP clients.
func (t *Tool) Description() string {
	return "Download a Jira ticket (by key or browse URL) and write it to " +
		"disk as an Obsidian-friendly markdown file with sync metadata in " +
		"the YAML frontmatter."
}

// InputSchema declares the tool's two arguments.
func (t *Tool) InputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"ticket": {
				Type: "string",
				Description: "Jira ticket identifier: either an issue key " +
					"(e.g. PROJ-123) or a full browse URL on the workspace " +
					"configured via ATLASSIAN_BASE_URL.",
			},
			"output_dir": {
				Type: "string",
				Description: "Directory in which to write the markdown file. " +
					"Defaults to " + DefaultOutputDir + " and must already exist.",
			},
		},
		Required: []string{"ticket"},
	}
}

// Result is what Invoke returns. JSON-marshals cleanly for MCP; the
// String() method drives the CLI single-line summary.
type Result struct {
	Action   string `json:"action"` // always "downloaded"
	Key      string `json:"key"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	WebURL   string `json:"web_url,omitempty"`
	FilePath string `json:"file_path"`
}

// String renders the result for the human-facing CLI.
func (r Result) String() string {
	return fmt.Sprintf("downloaded ticket %s [%s]: %s -> %s",
		r.Key, r.Status, r.Title, r.FilePath)
}


