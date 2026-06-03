package updateticket

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/markdown"
)

// fakeIssueClient is the in-package fake for atlassian.IssueClient.
// Counters let tests assert that early-abort paths never reach the
// PUT or POST.
type fakeIssueClient struct {
	issue         atlassian.Issue
	transitions   []atlassian.Transition
	transErr      error
	updateErr     error
	getCalls      int
	updateCalls   int
	transListCalls int
	transRunCalls  int
	updatedSummary string
	updatedDesc    string
	updatedType    string
	transitionID   string
}

func (f *fakeIssueClient) GetIssue(context.Context, string) (atlassian.Issue, error) {
	f.getCalls++
	return f.issue, nil
}

func (f *fakeIssueClient) UpdateIssue(_ context.Context, _, summary, desc, issueType string) error {
	f.updateCalls++
	f.updatedSummary = summary
	f.updatedDesc = desc
	f.updatedType = issueType
	return f.updateErr
}

func (f *fakeIssueClient) GetTransitions(context.Context, string) ([]atlassian.Transition, error) {
	f.transListCalls++
	return f.transitions, f.transErr
}

func (f *fakeIssueClient) TransitionIssue(_ context.Context, _, id string) error {
	f.transRunCalls++
	f.transitionID = id
	return nil
}

func (f *fakeIssueClient) GetDevSummary(context.Context, string) (atlassian.DevSummary, error) {
	return atlassian.DevSummary{}, nil
}

// newToolWith builds a Tool wired to the given fake client. The
// Converter is the package default so tests exercise the real
// markdown→ADF path.
func newToolWith(client atlassian.IssueClient) *Tool {
	return &Tool{
		Converter: markdown.Default(),
		NewClient: func() (atlassian.IssueClient, error) { return client, nil },
	}
}

