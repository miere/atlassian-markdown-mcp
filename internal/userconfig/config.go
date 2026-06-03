// Package userconfig centralises per-user dotfile lookup so any domain
// package can share the same XDG-aware location and parser. The file
// format is `KEY=VALUE` lines; `#` comments and blank lines are ignored.
// No quoting and no interpolation are performed — values are taken
// verbatim (after trimming surrounding whitespace).
package userconfig

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ConfigPath returns the per-user dotfile location following XDG. An
// empty string means we couldn't determine a home directory; callers
// should treat that as "no dotfile available" and skip the fallback.
func ConfigPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "atlassian-mcp", "config")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "atlassian-mcp", "config")
}

// LoadDotfile best-effort parses the per-user config. A missing file
// or unreadable home directory is not an error — the returned map is
// just empty so callers fall through to their other sources.
func LoadDotfile() map[string]string {
	path := ConfigPath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseDotfile(f)
}

// parseDotfile reads KEY=VALUE lines from r. Blank lines and `#`
// comments are skipped; lines without `=` are ignored. Values are
// trimmed of surrounding whitespace; no quoting or interpolation
// is performed.
func parseDotfile(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// Lookup returns the value for key, preferring the process environment
// over the dotfile. fileVals is typically the result of LoadDotfile;
// passing it in (rather than re-loading per call) lets callers do a
// single disk read for several keys.
func Lookup(fileVals map[string]string, key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fileVals[key]
}
