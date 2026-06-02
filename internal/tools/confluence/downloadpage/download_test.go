package downloadpage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ericmason/mdadf"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/markdown"
)

// fakeClient is the minimal atlassian.Client the download tool needs.
// getCalls records how many times GetPage ran so tests can assert that
// rejected inputs short-circuit before any network would happen.
type fakeClient struct {
	page     atlassian.Page
	getCalls int
	gotID    string
	baseURL  string
}

func (f *fakeClient) GetSpaceByKey(context.Context, string) (atlassian.Space, error) {
	return atlassian.Space{}, nil
}
func (f *fakeClient) FindPageBySpaceAndTitle(context.Context, string, string) (atlassian.Page, bool, error) {
	return atlassian.Page{}, false, nil
}
func (f *fakeClient) GetPage(_ context.Context, id string) (atlassian.Page, error) {
	f.getCalls++
	f.gotID = id
	return f.page, nil
}
func (f *fakeClient) CreatePage(context.Context, string, string, string) (atlassian.Page, error) {
	return atlassian.Page{}, nil
}
func (f *fakeClient) UpdatePage(context.Context, string, string, string, int) (atlassian.Page, error) {
	return atlassian.Page{}, nil
}

// fakeClientWithBase satisfies the optional BaseURL() seam the download
// tool uses for URL host validation and web-URL assembly.
type fakeClientWithBase struct {
	*fakeClient
	base string
}

func (f *fakeClientWithBase) BaseURL() string { return f.base }

// newToolWith builds a Tool wired to the given fake client.
func newToolWith(client atlassian.Client) *Tool {
	return &Tool{NewClient: func() (atlassian.Client, error) { return client, nil }}
}

// adfPageBody builds an ADF document with the given content and returns
// its JSON encoding, ready to slot into atlassian.Page.Body.AtlasDocFormat.Value.
func adfPageBody(t *testing.T, nodes ...markdown.Node) string {
	t.Helper()
	doc := &markdown.Document{Version: 1, Type: "doc", Content: nodes}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal adf: %v", err)
	}
	return string(b)
}

// propertyTableNode returns the exact 2-column "Property | Value" table
// the publish tool prepends to every published page.
func propertyTableNode(rows ...[2]string) markdown.Node {
	cells := []markdown.Node{
		mdadf.TableRowNode(
			mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Property", mdadf.StrongMark()))),
			mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Value", mdadf.StrongMark()))),
		),
	}
	for _, r := range rows {
		cells = append(cells, mdadf.TableRowNode(
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode(r[0]))),
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode(r[1]))),
		))
	}
	return mdadf.TableNode(cells...)
}

// TestInvoke_DownloadsAndLiftsPropertyTable covers the headline flow:
// numeric page ID + ADF body with leading property table → file is
// written, frontmatter carries page id + property rows, body is the
// rendered markdown without the table.
func TestInvoke_DownloadsAndLiftsPropertyTable(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeClient{page: atlassian.Page{ID: "42", Title: "My Page", SpaceID: "100"}}
	fc.page.Body.AtlasDocFormat.Value = adfPageBody(t,
		propertyTableNode([2]string{"author", "alice"}, [2]string{"status", "draft"}),
		mdadf.HeadingNode(1, mdadf.TextNode("Hello")),
		mdadf.ParagraphNode(mdadf.TextNode("World.")),
	)
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"page": "42", "output_dir": dir,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	r := res.(Result)
	if r.Action != "downloaded" || r.PageID != "42" || r.FilePath != filepath.Join(dir, "my-page.md") {
		t.Errorf("Result = %+v", r)
	}
	got, _ := os.ReadFile(r.FilePath)
	want := "---\nconfluence_page_id: 42\nauthor: alice\nstatus: draft\n---\n# Hello\n\nWorld.\n"
	if string(got) != want {
		t.Errorf("file content mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestInvoke_AcceptsURLAndValidatesHost makes sure URL input resolves to
// the embedded numeric ID and that a mismatched host is rejected before
// the fake client is even consulted.
func TestInvoke_AcceptsURLAndValidatesHost(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeClient{page: atlassian.Page{ID: "99", Title: "URL Page"}}
	fc.page.Body.AtlasDocFormat.Value = adfPageBody(t,
		mdadf.ParagraphNode(mdadf.TextNode("body")),
	)
	client := &fakeClientWithBase{fakeClient: fc, base: "https://acme.atlassian.net"}
	res, err := newToolWith(client).Invoke(context.Background(), map[string]any{
		"page":       "https://acme.atlassian.net/wiki/spaces/ENG/pages/99/URL+Page",
		"output_dir": dir,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if fc.gotID != "99" || res.(Result).PageID != "99" {
		t.Errorf("expected to resolve to id 99, got gotID=%q result=%+v", fc.gotID, res)
	}

	// Now point at a different host — must error before GetPage runs.
	prev := fc.getCalls
	_, err = newToolWith(client).Invoke(context.Background(), map[string]any{
		"page":       "https://other.atlassian.net/wiki/spaces/ENG/pages/99/URL+Page",
		"output_dir": dir,
	})
	if err == nil || !strings.Contains(err.Error(), "does not match configured") {
		t.Errorf("expected host-mismatch error, got %v", err)
	}
	if fc.getCalls != prev {
		t.Errorf("GetPage was called despite host mismatch (%d -> %d)", prev, fc.getCalls)
	}
}

// TestInvoke_ErrorsWhenOutputDirMissing surfaces a typed error and does
// not call the client when the destination directory does not exist.
func TestInvoke_ErrorsWhenOutputDirMissing(t *testing.T) {
	fc := &fakeClient{}
	_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"page": "42", "output_dir": filepath.Join(t.TempDir(), "nope"),
	})
	if err == nil || !strings.Contains(err.Error(), "must be an existing directory") {
		t.Errorf("expected output_dir error, got %v", err)
	}
	if fc.getCalls != 0 {
		t.Errorf("GetPage called despite missing output_dir")
	}
}

// TestInvoke_SlugFallsBackToPageIDForNonASCIITitle exercises the fallback
// branch in writeMarkdownFile when slugify yields an empty string.
func TestInvoke_SlugFallsBackToPageIDForNonASCIITitle(t *testing.T) {
	dir := t.TempDir()
	fc := &fakeClient{page: atlassian.Page{ID: "7", Title: "日本語"}}
	fc.page.Body.AtlasDocFormat.Value = adfPageBody(t,
		mdadf.ParagraphNode(mdadf.TextNode("body")),
	)
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"page": "7", "output_dir": dir,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if res.(Result).FilePath != filepath.Join(dir, "7.md") {
		t.Errorf("FilePath = %q, want fallback to <id>.md", res.(Result).FilePath)
	}
}

// TestInvoke_RejectsNonNumericNonURLPage validates the input parser
// before any client work.
func TestInvoke_RejectsNonNumericNonURLPage(t *testing.T) {
	fc := &fakeClient{}
	_, err := newToolWith(fc).Invoke(context.Background(), map[string]any{
		"page": "not-a-page", "output_dir": t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "neither a numeric ID nor a parseable URL") {
		t.Errorf("expected parser error, got %v", err)
	}
}
