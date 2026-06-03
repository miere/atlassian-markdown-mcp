package getticket

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmason/mdadf"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
)

// fakeIssueClient is the minimal atlassian.IssueClient the fetch tool
// uses. Counters let tests assert that rejected inputs short-circuit
// before any HTTP call would happen.
type fakeIssueClient struct {
	issue       atlassian.Issue
	getCalls    int
	gotKey      string
	dev         atlassian.DevSummary
	devCalls    int
	devErr      error
	updateCalls int
}

func (f *fakeIssueClient) GetIssue(_ context.Context, key string) (atlassian.Issue, error) {
	f.getCalls++
	f.gotKey = key
	return f.issue, nil
}
func (f *fakeIssueClient) UpdateIssue(context.Context, string, string, string, string) error {
	f.updateCalls++
	return nil
}
func (f *fakeIssueClient) GetTransitions(context.Context, string) ([]atlassian.Transition, error) {
	return nil, nil
}
func (f *fakeIssueClient) TransitionIssue(context.Context, string, string) error { return nil }
func (f *fakeIssueClient) GetDevSummary(context.Context, string) (atlassian.DevSummary, error) {
	f.devCalls++
	return f.dev, f.devErr
}

// fakeIssueClientWithBase satisfies the optional BaseURL() seam the
// fetch tool uses for host validation and browse-URL assembly.
type fakeIssueClientWithBase struct {
	*fakeIssueClient
	base string
}

func (f *fakeIssueClientWithBase) BaseURL() string { return f.base }

// newToolWith builds a Tool wired to the given fake client.
func newToolWith(client atlassian.IssueClient) *Tool {
	return &Tool{NewClient: func() (atlassian.IssueClient, error) { return client, nil }}
}

// adfDescription marshals an ADF document with the given nodes for
// embedding in atlassian.IssueFields.Description.
func adfDescription(t *testing.T, nodes ...mdadf.Node) json.RawMessage {
	t.Helper()
	doc := &mdadf.Document{Version: 1, Type: "doc", Content: nodes}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal adf: %v", err)
	}
	return b
}

// issueWithParent is a convenience constructor for the headline test.
func issueWithParent() atlassian.Issue {
	return atlassian.Issue{
		ID:  "10042",
		Key: "PROJ-123",
		Fields: atlassian.IssueFields{
			Summary:   "Fix login bug",
			Status:    atlassian.IssueStatus{Name: "In Progress", ID: "3"},
			IssueType: atlassian.IssueType{Name: "Bug"},
			Parent:    &atlassian.IssueParent{Key: "PROJ-100"},
			Labels:    []string{"backend", "hotfix"},
			Created:   "2026-05-30T08:15:42+00:00",
			Updated:   "2026-06-01T11:02:07+00:00",
		},
	}
}

// TestInvoke_DownloadsTicketWithFrontmatter covers the headline flow:
// numeric key + ADF description + DevTools summary -> file is written
// with the exact frontmatter ordering and the rendered markdown body.
func TestInvoke_DownloadsTicketWithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeIssueClient{issue: issueWithParent()}
	fc.issue.Fields.Description = adfDescription(t,
		mdadf.HeadingNode(1, mdadf.TextNode("Hello")),
		mdadf.ParagraphNode(mdadf.TextNode("World.")),
	)
	fc.dev = atlassian.DevSummary{
		PullRequests: []string{"https://github.com/acme/app/pull/42"},
		Branches:     []string{"feat-fix-login"},
	}
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"ticket": "PROJ-123", "output_dir": dir,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	r := res.(Result)
	want := filepath.Join(dir, "PROJ-123 - Fix login bug.md")
	if r.Action != "downloaded" || r.Key != "PROJ-123" || r.FilePath != want {
		t.Errorf("Result = %+v", r)
	}
	got, _ := os.ReadFile(r.FilePath)
	expected := "---\n" +
		"jira_ticket_key: PROJ-123\n" +
		"jira_ticket_title: Fix login bug\n" +
		"jira_ticket_status: In Progress\n" +
		"jira_ticket_type: Bug\n" +
		"jira_parent_ticket_key: PROJ-100\n" +
		"jira_created: \"2026-05-30T08:15:42+00:00\"\n" +
		"jira_updated: \"2026-06-01T11:02:07+00:00\"\n" +
		"jira_labels: [backend, hotfix]\n" +
		"jira_pull_requests: [\"https://github.com/acme/app/pull/42\"]\n" +
		"jira_branches: [feat-fix-login]\n" +
		"---\n" +
		"# Hello\n\nWorld.\n"
	if string(got) != expected {
		t.Errorf("file content mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, expected)
	}
}

