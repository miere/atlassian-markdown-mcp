// Package markdown wraps the third-party markdown→ADF conversion behind a
// single Converter interface so the rest of the codebase has one seam. Tools
// that publish content to Atlassian use Default(); tests substitute their own
// Converter implementation.
package markdown

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ericmason/mdadf"
)

// Document is the ADF root document a Converter returns.
type Document = mdadf.Document

// Node is an ADF node (block or inline).
type Node = mdadf.Node

// Mark is an ADF mark (bold, italic, link, ...). Aliased here so the ADF →
// markdown renderer in this package can reference it without importing
// mdadf directly.
type Mark = mdadf.Mark

// Attribute payload types re-exported for the ADF → markdown renderer.
type (
	HeadingAttrs     = mdadf.HeadingAttrs
	CodeBlockAttrs   = mdadf.CodeBlockAttrs
	LinkAttrs        = mdadf.LinkAttrs
	OrderedListAttrs = mdadf.OrderedListAttrs
	TaskItemAttrs    = mdadf.TaskItemAttrs
)

// Converter turns a UTF-8 markdown body into an ADF document.
type Converter interface {
	Convert(body []byte) (*Document, error)
}

// MDADFConverter is the default Converter, backed by ericmason/mdadf.
type MDADFConverter struct{}

// Default returns the package default Converter.
func Default() Converter { return MDADFConverter{} }

// Convert parses body as CommonMark + GFM and returns an ADF document.
// An empty body yields an empty doc (version 1, type "doc", no content).
func (MDADFConverter) Convert(body []byte) (*Document, error) {
	doc, err := mdadf.ConvertToDoc(string(body))
	if err != nil {
		return nil, fmt.Errorf("markdown: convert: %w", err)
	}
	if doc == nil {
		return mdadf.NewDocument(), nil
	}
	return doc, nil
}

// PrependPropertyTable returns a copy of doc with a two-column header+rows
// table inserted before the existing content. Keys are sorted for
// determinism. Values are rendered with FormatValue. An empty props map is
// a no-op (the original doc is returned unchanged).
func PrependPropertyTable(doc *Document, props map[string]any) *Document {
	if len(props) == 0 {
		return doc
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]Node, 0, len(keys)+1)
	rows = append(rows, mdadf.TableRowNode(
		mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Property", mdadf.StrongMark()))),
		mdadf.TableHeaderNode(mdadf.ParagraphNode(mdadf.TextNode("Value", mdadf.StrongMark()))),
	))
	for _, k := range keys {
		rows = append(rows, mdadf.TableRowNode(
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode(k))),
			mdadf.TableCellNode(mdadf.ParagraphNode(mdadf.TextNode(FormatValue(props[k])))),
		))
	}
	table := mdadf.TableNode(rows...)

	out := &Document{Version: doc.Version, Type: doc.Type}
	if out.Version == 0 {
		out.Version = 1
	}
	if out.Type == "" {
		out.Type = "doc"
	}
	out.Content = append([]Node{table}, doc.Content...)
	return out
}

// FormatValue renders an arbitrary frontmatter value as plain text suitable
// for a property-table cell. Scalars become their native string form;
// sequences become comma-joined; maps and anything else fall back to
// compact JSON so the user can still read the data.
func FormatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		// integers parsed as float64 (json default) should print without ".0"
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = FormatValue(e)
		}
		return strings.Join(parts, ", ")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
