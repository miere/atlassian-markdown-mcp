package obsidian

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp creates a markdown file under t.TempDir and returns its path.
func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

// TestLoad_ParsesFrontmatterAndBody covers the standard happy path: a file
// with a `---`-fenced YAML block followed by markdown content.
func TestLoad_ParsesFrontmatterAndBody(t *testing.T) {
	path := writeTemp(t, "---\nconfluence_space: ENG\nconfluence_title: My Page\n---\n# Hello\n\nBody here.\n")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := f.Frontmatter["confluence_space"]; got != "ENG" {
		t.Errorf("confluence_space = %v, want ENG", got)
	}
	if got := f.Frontmatter["confluence_title"]; got != "My Page" {
		t.Errorf("confluence_title = %v, want My Page", got)
	}
	if !strings.HasPrefix(f.Body, "# Hello") {
		t.Errorf("Body should start with heading, got %q", f.Body)
	}
}

// TestLoad_NoFrontmatter returns an empty map and the full file as body.
func TestLoad_NoFrontmatter(t *testing.T) {
	path := writeTemp(t, "# Plain\n\nNo frontmatter.\n")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(f.Frontmatter) != 0 {
		t.Errorf("Frontmatter = %v, want empty", f.Frontmatter)
	}
	if !strings.HasPrefix(f.Body, "# Plain") {
		t.Errorf("Body = %q", f.Body)
	}
}

// TestSetFrontmatterKey_ReplacesExisting rewrites a key in place while
// leaving the surrounding lines, order, and body untouched.
func TestSetFrontmatterKey_ReplacesExisting(t *testing.T) {
	path := writeTemp(t, "---\nconfluence_space: ENG\nconfluence_page_id: old\n---\nbody\n")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := f.SetFrontmatterKey("confluence_page_id", "12345"); err != nil {
		t.Fatalf("SetFrontmatterKey: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "---\nconfluence_space: ENG\nconfluence_page_id: 12345\n---\nbody\n"
	if string(got) != want {
		t.Errorf("file contents:\n--got--\n%s\n--want--\n%s", got, want)
	}
}

// TestSetFrontmatterKey_InsertsBeforeClosingFence appends a new key inside
// the existing frontmatter block instead of overwriting an existing line.
func TestSetFrontmatterKey_InsertsBeforeClosingFence(t *testing.T) {
	path := writeTemp(t, "---\nconfluence_space: ENG\n---\nbody\n")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := f.SetFrontmatterKey("confluence_page_id", "999"); err != nil {
		t.Fatalf("SetFrontmatterKey: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "---\nconfluence_space: ENG\nconfluence_page_id: 999\n---\nbody\n"
	if string(got) != want {
		t.Errorf("file contents:\n--got--\n%s\n--want--\n%s", got, want)
	}
}

// TestSetFrontmatterKey_AddsFrontmatterWhenAbsent prepends a fresh block
// when the file had no frontmatter to begin with.
func TestSetFrontmatterKey_AddsFrontmatterWhenAbsent(t *testing.T) {
	path := writeTemp(t, "# Plain\n")
	f, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := f.SetFrontmatterKey("confluence_page_id", "42"); err != nil {
		t.Fatalf("SetFrontmatterKey: %v", err)
	}
	got, _ := os.ReadFile(path)
	// A blank line is inserted between the closing fence and the original
	// body so the frontmatter visually separates from the markdown.
	want := "---\nconfluence_page_id: 42\n---\n\n# Plain\n"
	if string(got) != want {
		t.Errorf("file contents:\n--got--\n%s\n--want--\n%s", got, want)
	}
}

// TestScalarYAML_QuotesAmbiguousStrings verifies that values which would
// otherwise be parsed as booleans, nulls, or comments are quoted on write.
func TestScalarYAML_QuotesAmbiguousStrings(t *testing.T) {
	cases := map[string]string{
		"plain":   "plain",
		"":        `""`,
		"yes":     `"yes"`,
		"value:1": `"value:1"`,
	}
	for in, want := range cases {
		if got := scalarYAML(in); got != want {
			t.Errorf("scalarYAML(%q) = %q, want %q", in, got, want)
		}
	}
}
