package obsidian

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/userconfig"
)

// EnvVaultDir is the env var / dotfile key that, when set, becomes
// the base for resolving relative file_path and output_dir values
// passed to the sync tools. An absolute path bypasses this; a
// relative path with no vault configured keeps today's CWD-relative
// behaviour.
const EnvVaultDir = "OBSIDIAN_VAULT_DIR"

// VaultDir returns the configured Obsidian vault root, with a
// leading `~/` expanded against the current user's home directory.
// Returns "" when the var is unset in both the process environment
// and the per-user dotfile, or when the value is set but `~/`
// expansion fails because the home directory cannot be resolved.
//
// Only `~/` (and bare `~`) are expanded; `$VAR`, `~user`, and other
// shell-style constructs are taken verbatim — matching the rest of
// the dotfile contract.
func VaultDir() string {
	raw := userconfig.Lookup(userconfig.LoadDotfile(), EnvVaultDir)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return expandTilde(raw)
}

// expandTilde rewrites a leading `~/` (or bare `~`) to the current
// user's home directory. Anything else is returned verbatim. If the
// home directory cannot be resolved, the original string is returned
// to keep the resolver predictable rather than dropping the value
// silently.
func expandTilde(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// ResolvePath turns a user-supplied file path into the path the tool
// should actually open. Rules:
//
//  1. An empty input is returned verbatim so the calling tool can
//     keep emitting its own "required" error (we don't change error
//     wording for the empty case).
//  2. An absolute path is returned verbatim — OBSIDIAN_VAULT_DIR
//     never overrides an absolute path.
//  3. When OBSIDIAN_VAULT_DIR is unset, the input is returned
//     verbatim so today's CWD-relative behaviour is preserved.
//  4. Otherwise the input is joined onto the vault root.
func ResolvePath(p string) string {
	if p == "" {
		return p
	}
	if filepath.IsAbs(p) {
		return p
	}
	vault := VaultDir()
	if vault == "" {
		return p
	}
	return filepath.Join(vault, p)
}

// ResolveDir is the output_dir variant of ResolvePath. It differs in
// the empty-input branch: when the caller passes no output_dir, the
// vault root takes over from the tool's hard-coded fallback (today
// "/tmp/"). When no vault is configured the fallback wins, matching
// the pre-feature default exactly.
func ResolveDir(p, fallback string) string {
	if p == "" {
		if vault := VaultDir(); vault != "" {
			return vault
		}
		return fallback
	}
	return ResolvePath(p)
}
