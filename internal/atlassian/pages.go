// Confluence v2 page and space operations implemented on top of HTTPClient.
package atlassian

import (
	"context"
	"fmt"
	"net/url"
)

// GetSpaceByKey looks the space up by its short key (e.g. "ENG").
// Returns an error if the key matches zero spaces or fails to load.
func (c *HTTPClient) GetSpaceByKey(ctx context.Context, key string) (Space, error) {
	var resp struct {
		Results []Space `json:"results"`
	}
	path := "/wiki/api/v2/spaces?keys=" + url.QueryEscape(key)
	if err := c.do(ctx, "GET", path, nil, &resp); err != nil {
		return Space{}, err
	}
	if len(resp.Results) == 0 {
		return Space{}, fmt.Errorf("atlassian: no space with key %q", key)
	}
	return resp.Results[0], nil
}

// FindPageBySpaceAndTitle returns the page matching (spaceID, title), or
// (_, false, nil) if no such page exists. An API error is propagated as-is.
func (c *HTTPClient) FindPageBySpaceAndTitle(ctx context.Context, spaceID, title string) (Page, bool, error) {
	var resp struct {
		Results []Page `json:"results"`
	}
	path := fmt.Sprintf("/wiki/api/v2/spaces/%s/pages?title=%s&body-format=atlas_doc_format&limit=2",
		url.PathEscape(spaceID), url.QueryEscape(title))
	if err := c.do(ctx, "GET", path, nil, &resp); err != nil {
		return Page{}, false, err
	}
	if len(resp.Results) == 0 {
		return Page{}, false, nil
	}
	return resp.Results[0], true, nil
}

// GetPage fetches a single page by ID, including its current version.
func (c *HTTPClient) GetPage(ctx context.Context, id string) (Page, error) {
	var p Page
	path := "/wiki/api/v2/pages/" + url.PathEscape(id) + "?body-format=atlas_doc_format"
	if err := c.do(ctx, "GET", path, nil, &p); err != nil {
		return Page{}, err
	}
	return p, nil
}

// CreatePage creates a current page under spaceID with the given title and
// ADF-stringified body. parentId is intentionally not exposed in v1.
func (c *HTTPClient) CreatePage(ctx context.Context, spaceID, title, adfBody string) (Page, error) {
	payload := map[string]any{
		"spaceId": spaceID,
		"status":  "current",
		"title":   title,
		"body": map[string]any{
			"representation": "atlas_doc_format",
			"value":          adfBody,
		},
	}
	var p Page
	if err := c.do(ctx, "POST", "/wiki/api/v2/pages", payload, &p); err != nil {
		return Page{}, err
	}
	return p, nil
}

// UpdatePage replaces an existing page's title and body. version MUST be the
// new version number (current + 1); the Confluence server rejects equal or
// lower numbers with a 409.
func (c *HTTPClient) UpdatePage(ctx context.Context, id, title, adfBody string, version int) (Page, error) {
	payload := map[string]any{
		"id":     id,
		"status": "current",
		"title":  title,
		"body": map[string]any{
			"representation": "atlas_doc_format",
			"value":          adfBody,
		},
		"version": map[string]any{"number": version},
	}
	var p Page
	if err := c.do(ctx, "PUT", "/wiki/api/v2/pages/"+url.PathEscape(id), payload, &p); err != nil {
		return Page{}, err
	}
	return p, nil
}