// writeFile creates a temp markdown file with the given frontmatter
// and body. Returns its path so tests can hand it straight to Invoke.
func writeFile(t *testing.T, fm, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ticket.md")
	content := "---\n" + fm + "---\n" + body
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// baseFM is the minimum frontmatter every test needs.
const baseFM = `jira_ticket_key: PROJ-123
jira_ticket_title: Fix login bug
jira_ticket_status: In Progress
jira_ticket_type: Bug
`

// liveIssue is the live state the fake returns by default.
func liveIssue() atlassian.Issue {
	return atlassian.Issue{
		ID:  "10042",
		Key: "PROJ-123",
		Fields: atlassian.IssueFields{
			Summary:   "Fix login bug",
			Status:    atlassian.IssueStatus{Name: "In Progress"},
			IssueType: atlassian.IssueType{Name: "Bug"},
		},
	}
}

// TestInvoke_HappyPathSameStatusAndType pushes title+description in
// one PUT and does not consult the transitions endpoint.
func TestInvoke_HappyPathSameStatusAndType(t *testing.T) {
	path := writeFile(t, baseFM, "Updated body.\n")
	fc := &fakeIssueClient{issue: liveIssue()}
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	r := res.(Result)
	if r.Transitioned || fc.transListCalls != 0 || fc.transRunCalls != 0 {
		t.Errorf("expected no transition, got %+v (list=%d, run=%d)", r, fc.transListCalls, fc.transRunCalls)
	}
	if fc.updateCalls != 1 || fc.updatedSummary != "Fix login bug" || fc.updatedType != "" {
		t.Errorf("PUT mismatch: calls=%d summary=%q type=%q", fc.updateCalls, fc.updatedSummary, fc.updatedType)
	}
	if !strings.Contains(fc.updatedDesc, "Updated") || !strings.Contains(fc.updatedDesc, "body.") {
		t.Errorf("description does not carry the body text: %s", fc.updatedDesc)
	}
}


// TestInvoke_StatusChangeRunsTransition resolves a reachable target
// status and POSTs the transition after the PUT.
func TestInvoke_StatusChangeRunsTransition(t *testing.T) {
	fm := strings.Replace(baseFM, "jira_ticket_status: In Progress\n",
		"jira_ticket_status: Done\n", 1)
	path := writeFile(t, fm, "body\n")
	fc := &fakeIssueClient{
		issue: liveIssue(),
		transitions: []atlassian.Transition{
			{ID: "31", Name: "Resolve", To: atlassian.TransitionStep{Name: "Done"}},
		},
	}
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !res.(Result).Transitioned || fc.transitionID != "31" {
		t.Errorf("expected transition 31, got %+v (id=%q)", res, fc.transitionID)
	}
	if fc.updateCalls != 1 || fc.transRunCalls != 1 {
		t.Errorf("expected one PUT then one transition, got update=%d run=%d", fc.updateCalls, fc.transRunCalls)
	}
}

// TestInvoke_TypeChangeIncludedInPUT pushes a new issue type in the
// same PUT and does not transition (status unchanged).
func TestInvoke_TypeChangeIncludedInPUT(t *testing.T) {
	fm := strings.Replace(baseFM, "jira_ticket_type: Bug\n",
		"jira_ticket_type: Story\n", 1)
	path := writeFile(t, fm, "body\n")
	fc := &fakeIssueClient{issue: liveIssue()}
	if _, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if fc.updatedType != "Story" {
		t.Errorf("updated type = %q, want Story", fc.updatedType)
	}
	if fc.transListCalls != 0 || fc.transRunCalls != 0 {
		t.Errorf("transitions endpoint unexpectedly hit (list=%d, run=%d)", fc.transListCalls, fc.transRunCalls)
	}
}

// TestInvoke_InvalidStatusAbortsBeforePUT covers the headline
// fail-fast guarantee: an unreachable status raises a typed error
// and the PUT is never sent.
func TestInvoke_InvalidStatusAbortsBeforePUT(t *testing.T) {
	fm := strings.Replace(baseFM, "jira_ticket_status: In Progress\n",
		"jira_ticket_status: Cancelled\n", 1)
	path := writeFile(t, fm, "body\n")
	fc := &fakeIssueClient{
		issue: liveIssue(),
		transitions: []atlassian.Transition{
			{ID: "31", Name: "Resolve", To: atlassian.TransitionStep{Name: "Done"}},
			{ID: "41", Name: "Reopen", To: atlassian.TransitionStep{Name: "To Do"}},
		},
	}
	_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err == nil || !strings.Contains(err.Error(), "is not reachable from") {
		t.Errorf("expected unreachable-status error, got %v", err)
	}
	if !strings.Contains(err.Error(), "Done") || !strings.Contains(err.Error(), "To Do") {
		t.Errorf("error should list available targets: %v", err)
	}
	if fc.updateCalls != 0 || fc.transRunCalls != 0 {
		t.Errorf("PUT/transition fired despite invalid status: update=%d, run=%d", fc.updateCalls, fc.transRunCalls)
	}
}

// TestInvoke_ParentMismatchAbortsBeforePUT covers the three parent
// drift cases. In each, the file is touched, the issue is fetched,
// but no PUT and no transition runs.
func TestInvoke_ParentMismatchAbortsBeforePUT(t *testing.T) {
	t.Run("local present, live absent", func(t *testing.T) {
		fm := baseFM + "jira_parent_ticket_key: PROJ-100\n"
		runParentMismatch(t, fm, liveIssue(), "is not set on live issue")
	})
	t.Run("local absent, live present", func(t *testing.T) {
		issue := liveIssue()
		issue.Fields.Parent = &atlassian.IssueParent{Key: "PROJ-99"}
		runParentMismatch(t, baseFM, issue, "frontmatter omits it")
	})
	t.Run("both present but different", func(t *testing.T) {
		fm := baseFM + "jira_parent_ticket_key: PROJ-100\n"
		issue := liveIssue()
		issue.Fields.Parent = &atlassian.IssueParent{Key: "PROJ-99"}
		runParentMismatch(t, fm, issue, "does not match live parent")
	})
}

// runParentMismatch is the shared body for the three parent-drift
// subtests above.
func runParentMismatch(t *testing.T, fm string, issue atlassian.Issue, wantErr string) {
	t.Helper()
	path := writeFile(t, fm, "body\n")
	fc := &fakeIssueClient{issue: issue}
	_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("expected parent-drift error %q, got %v", wantErr, err)
	}
	if fc.updateCalls != 0 || fc.transRunCalls != 0 {
		t.Errorf("PUT/transition fired despite parent mismatch: update=%d, run=%d", fc.updateCalls, fc.transRunCalls)
	}
}

