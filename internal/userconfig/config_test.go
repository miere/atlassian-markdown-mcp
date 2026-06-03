package userconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempXDG points XDG_CONFIG_HOME at a fresh temp dir and returns
// the dotfile path inside it so callers can populate the fallback.
func withTempXDG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "obsidian-workspace-mcp", "config")
}

func writeDotfile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestConfigPath_XDGWins locks the XDG-first precedence.
func TestConfigPath_XDGWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got := ConfigPath()
	want := filepath.Join(dir, "obsidian-workspace-mcp", "config")
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

// TestLoadDotfile_Missing returns an empty map (nil) rather than
// erroring when the file does not exist.
func TestLoadDotfile_Missing(t *testing.T) {
	withTempXDG(t)
	if got := LoadDotfile(); len(got) != 0 {
		t.Errorf("LoadDotfile() = %v, want empty", got)
	}
}

// TestLoadDotfile_ParsesKnownShape exercises the full file path: XDG
// resolution + open + parse + KEY=VALUE extraction.
func TestLoadDotfile_ParsesKnownShape(t *testing.T) {
	path := withTempXDG(t)
	writeDotfile(t, path,
		"# header\n\nA=1\nB = two \nOBSIDIAN_VAULT_DIR=~/Vault\n")
	got := LoadDotfile()
	want := map[string]string{"A": "1", "B": "two", "OBSIDIAN_VAULT_DIR": "~/Vault"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestParseDotfile_SkipsBlanksAndComments locks the documented
// dotfile syntax in isolation from the env lookup.
func TestParseDotfile_SkipsBlanksAndComments(t *testing.T) {
	got := parseDotfile(strings.NewReader(
		"# c\n\nA=1\nB =  two \nnoequals\n=val\nC=a=b\n"))
	want := map[string]string{"A": "1", "B": "two", "C": "a=b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

// TestLookup_EnvWins guards the documented precedence: env beats
// the dotfile value for the same key.
func TestLookup_EnvWins(t *testing.T) {
	t.Setenv("FOO", "env")
	fileVals := map[string]string{"FOO": "file"}
	if got := Lookup(fileVals, "FOO"); got != "env" {
		t.Errorf("Lookup = %q, want env", got)
	}
}

// TestLookup_FallbackToFile drops back to the dotfile when env is
// unset (and treats empty env exactly like unset, per the existing
// LoadConfig contract).
func TestLookup_FallbackToFile(t *testing.T) {
	t.Setenv("FOO", "")
	fileVals := map[string]string{"FOO": "file"}
	if got := Lookup(fileVals, "FOO"); got != "file" {
		t.Errorf("Lookup = %q, want file", got)
	}
}
