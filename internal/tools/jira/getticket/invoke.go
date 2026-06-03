package getticket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/markdown"
)

// Invoke resolves the ticket identifier to a Jira issue key, fetches
// the issue, converts its description from ADF to markdown, gathers
// best-effort DevTools metadata, and writes the result to disk under
// output_dir.
func (t *Tool) Invoke(ctx context.Context, args map[string]any) (any, error) {
	ticketArg, err := requireString(args, "ticket")
	if err != nil {
		return nil, err
	}
	outDir, err := optionalString(args, "output_dir", DefaultOutputDir)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(outDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("output_dir %q must be an existing directory", outDir)
	}
	client, err := t.NewClient()
	if err != nil {
		return nil, err
	}
	key, err := resolveTicketKey(ticketArg, baseURLOf(client))
	if err != nil {
		return nil, err
	}
	issue, err := client.GetIssue(ctx, key)
	if err != nil {
		return nil, err
	}
	dev, _ := client.GetDevSummary(ctx, issue.ID)
	mdBody, err := renderDescription(issue)
	if err != nil {
		return nil, err
	}
	fm := frontmatterFrom(issue, dev)
	dest, err := writeMarkdownFile(outDir, issue, fm, mdBody)
	if err != nil {
		return nil, err
	}
	return resultFrom(issue, dest, client), nil
}

// resolveTicketKey accepts either a bare issue key or a Jira browse
// URL. URL inputs are validated against baseURL (the configured
// workspace) so the tool refuses to leak credentials to a different
// tenant. baseURL may be empty when the client does not expose one
// (only the production HTTPClient does); in that case the host check
// is skipped — tests rely on this.
func resolveTicketKey(input, baseURL string) (string, error) {
	input = strings.TrimSpace(input)
	if keyRE.MatchString(input) {
		return input, nil
	}
	u, err := url.Parse(input)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("ticket %q is neither a Jira key nor a parseable URL", input)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("ticket URL %q must use http(s)", input)
	}
	if baseURL != "" {
		bu, err := url.Parse(baseURL)
		if err == nil && bu.Host != "" && !strings.EqualFold(u.Host, bu.Host) {
			return "", fmt.Errorf("ticket URL host %q does not match configured ATLASSIAN_BASE_URL host %q",
				u.Host, bu.Host)
		}
	}
	m := browseURLRE.FindStringSubmatch(u.Path)
	if len(m) < 2 {
		return "", fmt.Errorf("ticket URL %q does not contain a /browse/<KEY> segment", input)
	}
	return m[1], nil
}

// renderDescription unmarshals the issue's ADF description into a
// markdown.Document and renders it. A `null`/missing description (the
// raw value is literal "null" or empty) yields an empty string so the
// downstream writer just emits frontmatter followed by a blank body.
func renderDescription(issue atlassian.Issue) (string, error) {
	raw := bytesOrNull(issue.Fields.Description)
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var doc markdown.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("parse ADF description for %s: %w", issue.Key, err)
	}
	return markdown.RenderMarkdown(&doc), nil
}

// bytesOrNull normalises json.RawMessage: nil and empty slices map to
// nil so the caller's len-based fast paths apply.
func bytesOrNull(b json.RawMessage) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}

// frontmatterFrom builds the YAML metadata block from the issue and
// the (possibly empty) DevTools summary. labels/PRs/branches are
// normalised to non-nil slices so the writer always emits the keys,
// even on tickets with no labels and no dev-status data.
func frontmatterFrom(issue atlassian.Issue, dev atlassian.DevSummary) frontmatter {
	fm := frontmatter{
		Key:         issue.Key,
		Title:       issue.Fields.Summary,
		Status:      issue.Fields.Status.Name,
		Type:        issue.Fields.IssueType.Name,
		Created:     issue.Fields.Created,
		Updated:     issue.Fields.Updated,
		Labels:      nonNil(issue.Fields.Labels),
		PRURLs:      nonNil(dev.PullRequests),
		BranchNames: nonNil(dev.Branches),
	}
	if issue.Fields.Parent != nil {
		fm.ParentKey = issue.Fields.Parent.Key
	}
	return fm
}

// nonNil returns an empty slice when xs is nil so the YAML writer
// can emit `[]` rather than skipping the key entirely.
func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

// writeMarkdownFile atomically writes the frontmatter+body to
// <outDir>/<KEY> - <sanitised title>.md. Falls back to <KEY>.md when
// the sanitised title is empty (CJK-only titles, etc.). Existing
// destinations are overwritten via os.Rename.
func writeMarkdownFile(outDir string, issue atlassian.Issue, fm frontmatter, body string) (string, error) {
	name := issue.Key
	if t := sanitiseTitle(issue.Fields.Summary); t != "" {
		name = issue.Key + " - " + t
	}
	dest := filepath.Join(outDir, name+".md")
	content := fm.render() + body
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return atomicWrite(outDir, dest, content)
}