// TestInvoke_MissingRequiredFrontmatter rejects each of the four
// mandatory keys with a typed error, before any client call.
func TestInvoke_MissingRequiredFrontmatter(t *testing.T) {
	cases := []struct {
		name, drop string
	}{
		{"key", "jira_ticket_key: PROJ-123\n"},
		{"title", "jira_ticket_title: Fix login bug\n"},
		{"status", "jira_ticket_status: In Progress\n"},
		{"type", "jira_ticket_type: Bug\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fm := strings.Replace(baseFM, c.drop, "", 1)
			path := writeFile(t, fm, "body\n")
			fc := &fakeIssueClient{issue: liveIssue()}
			_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
			if err == nil || !strings.Contains(err.Error(), "is required") {
				t.Errorf("expected required-key error, got %v", err)
			}
			if fc.getCalls != 0 || fc.updateCalls != 0 {
				t.Errorf("client called despite missing frontmatter (get=%d, update=%d)", fc.getCalls, fc.updateCalls)
			}
		})
	}
}

// failingConverter satisfies markdown.Converter and always errors,
// so we can drive the markdown→ADF fail-fast branch deterministically.
type failingConverter struct{ err error }

func (f failingConverter) Convert([]byte) (*markdown.Document, error) { return nil, f.err }

// TestInvoke_MarkdownConversionErrorAbortsBeforeFetch ensures a
// malformed body never reaches the client.
func TestInvoke_MarkdownConversionErrorAbortsBeforeFetch(t *testing.T) {
	path := writeFile(t, baseFM, "body\n")
	fc := &fakeIssueClient{issue: liveIssue()}
	tool := &Tool{
		Converter: failingConverter{err: errors.New("bad markdown")},
		NewClient: func() (atlassian.IssueClient, error) { return fc, nil },
	}
	_, err := tool.Invoke(context.Background(), map[string]any{"file_path": path})
	if err == nil || !strings.Contains(err.Error(), "bad markdown") {
		t.Errorf("expected converter error, got %v", err)
	}
	if fc.getCalls != 0 || fc.updateCalls != 0 {
		t.Errorf("client called despite converter failure (get=%d, update=%d)", fc.getCalls, fc.updateCalls)
	}
}

// TestInvoke_ServerRejectedTypeChangeSkipsTransition ensures a 4xx on
// the PUT short-circuits before the transition POST.
func TestInvoke_ServerRejectedTypeChangeSkipsTransition(t *testing.T) {
	fm := strings.Replace(baseFM, "jira_ticket_status: In Progress\n",
		"jira_ticket_status: Done\n", 1)
	fm = strings.Replace(fm, "jira_ticket_type: Bug\n", "jira_ticket_type: Story\n", 1)
	path := writeFile(t, fm, "body\n")
	fc := &fakeIssueClient{
		issue:     liveIssue(),
		updateErr: &atlassian.APIError{Method: "PUT", URL: "/rest/api/3/issue/PROJ-123", StatusCode: 400, Body: "issue type Story not allowed"},
		transitions: []atlassian.Transition{
			{ID: "31", Name: "Resolve", To: atlassian.TransitionStep{Name: "Done"}},
		},
	}
	_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	var apiErr *atlassian.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		t.Errorf("expected APIError 400, got %v", err)
	}
	if fc.transRunCalls != 0 {
		t.Errorf("transition POST should not run when PUT fails (got %d)", fc.transRunCalls)
	}
}

// TestInvoke_PUTBodyShape spot-checks the ADF JSON that lands on
// UpdateIssue is a doc-shaped object the API would accept verbatim.
func TestInvoke_PUTBodyShape(t *testing.T) {
	path := writeFile(t, baseFM, "Some **bold** text.\n")
	fc := &fakeIssueClient{issue: liveIssue()}
	if _, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(fc.updatedDesc), &doc); err != nil {
		t.Fatalf("description is not valid JSON: %v", err)
	}
	if doc["type"] != "doc" || doc["version"] != float64(1) {
		t.Errorf("ADF doc envelope wrong: %v", doc)
	}
}

