package updateticket

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
)

// resolveTransition returns the transition ID required to move the
// live issue to local.status, or "" when no transition is needed
// (status unchanged, case-insensitive match against the live name).
// A status the workflow cannot reach is an error — the lookup runs
// before the PUT so an invalid target never produces a half-written
// ticket.
func resolveTransition(
	ctx context.Context,
	client atlassian.IssueClient,
	local localFrontmatter,
	issue atlassian.Issue,
) (string, error) {
	if strings.EqualFold(local.status, issue.Fields.Status.Name) {
		return "", nil
	}
	transitions, err := client.GetTransitions(ctx, local.key)
	if err != nil {
		return "", err
	}
	for _, tr := range transitions {
		if strings.EqualFold(tr.To.Name, local.status) {
			return tr.ID, nil
		}
	}
	return "", fmt.Errorf(
		"jira.update-ticket: status %q is not reachable from %q (available: %s)",
		local.status, issue.Fields.Status.Name, formatTargets(transitions))
}

// formatTargets renders the target statuses of the available
// transitions as a sorted, comma-separated list, dropping duplicates.
// Sorting keeps error messages deterministic across runs even when
// Jira returns transitions in workflow-defined order.
func formatTargets(transitions []atlassian.Transition) string {
	if len(transitions) == 0 {
		return "(no transitions available)"
	}
	seen := map[string]struct{}{}
	names := make([]string, 0, len(transitions))
	for _, tr := range transitions {
		if tr.To.Name == "" {
			continue
		}
		if _, dup := seen[tr.To.Name]; dup {
			continue
		}
		seen[tr.To.Name] = struct{}{}
		names = append(names, tr.To.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// resultFrom assembles the Result struct from the local frontmatter
// and the pre-update live issue. transitioned reflects whether the
// caller actually issued a transition; the from/to fields are
// populated either way so the caller can log the no-op vs. moved
// distinction.
func resultFrom(
	local localFrontmatter,
	issue atlassian.Issue,
	transitioned bool,
	client atlassian.IssueClient,
) Result {
	return Result{
		Action:       "updated",
		Key:          local.key,
		Title:        local.title,
		FromStatus:   issue.Fields.Status.Name,
		ToStatus:     local.status,
		Transitioned: transitioned,
		WebURL:       browseURL(baseURLOf(client), local.key),
	}
}

// baseURLOf returns the configured Atlassian base URL for clients
// that expose one (the production HTTPClient), or "" otherwise.
func baseURLOf(client atlassian.IssueClient) string {
	if hc, ok := client.(interface{ BaseURL() string }); ok {
		return hc.BaseURL()
	}
	return ""
}

// browseURL returns the absolute /browse/<KEY> URL for the issue, or
// "" when no base URL is configured (tests with bare fakes).
func browseURL(baseURL, key string) string {
	if baseURL == "" || key == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/browse/" + key
}
