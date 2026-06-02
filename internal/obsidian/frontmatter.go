// Package obsidian loads Obsidian-style markdown files: an optional YAML
// frontmatter block between `---` fences, followed by the body. It supports
// targeted in-place updates of a single frontmatter key while preserving the
// formatting of every other line (comments, key order, blank lines).
package obsidian

import (
	"bytes"
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// File is a parsed Obsidian markdown file.
type File struct {
	Path        string         // absolute or relative path the file was read from
	Raw         []byte         // original file bytes, refreshed by SetFrontmatterKey
	Frontmatter map[string]any // parsed YAML; empty when there is no frontmatter
	Body        string         // bytes after the closing fence (or the full file)

	hasFrontmatter bool
	openLine       int // line index of the opening `---`
	closeLine      int // line index of the closing `---`
}

// Load reads the file at path and parses its YAML frontmatter, if any. A file
// without a frontmatter block is returned with an empty Frontmatter map and
// Body equal to the full file contents.
func Load(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("obsidian: read %s: %w", path, err)
	}
	f := &File{Path: path, Raw: raw, Body: string(raw), Frontmatter: map[string]any{}}
	lines := splitLines(raw)
	if len(lines) == 0 || string(lines[0]) != "---" {
		return f, nil
	}
	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if string(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		return f, nil // unterminated frontmatter — treat as plain markdown
	}
	fmYAML := bytes.Join(lines[1:closeIdx], []byte("\n"))
	var fm map[string]any
	if err := yaml.Unmarshal(fmYAML, &fm); err != nil {
		return nil, fmt.Errorf("obsidian: parse frontmatter in %s: %w", path, err)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	f.Frontmatter = fm
	f.hasFrontmatter = true
	f.openLine = 0
	f.closeLine = closeIdx
	bodyStart := closeIdx + 1
	if bodyStart < len(lines) {
		f.Body = string(bytes.Join(lines[bodyStart:], []byte("\n")))
	} else {
		f.Body = ""
	}
	return f, nil
}

// SetFrontmatterKey writes key: value into the file's frontmatter block,
// in-place on disk. If the file had no frontmatter block, one is added at
// the top. If the key already exists, the existing line is replaced;
// otherwise the key is inserted just before the closing fence.
//
// Only scalar values are intended (string, int, bool); other types are
// rendered with fmt.Sprintf("%v", value).
func (f *File) SetFrontmatterKey(key string, value any) error {
	scalar := scalarYAML(value)
	lines := splitLines(f.Raw)

	if !f.hasFrontmatter {
		block := [][]byte{[]byte("---"), []byte(key + ": " + scalar), []byte("---"), nil}
		merged := bytes.Join(append(block, lines...), []byte("\n"))
		return f.writeBack(merged)
	}

	keyRE := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `\s*:`)
	replaced := false
	for i := f.openLine + 1; i < f.closeLine; i++ {
		if keyRE.Match(lines[i]) {
			lines[i] = []byte(key + ": " + scalar)
			replaced = true
			break
		}
	}
	if !replaced {
		newLine := []byte(key + ": " + scalar)
		inserted := make([][]byte, 0, len(lines)+1)
		inserted = append(inserted, lines[:f.closeLine]...)
		inserted = append(inserted, newLine)
		inserted = append(inserted, lines[f.closeLine:]...)
		lines = inserted
		f.closeLine++
	}
	return f.writeBack(bytes.Join(lines, []byte("\n")))
}

func (f *File) writeBack(content []byte) error {
	if len(f.Raw) > 0 && f.Raw[len(f.Raw)-1] == '\n' &&
		(len(content) == 0 || content[len(content)-1] != '\n') {
		content = append(content, '\n')
	}
	if err := os.WriteFile(f.Path, content, 0o644); err != nil {
		return fmt.Errorf("obsidian: write %s: %w", f.Path, err)
	}
	f.Raw = content
	return nil
}

// scalarYAML renders a Go value as a YAML scalar suitable for the right-hand
// side of `key: ...`. Strings that would be misparsed as another type or that
// contain YAML-significant characters are quoted.
func scalarYAML(v any) string {
	switch x := v.(type) {
	case string:
		if needsQuoting(x) {
			return fmt.Sprintf("%q", x)
		}
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	switch s {
	case "true", "false", "null", "yes", "no", "on", "off":
		return true
	}
	for _, r := range s {
		if r == ':' || r == '#' || r == '\n' || r == '"' || r == '\'' {
			return true
		}
	}
	return false
}

func splitLines(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	parts := bytes.Split(b, []byte("\n"))
	if len(parts) > 1 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	return parts
}
