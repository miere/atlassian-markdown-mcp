// Block-shape renderers shared by RenderMarkdown. Split off from
// adf_to_md.go so each file fits comfortably in a single screen.
package markdown

import (
	"encoding/json"
	"fmt"
	"strings"
)

// renderHeading renders a heading node as the matching number of `#`
// markers followed by the inline content. Levels outside 1..6 are clamped
// so we never emit a 0- or 7-`#` line.
func (r *mdRenderer) renderHeading(n Node) string {
	lvl := 1
	if len(n.Attrs) > 0 {
		var a HeadingAttrs
		_ = json.Unmarshal(n.Attrs, &a)
		if a.Level >= 1 && a.Level <= 6 {
			lvl = a.Level
		}
	}
	return strings.Repeat("#", lvl) + " " + r.renderInline(n.Content)
}

// renderList renders a bullet or ordered list. indent is the leading
// whitespace already imposed by an enclosing list (two spaces per level),
// used so nested lists line up correctly under their parent item.
func (r *mdRenderer) renderList(n Node, indent string) string {
	ordered := n.Type == "orderedList"
	start := 1
	if ordered && len(n.Attrs) > 0 {
		var a OrderedListAttrs
		_ = json.Unmarshal(n.Attrs, &a)
		if a.Order > 0 {
			start = a.Order
		}
	}
	lines := make([]string, 0, len(n.Content))
	for i, item := range n.Content {
		if item.Type != "listItem" {
			continue
		}
		marker := "- "
		if ordered {
			marker = fmt.Sprintf("%d. ", start+i)
		}
		lines = append(lines, r.renderListItem(item, indent, marker))
	}
	return strings.Join(lines, "\n")
}

// renderListItem renders a single list item. The first block child carries
// the bullet marker; subsequent children (nested lists, additional
// paragraphs) are indented to align under the first child's text.
func (r *mdRenderer) renderListItem(n Node, indent, marker string) string {
	childIndent := indent + strings.Repeat(" ", len(marker))
	parts := make([]string, 0, len(n.Content))
	for i, c := range n.Content {
		var body string
		// Lists are rendered with the indent threaded through, so their
		// output is already aligned and must not be re-prefixed below.
		preIndented := false
		switch c.Type {
		case "bulletList", "orderedList":
			body = r.renderList(c, childIndent)
			preIndented = true
		case "taskList":
			body = r.renderTaskList(c, childIndent)
			preIndented = true
		default:
			body = r.renderBlock(c)
		}
		switch {
		case i == 0:
			parts = append(parts, indent+marker+body)
		case preIndented:
			parts = append(parts, body)
		default:
			parts = append(parts, prefixLines(body, childIndent))
		}
	}
	return strings.Join(parts, "\n")
}

// renderTaskList renders a taskList as a sequence of `- [ ]` / `- [x]`
// items. Nested taskLists are supported via the same indent mechanism as
// bullet lists.
func (r *mdRenderer) renderTaskList(n Node, indent string) string {
	lines := make([]string, 0, len(n.Content))
	for _, item := range n.Content {
		if item.Type != "taskItem" {
			continue
		}
		checked := false
		if len(item.Attrs) > 0 {
			var a TaskItemAttrs
			_ = json.Unmarshal(item.Attrs, &a)
			checked = a.State == "DONE"
		}
		box := "[ ]"
		if checked {
			box = "[x]"
		}
		body := r.renderInline(item.Content)
		lines = append(lines, indent+"- "+box+" "+body)
	}
	return strings.Join(lines, "\n")
}

// renderCodeBlock renders a fenced code block. The language attribute, if
// any, is emitted on the opening fence; the body is the verbatim
// concatenation of any text children (mdadf only ever emits one).
func (r *mdRenderer) renderCodeBlock(n Node) string {
	lang := ""
	if len(n.Attrs) > 0 {
		var a CodeBlockAttrs
		_ = json.Unmarshal(n.Attrs, &a)
		lang = a.Language
	}
	var body strings.Builder
	for _, c := range n.Content {
		if c.Type == "text" {
			body.WriteString(c.Text)
		}
	}
	return "```" + lang + "\n" + body.String() + "\n```"
}

// renderPanel renders a Confluence panel as an Obsidian callout: a
// blockquote whose first line is `[!TYPE]` (uppercased panelType). Falls
// back to `[!NOTE]` when the panelType attribute is missing or unknown.
func (r *mdRenderer) renderPanel(n Node) string {
	pt := "note"
	if len(n.Attrs) > 0 {
		var a struct {
			PanelType string `json:"panelType"`
		}
		_ = json.Unmarshal(n.Attrs, &a)
		if a.PanelType != "" {
			pt = a.PanelType
		}
	}
	body := r.renderBlocks(n.Content)
	header := "[!" + strings.ToUpper(pt) + "]"
	if body == "" {
		return "> " + header
	}
	return prefixLines(header+"\n"+body, "> ")
}

// renderExpand renders a Confluence expand/nestedExpand block as the HTML
// <details> element Obsidian recognises. The title attribute becomes the
// <summary>; an empty title is omitted entirely.
func (r *mdRenderer) renderExpand(n Node) string {
	title := ""
	if len(n.Attrs) > 0 {
		var a struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(n.Attrs, &a)
		title = a.Title
	}
	body := r.renderBlocks(n.Content)
	if title == "" {
		return "<details>\n" + body + "\n</details>"
	}
	return "<details><summary>" + title + "</summary>\n" + body + "\n</details>"
}

// renderTable renders a `table` node as a GFM pipe table. Cells are
// flattened to their plain text via nodePlainText because pipe tables
// cannot host block content. The first row is always treated as the
// header (matching how Confluence emits its tables); a synthetic header
// is generated when the first row has no tableHeader cells, since GFM
// requires the separator line.
func (r *mdRenderer) renderTable(n Node) string {
	rows := make([][]string, 0, len(n.Content))
	cols := 0
	for _, row := range n.Content {
		if row.Type != "tableRow" {
			continue
		}
		cells := make([]string, 0, len(row.Content))
		for _, c := range row.Content {
			cells = append(cells, escapePipe(nodePlainText(c)))
		}
		if len(cells) > cols {
			cols = len(cells)
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 || cols == 0 {
		return ""
	}
	for i := range rows {
		for len(rows[i]) < cols {
			rows[i] = append(rows[i], "")
		}
	}
	var b strings.Builder
	b.WriteString("| " + strings.Join(rows[0], " | ") + " |\n")
	b.WriteString("|" + strings.Repeat(" --- |", cols) + "\n")
	for _, row := range rows[1:] {
		b.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// escapePipe escapes the `|` byte so it does not break out of a GFM
// table cell. Newlines inside a cell are replaced with a `<br>` for the
// same reason — pipe tables are strictly one row per source line.
func escapePipe(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}
