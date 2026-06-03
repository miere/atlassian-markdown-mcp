package publishobsidianfile

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/markdown"
	"github.com/miere/atlassian-markdown-mcp/internal/obsidian"
)

// Invoke loads the file, converts its body to ADF, prepends the
// frontmatter-derived property table, and pushes the result to Confluence,
// fully rewriting the page. On first publish the new page ID is written
// back into the file's YAML frontmatter.
func (t *Tool) Invoke(ctx context.Context, args map[string]any) (any, error) {
	path, ok := args["file_path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("file_path is required and must be a non-empty string")
	}
	path = obsidian.ResolvePath(path)
	file, err := obsidian.Load(path)
	if err != nil {
		return nil, err
	}
	sync, props := splitFrontmatter(file.Frontmatter)
	pageID, err := optionalString(sync, KeyPageID)
	if err != nil {
		return nil, err
	}
	// Space and title are only needed when the page hasn't been published
	// yet. Once confluence_page_id is recorded, the update branch reuses the
	// existing page's title, so the local frontmatter can drop both keys.
	var title, spaceKey string
	if pageID == "" {
		title, err = requireString(sync, KeyTitle)
		if err != nil {
			return nil, err
		}
		spaceKey, err = requireString(sync, KeySpace)
		if err != nil {
			return nil, err
		}
	}

	adfBody, err := t.buildADFBody([]byte(file.Body), props)
	if err != nil {
		return nil, err
	}
	client, err := t.NewClient()
	if err != nil {
		return nil, err
	}
	return t.publish(ctx, client, file, spaceKey, title, pageID, adfBody)
}

// buildADFBody renders the markdown body to ADF, prepends the property
// table, and JSON-encodes the result as the string Confluence expects in
// body.value.
func (t *Tool) buildADFBody(body []byte, props map[string]any) (string, error) {
	doc, err := t.Converter.Convert(body)
	if err != nil {
		return "", err
	}
	doc = markdown.PrependPropertyTable(doc, props)
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("marshal ADF doc: %w", err)
	}
	return string(raw), nil
}

// publish resolves the target page and creates or updates it. On create it
// writes the new page ID back into the source file's frontmatter so the
// next invocation goes straight to the update branch.
func (t *Tool) publish(
	ctx context.Context,
	client atlassian.Client,
	file *obsidian.File,
	spaceKey, title, pageID, adfBody string,
) (Result, error) {
	if pageID != "" {
		current, err := client.GetPage(ctx, pageID)
		if err != nil {
			return Result{}, err
		}
		// Preserve the page's existing title on Confluence; the source
		// frontmatter is not the source of truth for the title once a
		// page_id is bound, so any local `confluence_title` is ignored.
		updated, err := client.UpdatePage(ctx, pageID, current.Title, adfBody, current.Version.Number+1)
		if err != nil {
			return Result{}, err
		}
		return resultFrom("updated", updated, client), nil
	}

	space, err := client.GetSpaceByKey(ctx, spaceKey)
	if err != nil {
		return Result{}, err
	}
	existing, found, err := client.FindPageBySpaceAndTitle(ctx, space.ID, title)
	if err != nil {
		return Result{}, err
	}
	if found {
		updated, err := client.UpdatePage(ctx, existing.ID, title, adfBody, existing.Version.Number+1)
		if err != nil {
			return Result{}, err
		}
		if err := file.SetFrontmatterKey(KeyPageID, updated.ID); err != nil {
			return Result{}, err
		}
		return resultFrom("updated", updated, client), nil
	}

	created, err := client.CreatePage(ctx, space.ID, title, adfBody)
	if err != nil {
		return Result{}, err
	}
	if err := file.SetFrontmatterKey(KeyPageID, created.ID); err != nil {
		return Result{}, err
	}
	return resultFrom("created", created, client), nil
}

// resultFrom converts an atlassian.Page into the tool's Result. The web URL
// is best-effort: we only have a base URL when the production HTTPClient is
// in use; the fake clients used in tests leave it blank.
func resultFrom(action string, p atlassian.Page, client atlassian.Client) Result {
	r := Result{Action: action, PageID: p.ID, Title: p.Title, SpaceID: p.SpaceID}
	if hc, ok := client.(interface{ BaseURL() string }); ok {
		r.WebURL = p.WebURL(hc.BaseURL())
	}
	return r
}
