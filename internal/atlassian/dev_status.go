// Best-effort scrape of Jira's undocumented /rest/dev-status/latest
// endpoint. The shape returned varies between integrations (GitHub,
// Bitbucket, GitLab) but each provider keeps a `pullRequests` or
// `branches` array whose elements have a `url` (or `name` for
// branches) field. We walk the response generically so we do not have
// to model each provider, and any error short-circuits to an empty
// slice — the caller's only requirement is that the keys exist in
// frontmatter, even if the list is empty.
package atlassian

import (
	"context"
	"fmt"
	"net/url"
)

// fetchDevStatus issues GET /rest/dev-status/latest/issue/detail and
// extracts the per-provider list keyed by `dataType` ("pullrequest"
// or "branch"). The response is shaped like:
//
//	{
//	  "detail": [
//	    {
//	      "pullRequests": [ {"url": "https://...", ...}, ... ],
//	      "branches":     [ {"name": "feat-x", ...}, ... ],
//	      ...
//	    }
//	  ]
//	}
//
// We accept any of the documented dataType strings on the wire but
// only ever read the two we care about. A non-2xx (common: 400 on
// tenants without the DevTools add-on) returns (nil, err) and the
// caller swallows the error.
func (c *HTTPClient) fetchDevStatus(ctx context.Context, issueID, dataType string) ([]string, error) {
	q := url.Values{}
	q.Set("issueId", issueID)
	q.Set("applicationType", "GitHub")
	q.Set("dataType", dataType)
	path := "/rest/dev-status/latest/issue/detail?" + q.Encode()
	var resp devStatusResponse
	if err := c.do(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, detail := range resp.Detail {
		switch dataType {
		case "pullrequest":
			for _, pr := range detail.PullRequests {
				if pr.URL != "" {
					out = append(out, pr.URL)
				}
			}
		case "branch":
			for _, b := range detail.Branches {
				if b.Name != "" {
					out = append(out, b.Name)
				}
			}
		default:
			return nil, fmt.Errorf("atlassian: unsupported dev-status dataType %q", dataType)
		}
	}
	return out, nil
}

// devStatusResponse is the minimal projection of the dev-status
// endpoint. Provider-specific extras (avatar, status, lastUpdate, …)
// are dropped on unmarshal.
type devStatusResponse struct {
	Detail []struct {
		PullRequests []devStatusPR     `json:"pullRequests"`
		Branches     []devStatusBranch `json:"branches"`
	} `json:"detail"`
}

type devStatusPR struct {
	URL string `json:"url"`
}

type devStatusBranch struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
