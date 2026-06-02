package markdown

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ericmason/mdadf"
)

// adfDoc wraps a slice of nodes in a fully-formed ADF Document so each
// test can be written as just the interesting content.
func adfDoc(nodes ...Node) *Document {
	return &Document{Version: 1, Type: "doc", Content: nodes}
}

// TestRenderMarkdown_HandlesAllSupportedNodes is a single round-trip-style
// test that exercises every node type listed in the design's coverage
// table. The assertion uses substring checks so the test stays readable
// without freezing every byte of whitespace.
func TestRenderMarkdown_HandlesAllSupportedNodes(t *testing.T) {
	doc := adfDoc(
		mdadf.HeadingNode(2, mdadf.TextNode("Heading two")),
		mdadf.ParagraphNode(
			mdadf.TextNode("plain "),
			mdadf.TextNode("bold", mdadf.StrongMark()),
			mdadf.TextNode(" "),
			mdadf.TextNode("italic", mdadf.EmMark()),
			mdadf.TextNode(" "),
			mdadf.TextNode("code", mdadf.CodeMark()),
			mdadf.TextNode(" "),
			mdadf.TextNode("link", mdadf.LinkMark("https://x.test", "")),
		),
		mdadf.BulletListNode(
			mdadf.ListItemNode(mdadf.ParagraphNode(mdadf.TextNode("one"))),
			mdadf.ListItemNode(
				mdadf.ParagraphNode(mdadf.TextNode("two")),
				mdadf.BulletListNode(
					mdadf.ListItemNode(mdadf.ParagraphNode(mdadf.TextNode("nested"))),
				),
			),
		),
		mdadf.OrderedListNode(
			mdadf.ListItemNode(mdadf.ParagraphNode(mdadf.TextNode("first"))),
		),
		mdadf.CodeBlockNode("go", mdadf.TextNode("fmt.Println()")),
		mdadf.BlockquoteNode(mdadf.ParagraphNode(mdadf.TextNode("quoted"))),
		mdadf.RuleNode(),
		mdadf.TableNode(
			mdadf.TableRowNode(
				mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("h1"))),
				mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("h2"))),
			),
			mdadf.TableRowNode(
				mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode("a"))),
				mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode("b"))),
			),
		),
		mdadf.TaskListNode(
			mdadf.TaskItemNode(false, mdadf.TextNode("todo")),
			mdadf.TaskItemNode(true, mdadf.TextNode("done")),
		),
	)
	got := RenderMarkdown(doc)
	wants := []string{
		"## Heading two",
		"plain **bold** *italic* `code` [link](https://x.test)",
		"- one",
		"- two\n  - nested",
		"1. first",
		"```go\nfmt.Println()\n```",
		"> quoted",
		"\n---\n",
		"| h1 | h2 |\n| --- | --- |\n| a | b |",
		"- [ ] todo",
		"- [x] done",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("RenderMarkdown missing %q\n---\n%s", w, got)
		}
	}
}

// TestRenderMarkdown_HandlesConfluenceOnlyNodes covers panel, expand,
// mention, emoji, status, and inlineCard — none of which mdadf emits, so
// they are built by hand here.
func TestRenderMarkdown_HandlesConfluenceOnlyNodes(t *testing.T) {
	attrs := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	doc := adfDoc(
		Node{Type: "panel", Attrs: attrs(map[string]string{"panelType": "warning"}),
			Content: []Node{mdadf.ParagraphNode(mdadf.TextNode("careful"))}},
		Node{Type: "expand", Attrs: attrs(map[string]string{"title": "Details"}),
			Content: []Node{mdadf.ParagraphNode(mdadf.TextNode("hidden"))}},
		mdadf.ParagraphNode(
			Node{Type: "mention", Attrs: attrs(map[string]string{"text": "@Alice", "id": "u1"})},
			mdadf.TextNode(" "),
			Node{Type: "emoji", Attrs: attrs(map[string]string{"shortName": ":smile:", "text": "😄"})},
			mdadf.TextNode(" "),
			Node{Type: "status", Attrs: attrs(map[string]string{"text": "Done", "color": "green"})},
			mdadf.TextNode(" "),
			Node{Type: "inlineCard", Attrs: attrs(map[string]string{"url": "https://card.test"})},
		),
	)
	got := RenderMarkdown(doc)
	for _, w := range []string{
		"> [!WARNING]\n> careful",
		"<details><summary>Details</summary>\nhidden\n</details>",
		"@Alice :smile: `[STATUS:green:Done]` <https://card.test>",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("RenderMarkdown missing %q\n---\n%s", w, got)
		}
	}
}

// TestRenderMarkdown_UnknownNodeDegradesGracefully checks the fallback
// for nodes the renderer does not know about — it must emit a placeholder
// comment instead of failing or dropping content silently.
func TestRenderMarkdown_UnknownNodeDegradesGracefully(t *testing.T) {
	doc := adfDoc(mdadf.ParagraphNode(Node{Type: "mediaSingle"}))
	got := RenderMarkdown(doc)
	if !strings.Contains(got, "<!-- unsupported: mediaSingle -->") {
		t.Errorf("expected unsupported placeholder, got %q", got)
	}
}

// TestSplitPropertyTable_DetectsAndStripsLeadingTable lifts the
// property/value rows back into a map and returns the rest of the doc
// untouched.
func TestSplitPropertyTable_DetectsAndStripsLeadingTable(t *testing.T) {
	table := mdadf.TableNode(
		mdadf.TableRowNode(
			mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Property", mdadf.StrongMark()))),
			mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Value", mdadf.StrongMark()))),
		),
		mdadf.TableRowNode(
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode("author"))),
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode("alice"))),
		),
		mdadf.TableRowNode(
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode("status"))),
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode("draft"))),
		),
	)
	body := mdadf.ParagraphNode(mdadf.TextNode("body"))
	props, keys, rest := SplitPropertyTable(adfDoc(table, body))
	if props["author"] != "alice" || props["status"] != "draft" {
		t.Errorf("props = %v", props)
	}
	if len(keys) != 2 || keys[0] != "author" || keys[1] != "status" {
		t.Errorf("keys = %v, want [author status]", keys)
	}
	if len(rest.Content) != 1 || rest.Content[0].Type != "paragraph" {
		t.Errorf("rest doc not stripped: %+v", rest.Content)
	}
}

// TestSplitPropertyTable_IgnoresNonMatchingLeadingTable returns the
// original doc when the first table looks nothing like our marker.
func TestSplitPropertyTable_IgnoresNonMatchingLeadingTable(t *testing.T) {
	table := mdadf.TableNode(
		mdadf.TableRowNode(
			mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Name"))),
			mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Email"))),
		),
	)
	doc := adfDoc(table)
	props, keys, rest := SplitPropertyTable(doc)
	if props != nil || keys != nil {
		t.Errorf("unexpected match: props=%v keys=%v", props, keys)
	}
	if rest != doc {
		t.Errorf("rest should be the original doc when no match")
	}
}
