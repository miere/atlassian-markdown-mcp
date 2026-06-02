// Inline (text + marks) and table rendering for the ADF → markdown path.
// Kept separate from adf_to_md.go for the same reason as the block file.
package markdown

import (
	"encoding/json"
	"strings"
)

// renderInline walks inline content (text, hardBreak, mention, emoji,
// status, plus any unexpected nested marks/inline nodes) and joins it into
// a single line.
func (r *mdRenderer) renderInline(nodes []Node) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case "text":
			b.WriteString(applyMarks(n.Text, n.Marks))
		case "hardBreak":
			b.WriteString("  \n")
		case "mention":
			b.WriteString(renderMention(n))
		case "emoji":
			b.WriteString(renderEmoji(n))
		case "status":
			b.WriteString(renderStatus(n))
		case "inlineCard":
			b.WriteString(renderInlineCard(n))
		default:
			b.WriteString("<!-- unsupported: " + n.Type + " -->")
		}
	}
	return b.String()
}

// applyMarks wraps text with markdown markers for each mark in a fixed
// order: code is innermost (so the surrounding markers do not get treated
// as code), then strike, em, strong, underline (<u> tags), link outermost.
// Marks of unknown type are dropped silently.
func applyMarks(text string, marks []Mark) string {
	if text == "" {
		return ""
	}
	has := func(t string) bool {
		for _, m := range marks {
			if m.Type == t {
				return true
			}
		}
		return false
	}
	out := text
	if has("code") {
		out = "`" + out + "`"
	}
	if has("strike") {
		out = "~~" + out + "~~"
	}
	if has("em") {
		out = "*" + out + "*"
	}
	if has("strong") {
		out = "**" + out + "**"
	}
	if has("underline") {
		out = "<u>" + out + "</u>"
	}
	for _, m := range marks {
		if m.Type == "link" {
			var a LinkAttrs
			_ = json.Unmarshal(m.Attrs, &a)
			out = "[" + out + "](" + a.Href + ")"
			break
		}
	}
	return out
}

// renderMention renders an @-mention as `@displayName`. The internal
// account id is intentionally discarded — round-tripping a mention back to
// Confluence would require an API lookup we deliberately do not perform.
func renderMention(n Node) string {
	var a struct {
		Text string `json:"text"`
		ID   string `json:"id"`
	}
	_ = json.Unmarshal(n.Attrs, &a)
	name := strings.TrimPrefix(a.Text, "@")
	if name == "" {
		name = a.ID
	}
	return "@" + name
}

// renderEmoji renders a Confluence emoji as its shortcode form (`:name:`).
// Falls back to the literal text attribute (often the unicode glyph) when
// no shortName is present.
func renderEmoji(n Node) string {
	var a struct {
		ShortName string `json:"shortName"`
		Text      string `json:"text"`
	}
	_ = json.Unmarshal(n.Attrs, &a)
	if a.ShortName != "" {
		return ":" + strings.Trim(a.ShortName, ":") + ":"
	}
	return a.Text
}

// renderStatus renders a Confluence status lozenge as a backtick-quoted
// `[STATUS:color:text]` placeholder so the visual structure is obvious in
// plain markdown without depending on a renderer.
func renderStatus(n Node) string {
	var a struct {
		Text  string `json:"text"`
		Color string `json:"color"`
	}
	_ = json.Unmarshal(n.Attrs, &a)
	return "`[STATUS:" + a.Color + ":" + a.Text + "]`"
}

// renderInlineCard renders a Confluence inline card as a plain link to its
// URL — the closest lossless markdown equivalent.
func renderInlineCard(n Node) string {
	var a struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(n.Attrs, &a)
	if a.URL == "" {
		return "<!-- unsupported: inlineCard -->"
	}
	return "<" + a.URL + ">"
}

// nodePlainText returns the concatenated text of all descendant text
// nodes. Used both to read property-table cells and to flatten table
// cells when rendering a GFM table (pipe tables cannot host block
// content).
func nodePlainText(n Node) string {
	if n.Type == "text" {
		return n.Text
	}
	var b strings.Builder
	for _, c := range n.Content {
		b.WriteString(nodePlainText(c))
	}
	return strings.TrimSpace(b.String())
}

// prefixLines prepends prefix to every line of s. An empty s yields the
// prefix alone, matching how blockquotes render an empty body.
func prefixLines(s, prefix string) string {
	if s == "" {
		return strings.TrimRight(prefix, " ")
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
