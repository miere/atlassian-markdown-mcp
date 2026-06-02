package publishobsidianfile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miere/atlassian-markdown-mcp/internal/atlassian"
	"github.com/miere/atlassian-markdown-mcp/internal/markdown"
)

// fakeClient is a minimal atlassian.Client whose methods record their args
// and return canned values. Each test wires only the fields it cares about.
type fakeClient struct {
	space     atlassian.Space
	findPage  atlassian.Page
	findFound bool
	getPage   atlassian.Page

	createSpaceID, createTitle, createBody string
	updateID, updateTitle, updateBody      string
	updateVersion                          int
	created, updated                       atlassian.Page
}

func (f *fakeClient) GetSpaceByKey(_ context.Context, _ string) (atlassian.Space, error) {
	return f.space, nil
}
func (f *fakeClient) FindPageBySpaceAndTitle(_ context.Context, _, _ string) (atlassian.Page, bool, error) {
	return f.findPage, f.findFound, nil
}
func (f *fakeClient) GetPage(_ context.Context, _ string) (atlassian.Page, error) {
	return f.getPage, nil
}
func (f *fakeClient) CreatePage(_ context.Context, spaceID, title, body string) (atlassian.Page, error) {
	f.createSpaceID, f.createTitle, f.createBody = spaceID, title, body
	return f.created, nil
}
func (f *fakeClient) UpdatePage(_ context.Context, id, title, body string, version int) (atlassian.Page, error) {
	f.updateID, f.updateTitle, f.updateBody, f.updateVersion = id, title, body, version
	return f.updated, nil
}

func writeNote(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func newToolWith(client atlassian.Client) *Tool {
	return &Tool{
		Converter: markdown.Default(),
		NewClient: func() (atlassian.Client, error) { return client, nil },
	}
}

// TestInvoke_CreatesPageAndWritesBackPageID exercises the first-publish
// branch: no confluence_page_id in frontmatter, no existing page in the
// target space → CreatePage is called and the new ID lands in the file.
func TestInvoke_CreatesPageAndWritesBackPageID(t *testing.T) {
	path := writeNote(t, "---\nconfluence_space: ENG\nconfluence_title: My Page\n---\n# Hello\n")
	fc := &fakeClient{
		space:   atlassian.Space{ID: "100", Key: "ENG"},
		created: atlassian.Page{ID: "555", Title: "My Page", SpaceID: "100"},
	}
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	r := res.(Result)
	if r.Action != "created" || r.PageID != "555" {
		t.Errorf("Result = %+v, want created/555", r)
	}
	if fc.createSpaceID != "100" || fc.createTitle != "My Page" {
		t.Errorf("Create args = (%q,%q,...)", fc.createSpaceID, fc.createTitle)
	}
	if !strings.Contains(fc.createBody, `"type":"doc"`) {
		t.Errorf("create body not an ADF doc: %s", fc.createBody)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "confluence_page_id: 555") {
		t.Errorf("page id not written back; file:\n%s", got)
	}
}

// TestInvoke_UpdatesByPageIDWhenPresent skips the title lookup entirely
// when confluence_page_id is set, fetching the current version and PUT-ing
// version+1 to that same ID. The remote title wins over any local
// confluence_title — the frontmatter value is ignored on update.
func TestInvoke_UpdatesByPageIDWhenPresent(t *testing.T) {
	path := writeNote(t, "---\nconfluence_space: ENG\nconfluence_title: Stale Local Title\nconfluence_page_id: \"42\"\n---\nBody.\n")
	fc := &fakeClient{
		getPage: atlassian.Page{ID: "42", Title: "Live Remote Title", SpaceID: "100",
			Version: struct {
				Number int `json:"number"`
			}{Number: 7}},
		updated: atlassian.Page{ID: "42", Title: "Live Remote Title", SpaceID: "100"},
	}
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	r := res.(Result)
	if r.Action != "updated" || r.PageID != "42" {
		t.Errorf("Result = %+v, want updated/42", r)
	}
	if fc.updateID != "42" || fc.updateVersion != 8 {
		t.Errorf("Update args id=%q v=%d, want 42/8", fc.updateID, fc.updateVersion)
	}
	if fc.updateTitle != "Live Remote Title" {
		t.Errorf("Update title = %q, want %q (frontmatter title must be ignored)",
			fc.updateTitle, "Live Remote Title")
	}
}

// TestInvoke_UpdatesByPageIDWithoutSpaceOrTitle covers the relaxed contract:
// once confluence_page_id is bound, confluence_space and confluence_title
// may be omitted entirely and the update still goes through.
func TestInvoke_UpdatesByPageIDWithoutSpaceOrTitle(t *testing.T) {
	path := writeNote(t, "---\nconfluence_page_id: \"42\"\n---\nBody.\n")
	fc := &fakeClient{
		getPage: atlassian.Page{ID: "42", Title: "Live Remote Title", SpaceID: "100",
			Version: struct {
				Number int `json:"number"`
			}{Number: 7}},
		updated: atlassian.Page{ID: "42", Title: "Live Remote Title", SpaceID: "100"},
	}
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	r := res.(Result)
	if r.Action != "updated" || r.PageID != "42" || r.Title != "Live Remote Title" {
		t.Errorf("Result = %+v, want updated/42/Live Remote Title", r)
	}
	if fc.updateID != "42" || fc.updateTitle != "Live Remote Title" || fc.updateVersion != 8 {
		t.Errorf("Update args = (%q,%q,v=%d), want (42, Live Remote Title, 8)",
			fc.updateID, fc.updateTitle, fc.updateVersion)
	}
}

// TestInvoke_UpdatesByTitleWhenPageExists covers the second-publish branch
// where the local file lost its page_id but the title still resolves to a
// real page in the target space.
func TestInvoke_UpdatesByTitleWhenPageExists(t *testing.T) {
	path := writeNote(t, "---\nconfluence_space: ENG\nconfluence_title: My Page\n---\nBody.\n")
	fc := &fakeClient{
		space: atlassian.Space{ID: "100", Key: "ENG"},
		findPage: atlassian.Page{ID: "99", Title: "My Page", SpaceID: "100",
			Version: struct {
				Number int `json:"number"`
			}{Number: 3}},
		findFound: true,
		updated:   atlassian.Page{ID: "99", Title: "My Page", SpaceID: "100"},
	}
	res, err := newToolWith(fc).Invoke(context.Background(), map[string]any{"file_path": path})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	r := res.(Result)
	if r.Action != "updated" || r.PageID != "99" {
		t.Errorf("Result = %+v, want updated/99", r)
	}
	if fc.updateVersion != 4 {
		t.Errorf("Update version = %d, want 4", fc.updateVersion)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "confluence_page_id: 99") {
		t.Errorf("page id not written back; file:\n%s", got)
	}
}

// TestInvoke_ErrorsWhenFrontmatterIncomplete fails fast before touching the
// network when the required sync metadata is missing.
func TestInvoke_ErrorsWhenFrontmatterIncomplete(t *testing.T) {
	path := writeNote(t, "---\nconfluence_space: ENG\n---\nBody.\n")
	_, err := newToolWith(&fakeClient{}).Invoke(context.Background(), map[string]any{"file_path": path})
	if err == nil || !strings.Contains(err.Error(), "confluence_title") {
		t.Errorf("err = %v, want missing confluence_title", err)
	}
}
