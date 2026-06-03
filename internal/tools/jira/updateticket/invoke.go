package updateticket

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/obsidian"
)

// Invoke loads the file, validates the frontmatter, converts the
// markdown body to ADF, fetches the live issue, enforces parent
// immutability, resolves any pending status transition, PUTs the
// changes, and finally executes the transition (if any). Every
// validation runs before any HTTP mutation, so an error from an
// intermediate step never leaves Jira partially updated.
func (t *Tool) Invoke(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args["file_path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("file_path is required and must be a non-empty string")
	}
	file, err := obsidian.Load(path)
	if err != nil {
		return nil, err
	}
	local, err := readLocal(file)
	if err != nil {
		return nil, err
	}
	adfBody, err := t.encodeBody([]byte(file.Body))
	if err != nil {
		return nil, err
	}
	client, err := t.NewClient()
	if err != nil {
		return nil, err
	}
	issue, err := client.GetIssue(ctx, local.key)
	if err != nil {
		return nil, err
	}
	if err := checkParent(local, issue); err != nil {
		return nil, err
	}
	transitionID, err := resolveTransition(ctx, client, local, issue)
	if err != nil {
		return nil, err
	}
	if err := client.UpdateIssue(ctx, local.key, local.title, adfBody, typeIfChanged(local, issue)); err != nil {
		return nil, err
	}
	if transitionID != "" {
		if err := client.TransitionIssue(ctx, local.key, transitionID); err != nil {
			return nil, err
		}
	}
	return resultFrom(local, issue, transitionID != "", client), nil
}

// localFrontmatter is the validated view of the source file's YAML
// keys the tool actually reads. parentSet captures whether the user
// wrote a parent key at all, so we can distinguish "explicitly empty"
// from "absent" against the live ticket's parent.
type localFrontmatter struct {
	key, title, status, ticketType, parent string
	parentSet                              bool
}

// readLocal extracts and validates the four required keys plus the
// optional parent key. Errors here are typed so callers can tell a
// frontmatter problem apart from a network failure.
func readLocal(file *obsidian.File) (localFrontmatter, error) {
	var out localFrontmatter
	var err error
	if out.key, err = requireFrontmatterString(file.Frontmatter, KeyTicketKey); err != nil {
		return out, err
	}
	if out.title, err = requireFrontmatterString(file.Frontmatter, KeyTicketTitle); err != nil {
		return out, err
	}
	if out.status, err = requireFrontmatterString(file.Frontmatter, KeyTicketStatus); err != nil {
		return out, err
	}
	if out.ticketType, err = requireFrontmatterString(file.Frontmatter, KeyTicketType); err != nil {
		return out, err
	}
	if _, ok := file.Frontmatter[KeyParentKey]; ok {
		out.parentSet = true
		if out.parent, err = optionalFrontmatterString(file.Frontmatter, KeyParentKey); err != nil {
			return out, err
		}
	}
	return out, nil
}

// encodeBody converts the markdown body to an ADF document and
// JSON-encodes it. Returning the string form keeps the call site
// symmetric with the Confluence publish tool even though Jira embeds
// the doc as a literal object (the atlassian client decodes it with
// json.RawMessage on the way out).
func (t *Tool) encodeBody(body []byte) (string, error) {
	doc, err := t.Converter.Convert(body)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal ADF doc: %w", err)
	}
	return string(raw), nil
}

// checkParent enforces the "parent is immutable" rule. Any of these
// produces a typed error before the PUT:
//   - local key present, live parent absent
//   - local key absent, live parent present
//   - both present but different
//
// Only "both absent" and "both equal" pass through.
func checkParent(local localFrontmatter, issue atlassian.Issue) error {
	live := ""
	if issue.Fields.Parent != nil {
		live = issue.Fields.Parent.Key
	}
	if !local.parentSet && live == "" {
		return nil
	}
	if local.parent == live {
		return nil
	}
	switch {
	case local.parent != "" && live == "":
		return fmt.Errorf("jira.update-ticket: parent ticket %q is not set on live issue %s; "+
			"re-parenting is not supported", local.parent, issue.Key)
	case local.parent == "" && live != "":
		return fmt.Errorf("jira.update-ticket: live issue %s has parent %q but frontmatter omits it; "+
			"re-parenting is not supported", issue.Key, live)
	default:
		return fmt.Errorf("jira.update-ticket: parent ticket %q does not match live parent %q; "+
			"re-parenting is not supported", local.parent, live)
	}
}

// typeIfChanged returns local.ticketType when it differs (case-insensitive)
// from the live issue type, or "" when they match. The atlassian
// client omits the `issuetype` field from the PUT body when given an
// empty string, so an unchanged type stays out of the request.
func typeIfChanged(local localFrontmatter, issue atlassian.Issue) string {
	if strings.EqualFold(local.ticketType, issue.Fields.IssueType.Name) {
		return ""
	}
	return local.ticketType
}
