// Jira Cloud REST v3 issue operations implemented on top of HTTPClient.
// The IssueClient interface is the seam Jira tools depend on; HTTPClient
// is the production implementation that talks to Atlassian Cloud over
// Basic-auth-with-API-token. The Confluence-flavoured Client interface
// in client.go is intentionally separate so each tool only depends on
// the surface it actually needs.
package atlassian

import (
	"context"
	"encoding/json"
	"net/url"
)

// Issue mirrors the subset of Jira's v3 issue payload the tools consume.
// Fields not modelled here are simply dropped on unmarshal.
type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Fields IssueFields `json:"fields"`
}

// IssueFields carries the per-issue field bag returned by GET /issue.
// Description is left as json.RawMessage so the markdown converter can
// unmarshal it directly into a markdown.Document; an issue without a
// description gets a `null` raw value (length 4) which the renderer
// treats as an empty document.
type IssueFields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"`
	Status      IssueStatus     `json:"status"`
	IssueType   IssueType       `json:"issuetype"`
	// Parent is a pointer so absence (most issues) is distinguishable
	// from "present with empty key" — only the former should suppress
	// the jira_parent_ticket_key frontmatter entry on download.
	Parent  *IssueParent `json:"parent,omitempty"`
	Labels  []string     `json:"labels"`
	Created string       `json:"created"`
	Updated string       `json:"updated"`
}

// IssueStatus is the minimal shape of fields.status.
type IssueStatus struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// IssueType is the minimal shape of fields.issuetype.
type IssueType struct {
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

// IssueParent is the minimal shape of fields.parent used by the tools.
type IssueParent struct {
	Key string `json:"key"`
}

// Transition is one workflow transition available from the current
// status. The To field carries the post-transition status name.
type Transition struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	To   TransitionStep `json:"to"`
}

// TransitionStep is the target-status side of a Transition.
type TransitionStep struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// DevSummary is the best-effort PR/branch metadata pulled from Jira's
// undocumented /rest/dev-status endpoint. Missing data is represented
// by empty slices, never errors, so the fetch tool stays usable on
// tenants without Jira Software.
type DevSummary struct {
	PullRequests []string
	Branches     []string
}

// IssueClient is the seam Jira tools depend on. Each method targets a
// single REST endpoint; transactional behaviour (validate-then-mutate)
// lives in the tool layer so individual methods stay simple.
type IssueClient interface {
	GetIssue(ctx context.Context, key string) (Issue, error)
	UpdateIssue(ctx context.Context, key, summary, descADFJSON, issueType string) error
	GetTransitions(ctx context.Context, key string) ([]Transition, error)
	TransitionIssue(ctx context.Context, key, transitionID string) error
	GetDevSummary(ctx context.Context, issueID string) (DevSummary, error)
}

// GetIssue fetches the issue identified by key (e.g. "PROJ-123") with
// the field set the tools need. The expand=names parameter is omitted
// intentionally — we only key off field IDs that Jira ships by default.
func (c *HTTPClient) GetIssue(ctx context.Context, key string) (Issue, error) {
	var issue Issue
	path := "/rest/api/3/issue/" + url.PathEscape(key) +
		"?fields=summary,description,status,issuetype,parent,labels,created,updated"
	if err := c.do(ctx, "GET", path, nil, &issue); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

// UpdateIssue PUTs summary, description (as an ADF object), and — when
// issueType is non-empty — fields.issuetype.name in a single request.
// Passing issueType="" skips the type change so a no-op type stays out
// of the payload (avoids 4xx on tenants that restrict type editing).
// descADFJSON is embedded verbatim via json.RawMessage so the helper
// does not re-parse the converter's output.
func (c *HTTPClient) UpdateIssue(ctx context.Context, key, summary, descADFJSON, issueType string) error {
	fields := map[string]any{
		"summary":     summary,
		"description": json.RawMessage(descADFJSON),
	}
	if issueType != "" {
		fields["issuetype"] = map[string]string{"name": issueType}
	}
	payload := map[string]any{"fields": fields}
	return c.do(ctx, "PUT", "/rest/api/3/issue/"+url.PathEscape(key), payload, nil)
}

// GetTransitions lists the workflow transitions reachable from the
// issue's current status. The To.Name field is what tool callers
// compare jira_ticket_status against (case-insensitive).
func (c *HTTPClient) GetTransitions(ctx context.Context, key string) ([]Transition, error) {
	var resp struct {
		Transitions []Transition `json:"transitions"`
	}
	path := "/rest/api/3/issue/" + url.PathEscape(key) + "/transitions"
	if err := c.do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Transitions, nil
}

// TransitionIssue executes the given transition. transitionID must come
// from a prior GetTransitions call so the workflow accepts it; a wrong
// ID surfaces as an *APIError carrying the server's diagnostic body.
func (c *HTTPClient) TransitionIssue(ctx context.Context, key, transitionID string) error {
	payload := map[string]any{
		"transition": map[string]string{"id": transitionID},
	}
	path := "/rest/api/3/issue/" + url.PathEscape(key) + "/transitions"
	return c.do(ctx, "POST", path, payload, nil)
}

// GetDevSummary calls Jira's private /rest/dev-status/latest endpoint
// for PR and branch metadata. The endpoint is undocumented, gated on
// having Jira Software with a connected DevTools integration, and may
// return non-2xx for permission reasons. To keep the fetch tool
// resilient we swallow any error and return an empty DevSummary —
// callers are expected to render the empty case as a `[]` YAML
// sequence rather than skipping the keys.
func (c *HTTPClient) GetDevSummary(ctx context.Context, issueID string) (DevSummary, error) {
	prs, _ := c.devStatusURLs(ctx, issueID, "pullrequest")
	br, _ := c.devStatusURLs(ctx, issueID, "branch")
	return DevSummary{PullRequests: prs, Branches: br}, nil
}

// devStatusURLs is a thin helper around the two dev-status payload
// shapes (pull request / branch). See dev_status.go.
func (c *HTTPClient) devStatusURLs(ctx context.Context, issueID, dataType string) ([]string, error) {
	return c.fetchDevStatus(ctx, issueID, dataType)
}
