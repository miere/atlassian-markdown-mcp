package downloadpage

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
	"github.com/miere/atlassian-markdown-mcp/internal/obsidian"
)

// Invoke resolves the page identifier to a numeric ID, fetches the page in
// atlas_doc_format, converts the ADF body to markdown, optionally lifts
// the leading property table back into YAML frontmatter, and writes the
// result to disk under output_dir.
func (t *Tool) Invoke(ctx context.Context, args map[string]any) (any, error) {
	pageArg, err := requireString(args, "page")
	if err != nil {
		return nil, err
	}
	rawOutDir, err := optionalString(args, "output_dir", "")
	if err != nil {
		return nil, err
	}
	outDir := obsidian.ResolveDir(rawOutDir, DefaultOutputDir)
	if info, err := os.Stat(outDir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("output_dir %q must be an existing directory", outDir)
	}
	client, err := t.NewClient()
	if err != nil {
		return nil, err
	}
	id, err := resolvePageID(pageArg, baseURLOf(client))
	if err != nil {
		return nil, err
	}
	page, err := client.GetPage(ctx, id)
	if err != nil {
		return nil, err
	}
	mdBody, fm, err := bodyAndFrontmatter(page)
	if err != nil {
		return nil, err
	}
	dest, err := writeMarkdownFile(outDir, page, fm, mdBody)
	if err != nil {
		return nil, err
	}
	return resultFrom(page, dest, client), nil
}

// resolvePageID accepts either a numeric ID or a URL. URL inputs are
// validated against baseHost so the tool refuses to leak credentials to a
// different Confluence workspace. baseHost may be empty when the client
// does not expose a BaseURL (only the production HTTPClient does); in
// that case the host check is skipped.
func resolvePageID(input, baseURL string) (string, error) {
	input = strings.TrimSpace(input)
	if numericRE.MatchString(input) {
		return input, nil
	}
	u, err := url.Parse(input)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("page %q is neither a numeric ID nor a parseable URL", input)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("page URL %q must use http(s)", input)
	}
	if baseURL != "" {
		bu, err := url.Parse(baseURL)
		if err == nil && bu.Host != "" && !strings.EqualFold(u.Host, bu.Host) {
			return "", fmt.Errorf("page URL host %q does not match configured ATLASSIAN_BASE_URL host %q",
				u.Host, bu.Host)
		}
	}
	m := pageURLRE.FindStringSubmatch(u.Path)
	if len(m) < 2 {
		return "", fmt.Errorf("page URL %q does not contain a /pages/<id>/ segment", input)
	}
	return m[1], nil
}

// bodyAndFrontmatter parses the page's ADF body, splits off the leading
// property table (if any), renders the rest as markdown, and assembles
// the frontmatter map (always carrying confluence_page_id).
func bodyAndFrontmatter(page atlassian.Page) (string, frontmatter, error) {
	raw := page.Body.AtlasDocFormat.Value
	fm := frontmatter{Keys: []string{KeyPageID}, Values: map[string]string{KeyPageID: page.ID}}
	if raw == "" {
		return "", fm, nil
	}
	var doc markdown.Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fm, fmt.Errorf("parse ADF body for page %s: %w", page.ID, err)
	}
	props, propKeys, rest := markdown.SplitPropertyTable(&doc)
	for _, k := range propKeys {
		if k == KeyPageID {
			continue // never overwrite the canonical sync key
		}
		fm.Keys = append(fm.Keys, k)
		fm.Values[k] = props[k]
	}
	return markdown.RenderMarkdown(rest), fm, nil
}

// writeMarkdownFile renders the final file content and writes it
// atomically: the bytes go to a temp file in the same directory, then
// os.Rename replaces the destination. Existing files are overwritten.
// Returns the absolute destination path.
func writeMarkdownFile(outDir string, page atlassian.Page, fm frontmatter, body string) (string, error) {
	name := slugify(page.Title)
	if name == "" {
		name = page.ID
	}
	dest := filepath.Join(outDir, name+".md")
	content := fm.render() + body
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	tmp, err := os.CreateTemp(outDir, ".download-page-*.tmp")
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
