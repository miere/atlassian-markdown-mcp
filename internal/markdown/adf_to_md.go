// ADF → markdown rendering, the inverse of the markdown→ADF flow handled by
// converter.go. Used by `confluence.download-page` to materialise a remote
// Confluence page as a local Obsidian-friendly markdown file.
//
// Coverage parity with what mdadf can emit (paragraph, headings, lists, code
// blocks, blockquotes, rules, hard breaks, GFM tables, the five base marks +
// link), plus a small set of Confluence-only nodes that show up in
// human-edited pages: panel (Obsidian callout), expand, taskList/taskItem,
// mention, emoji, status. Unknown nodes degrade to
// `<!-- unsupported: <type> -->` so the converter never fails on novel
// content.
package markdown

import "strings"

// RenderMarkdown serialises an ADF document as markdown. The output ends
// with a single trailing newline iff there is any content.
func RenderMarkdown(doc *Document) string {
	if doc == nil || len(doc.Content) == 0 {
		return ""
	}
	r := &mdRenderer{}
	return r.renderBlocks(doc.Content) + "\n"
}

// SplitPropertyTable detects the property table that
// `confluence.publish-obsidian-file` prepends to a page and lifts it back
// into a map. It matches iff the document's first block is a `table` whose
// first row consists of exactly two `tableHeader` cells with plain text
// "Property" and "Value" (the exact shape PrependPropertyTable emits).
//
// On a match it returns (props, orderedKeys, docWithoutTable). props maps
// each row's left cell to its right cell as plain text; orderedKeys
// preserves the row order so the YAML frontmatter we emit stays stable
// across round-trips. On no match it returns (nil, nil, doc) unchanged.
func SplitPropertyTable(doc *Document) (map[string]string, []string, *Document) {
	if doc == nil || len(doc.Content) == 0 {
		return nil, nil, doc
	}
	first := doc.Content[0]
	if first.Type != "table" || len(first.Content) < 1 {
		return nil, nil, doc
	}
	header := first.Content[0]
	if header.Type != "tableRow" || len(header.Content) != 2 {
		return nil, nil, doc
	}
	h1, h2 := header.Content[0], header.Content[1]
	if h1.Type != "tableHeader" || h2.Type != "tableHeader" {
		return nil, nil, doc
	}
	if nodePlainText(h1) != "Property" || nodePlainText(h2) != "Value" {
		return nil, nil, doc
	}
	props := map[string]string{}
	keys := make([]string, 0, len(first.Content)-1)
	for _, row := range first.Content[1:] {
		if row.Type != "tableRow" || len(row.Content) < 2 {
			continue
		}
		k := nodePlainText(row.Content[0])
		v := nodePlainText(row.Content[1])
		if k == "" {
			continue
		}
		props[k] = v
		keys = append(keys, k)
	}
	rest := &Document{Version: doc.Version, Type: doc.Type, Content: doc.Content[1:]}
	if rest.Version == 0 {
		rest.Version = 1
	}
	if rest.Type == "" {
		rest.Type = "doc"
	}
	return props, keys, rest
}

// mdRenderer is a single-use renderer. It carries no state beyond the
// methods themselves; each renderBlock call computes its own indent and
// blockquote prefix from its arguments.
type mdRenderer struct{}

// renderBlocks joins block-level nodes with a blank line separator. Nodes
// that render to an empty string (e.g. unknown nodes silently skipped by a
// caller — not the default behaviour) are dropped so we do not emit
// stray double-newlines.
func (r *mdRenderer) renderBlocks(nodes []Node) string {
	parts := make([]string, 0, len(nodes))
	for _, n := range nodes {
		s := r.renderBlock(n)
		if s == "" {
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n")
}

// renderBlock dispatches on node type for block-level rendering. Inline
// nodes (text, hardBreak, mention, emoji, status) reaching this path are
// wrapped in an implicit paragraph so the output is always block-shaped.
func (r *mdRenderer) renderBlock(n Node) string {
	switch n.Type {
	case "paragraph":
		return r.renderInline(n.Content)
	case "heading":
		return r.renderHeading(n)
	case "bulletList", "orderedList":
		return r.renderList(n, "")
	case "taskList":
		return r.renderTaskList(n, "")
	case "codeBlock":
		return r.renderCodeBlock(n)
	case "blockquote":
		return prefixLines(r.renderBlocks(n.Content), "> ")
	case "rule":
		return "---"
	case "table":
		return r.renderTable(n)
	case "panel":
		return r.renderPanel(n)
	case "expand", "nestedExpand":
		return r.renderExpand(n)
	default:
		return r.renderInline([]Node{n})
	}
}
