package getticket

import (
	"fmt"
	"os"
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
)

// atomicWrite stages content in a temp file inside outDir and renames
// it over dest, so a crash mid-write never leaves a truncated file.
// The temp file is removed on any failure path so we do not litter
// the output directory with `.get-ticket-*.tmp` droppings.
func atomicWrite(outDir, dest, content string) (string, error) {
	tmp, err := os.CreateTemp(outDir, ".get-ticket-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file in %s: %w", outDir, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename %s -> %s: %w", tmpPath, dest, err)
	}
	return dest, nil
}

// baseURLOf returns the configured Atlassian base URL for clients
// that expose one (the production HTTPClient), or "" otherwise. The
// fetch tool uses it both for URL host validation and to assemble the
// browse URL stored on the Result.
func baseURLOf(client atlassian.IssueClient) string {
	if hc, ok := client.(interface{ BaseURL() string }); ok {
		return hc.BaseURL()
	}
	return ""
}

// browseURL returns the absolute /browse/<KEY> URL for the issue, or
// "" when no base URL is configured (which is only the case in tests
// using fake clients without the BaseURL seam).
func browseURL(baseURL, key string) string {
	if baseURL == "" || key == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/browse/" + key
}

// resultFrom assembles the Result struct returned by Invoke.
func resultFrom(issue atlassian.Issue, dest string, client atlassian.IssueClient) Result {
	return Result{
		Action:   "downloaded",
		Key:      issue.Key,
		Title:    issue.Fields.Summary,
		Status:   issue.Fields.Status.Name,
		WebURL:   browseURL(baseURLOf(client), issue.Key),
		FilePath: dest,
	}
}
