// Confluence v2 REST client. The Client interface is what tools depend on;
// HTTPClient is the production implementation. Tests substitute their own
// Client (or, for HTTPClient itself, an httptest.Server).
package atlassian

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Page is a minimal subset of the Confluence v2 page response.
type Page struct {
	ID       string `json:"id"`
	SpaceID  string `json:"spaceId"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	ParentID string `json:"parentId,omitempty"`
	Version  struct {
		Number int `json:"number"`
	} `json:"version"`
	Links struct {
		WebUI string `json:"webui"`
	} `json:"_links"`
	// Body is populated when the page was fetched with
	// body-format=atlas_doc_format. Value is the stringified ADF JSON, ready
	// to be unmarshalled into a markdown.Document.
	Body struct {
		AtlasDocFormat struct {
			Representation string `json:"representation"`
			Value          string `json:"value"`
		} `json:"atlas_doc_format"`
	} `json:"body,omitempty"`
}

// WebURL returns the absolute URL to the page in the Confluence UI.
// baseURL should NOT include a trailing /wiki.
func (p Page) WebURL(baseURL string) string {
	if p.Links.WebUI == "" {
		return ""
	}
	return strings.TrimRight(baseURL, "/") + "/wiki" + p.Links.WebUI
}

// Space is a minimal subset of the Confluence v2 space response.
type Space struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Client is the seam the tool depends on. Operations target current pages
// in the body.atlas_doc_format representation.
type Client interface {
	GetSpaceByKey(ctx context.Context, key string) (Space, error)
	FindPageBySpaceAndTitle(ctx context.Context, spaceID, title string) (Page, bool, error)
	GetPage(ctx context.Context, id string) (Page, error)
	CreatePage(ctx context.Context, spaceID, title, adfBody string) (Page, error)
	UpdatePage(ctx context.Context, id, title, adfBody string, version int) (Page, error)
}

// HTTPClient is the production Client backed by Confluence Cloud REST v2.
type HTTPClient struct {
	cfg  Config
	http *http.Client
}

// NewHTTPClient returns an HTTPClient with a sensible default timeout.
func NewHTTPClient(cfg Config) *HTTPClient {
	return &HTTPClient{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}}
}

// WithHTTPClient overrides the underlying *http.Client (used by tests).
func (c *HTTPClient) WithHTTPClient(h *http.Client) *HTTPClient {
	c.http = h
	return c
}

// BaseURL returns the configured workspace base URL. The publish tool uses
// it to assemble the page's absolute web URL from a Page.Links.WebUI value.
func (c *HTTPClient) BaseURL() string { return c.cfg.BaseURL }

// APIError is returned when the server responds with a non-2xx status.
type APIError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("atlassian: %s %s: %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

// do performs the request, decoding a 2xx JSON response into out (if non-nil).
// On non-2xx it returns an *APIError carrying the response body for context.
func (c *HTTPClient) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("atlassian: marshal %s %s: %w", method, path, err)
		}
		rdr = bytes.NewReader(buf)
	}
	full := c.cfg.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, full, rdr)
	if err != nil {
		return fmt.Errorf("atlassian: build %s %s: %w", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	auth := base64.StdEncoding.EncodeToString([]byte(c.cfg.Email + ":" + c.cfg.APIToken))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("atlassian: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Method: method, URL: full, StatusCode: resp.StatusCode, Body: string(raw)}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("atlassian: decode %s %s: %w", method, path, err)
	}
	return nil
}