// TestInvoke_OmitsParentKeyWhenAbsent ensures non-subtask tickets do
// not get an empty parent line on disk.
func TestInvoke_OmitsParentKeyWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	issue := issueWithParent()
	issue.Fields.Parent = nil
	fc := &fakeIssueClient{issue: issue}
	fc.issue.Fields.Description = adfDescription(t, mdadf.ParagraphNode(mdadf.TextNode("x")))
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"ticket": "PROJ-123", "output_dir": dir,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := os.ReadFile(res.(Result).FilePath)
	if strings.Contains(string(got), KeyParentKey) {
		t.Errorf("file unexpectedly carries parent key:\n%s", got)
	}
}

// Sentinel error used by TestInvoke_DevSummaryFailureLeavesEmptyLists.
var errDev = errors.New("boom")

// TestInvoke_AcceptsURLAndValidatesHost confirms a /browse/<KEY> URL
// resolves to the embedded key and that a mismatched host aborts
// before GetIssue is called.
func TestInvoke_AcceptsURLAndValidatesHost(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeIssueClient{issue: issueWithParent()}
	fc.issue.Fields.Description = adfDescription(t, mdadf.ParagraphNode(mdadf.TextNode("x")))
	client := &fakeIssueClientWithBase{fakeIssueClient: fc, base: "https://acme.atlassian.net"}
	if _, err := newToolWith(client).Invoke(context.Background(), map[string]any{
		"ticket": "https://acme.atlassian.net/browse/PROJ-123", "output_dir": dir,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if fc.gotKey != "PROJ-123" {
		t.Errorf("gotKey = %q, want PROJ-123", fc.gotKey)
	}
	prev := fc.getCalls
	_, err := newToolWith(client).Invoke(context.Background(), map[string]any{
		"ticket": "https://other.atlassian.net/browse/PROJ-123", "output_dir": dir,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match configured") {
		t.Errorf("expected host-mismatch error, got %v", err)
	}
	if fc.getCalls != prev {
		t.Errorf("GetIssue called despite host mismatch (%d -> %d)", prev, fc.getCalls)
	}
}

// TestInvoke_ErrorsWhenOutputDirMissing surfaces a typed error and
// does not call the client when the destination directory is absent.
func TestInvoke_ErrorsWhenOutputDirMissing(t *testing.T) {
	fc := &fakeIssueClient{}
	_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"ticket": "PROJ-1", "output_dir": filepath.Join(t.TempDir(), "nope"),
	})
	if err == nil || !strings.Contains(err.Error(), "must be an existing directory") {
		t.Errorf("expected output_dir error, got %v", err)
	}
	if fc.getCalls != 0 {
		t.Errorf("GetIssue called despite missing output_dir")
	}
}

// TestInvoke_TitleFallsBackToKeyForNonASCII exercises the fallback
// branch in writeMarkdownFile when sanitiseTitle yields an empty
// string after stripping.
func TestInvoke_TitleFallsBackToKeyForNonASCII(t *testing.T) {
	dir := t.TempDir()
	issue := issueWithParent()
	issue.Fields.Summary = "  /  "
	fc := &fakeIssueClient{issue: issue}
	fc.issue.Fields.Description = adfDescription(t, mdadf.ParagraphNode(mdadf.TextNode("x")))
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"ticket": "PROJ-123", "output_dir": dir,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.(Result).FilePath != filepath.Join(dir, "PROJ-123.md") {
		t.Errorf("FilePath = %q, want fallback to <KEY>.md", res.(Result).FilePath)
	}
}

// TestInvoke_DevSummaryFailureLeavesEmptyLists ensures a DevTools
// error does not abort the fetch — frontmatter still carries the
// PR and branch keys with empty `[]` values.
func TestInvoke_DevSummaryFailureLeavesEmptyLists(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeIssueClient{issue: issueWithParent(), devErr: errDev}
	fc.issue.Fields.Description = adfDescription(t, mdadf.ParagraphNode(mdadf.TextNode("x")))
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"ticket": "PROJ-123", "output_dir": dir,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	body, _ := os.ReadFile(res.(Result).FilePath)
	if !strings.Contains(string(body), "jira_pull_requests: []") {
		t.Errorf("expected empty PR list in frontmatter:\n%s", body)
	}
	if !strings.Contains(string(body), "jira_branches: []") {
		t.Errorf("expected empty branch list in frontmatter:\n%s", body)
	}
}

// TestInvoke_RejectsMalformedTicket validates the input parser before
// any client work.
func TestInvoke_RejectsMalformedTicket(t *testing.T) {
	fc := &fakeIssueClient{}
	_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"ticket": "not-a-ticket", "output_dir": t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "neither a Jira key nor a parseable URL") {
		t.Errorf("expected parser error, got %v", err)
	}
	if fc.getCalls != 0 {
		t.Errorf("GetIssue called despite invalid ticket")
	}
}
