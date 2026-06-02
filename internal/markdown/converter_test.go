package markdown

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestConvert_ProducesADFDoc renders a tiny markdown snippet and verifies
// the wrapper returned a real ADF document (version 1, type "doc", with
// some content) rather than nil or a malformed shell.
func TestConvert_ProducesADFDoc(t *testing.T) {
	doc, err := Default().Convert([]byte("# Title\n\nBody."))
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if doc.Version != 1 || doc.Type != "doc" {
		t.Errorf("doc = {v:%d t:%q}, want {1 doc}", doc.Version, doc.Type)
	}
	if len(doc.Content) == 0 {
		t.Fatalf("Content empty")
	}
}

// TestConvert_EmptyBody returns a valid empty doc rather than nil.
func TestConvert_EmptyBody(t *testing.T) {
	doc, err := Default().Convert(nil)
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if doc == nil || doc.Version != 1 || doc.Type != "doc" {
		t.Fatalf("doc = %+v, want valid empty doc", doc)
	}
}

// TestPrependPropertyTable_NoPropsIsNoop returns the original doc unchanged
// when there are no properties to render.
func TestPrependPropertyTable_NoPropsIsNoop(t *testing.T) {
	doc, _ := Default().Convert([]byte("body"))
	originalLen := len(doc.Content)
	out := PrependPropertyTable(doc, nil)
	if len(out.Content) != originalLen {
		t.Errorf("len(Content) = %d, want %d (no-op)", len(out.Content), originalLen)
	}
}

// TestPrependPropertyTable_InsertsSortedTable inserts a table as the first
// content node, with keys sorted and values rendered via FormatValue. The
// JSON form is checked so the test is robust to changes in node helpers.
func TestPrependPropertyTable_InsertsSortedTable(t *testing.T) {
	doc, _ := Default().Convert([]byte("Body."))
	props := map[string]any{
		"author":  "alice",
		"tags":    []any{"a", "b"},
		"version": 2,
	}
	out := PrependPropertyTable(doc, props)
	if len(out.Content) == 0 || out.Content[0].Type != "table" {
		t.Fatalf("first node = %+v, want table", out.Content[0])
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(raw)
	// Header row exists.
	if !strings.Contains(js, `"text":"Property"`) || !strings.Contains(js, `"text":"Value"`) {
		t.Errorf("header missing; doc:\n%s", js)
	}
	// Keys appear in sorted order: author, tags, version.
	a := strings.Index(js, `"text":"author"`)
	b := strings.Index(js, `"text":"tags"`)
	c := strings.Index(js, `"text":"version"`)
	if a < 0 || b < 0 || c < 0 || !(a < b && b < c) {
		t.Errorf("key order wrong: author=%d tags=%d version=%d", a, b, c)
	}
	// Values rendered: "alice", "a, b", "2".
	for _, want := range []string{`"text":"alice"`, `"text":"a, b"`, `"text":"2"`} {
		if !strings.Contains(js, want) {
			t.Errorf("missing value %q in:\n%s", want, js)
		}
	}
}

// TestFormatValue covers each scalar branch plus the slice and map fallback.
func TestFormatValue(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{nil, ""},
		{"hello", "hello"},
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{int64(99), "99"},
		{float64(3), "3"}, // integral floats lose the trailing zero
		{float64(1.5), "1.5"},
		{[]any{"x", 1, true}, "x, 1, true"},
		{map[string]any{"k": "v"}, `{"k":"v"}`},
	}
	for _, c := range cases {
		if got := FormatValue(c.in); got != c.want {
			t.Errorf("FormatValue(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
