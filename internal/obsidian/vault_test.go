package obsidian

import (
	"os"
	"path/filepath"
	"testing"
)

// cleanVaultEnv blanks OBSIDIAN_VAULT_DIR and points XDG at an empty
// temp dir so neither the env nor the dotfile leaks into the test.
func cleanVaultEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvVaultDir, "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// TestVaultDir_Unset returns the empty string when neither env nor
// dotfile carries the vault path.
func TestVaultDir_Unset(t *testing.T) {
	cleanVaultEnv(t)
	if got := VaultDir(); got != "" {
		t.Errorf("VaultDir() = %q, want empty", got)
	}
}

// TestVaultDir_EnvVerbatim takes a non-tilde value as-is.
func TestVaultDir_EnvVerbatim(t *testing.T) {
	cleanVaultEnv(t)
	t.Setenv(EnvVaultDir, "/srv/vault")
	if got := VaultDir(); got != "/srv/vault" {
		t.Errorf("VaultDir() = %q, want /srv/vault", got)
	}
}

// TestVaultDir_TildeExpanded rewrites a leading `~/` against the
// process home directory.
func TestVaultDir_TildeExpanded(t *testing.T) {
	cleanVaultEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvVaultDir, "~/Vault")
	want := filepath.Join(home, "Vault")
	if got := VaultDir(); got != want {
		t.Errorf("VaultDir() = %q, want %q", got, want)
	}
}

// TestResolvePath_EmptyPassthrough keeps the empty-string contract
// so callers' "required" error wording stays identical.
func TestResolvePath_EmptyPassthrough(t *testing.T) {
	cleanVaultEnv(t)
	t.Setenv(EnvVaultDir, "/v")
	if got := ResolvePath(""); got != "" {
		t.Errorf("ResolvePath(\"\") = %q, want empty", got)
	}
}

// TestResolvePath_AbsoluteNeverPrefixed guarantees an absolute path
// is never joined onto the vault root.
func TestResolvePath_AbsoluteNeverPrefixed(t *testing.T) {
	cleanVaultEnv(t)
	t.Setenv(EnvVaultDir, "/vault")
	if got := ResolvePath("/abs/path.md"); got != "/abs/path.md" {
		t.Errorf("ResolvePath = %q, want verbatim", got)
	}
}

// TestResolvePath_NoVaultIsCWDRelative preserves today's behaviour
// when the env var is absent.
func TestResolvePath_NoVaultIsCWDRelative(t *testing.T) {
	cleanVaultEnv(t)
	if got := ResolvePath("notes/foo.md"); got != "notes/foo.md" {
		t.Errorf("ResolvePath = %q, want verbatim", got)
	}
}

// TestResolvePath_VaultRelativeJoined exercises the headline case.
func TestResolvePath_VaultRelativeJoined(t *testing.T) {
	cleanVaultEnv(t)
	t.Setenv(EnvVaultDir, "/vault")
	want := filepath.Join("/vault", "notes/foo.md")
	if got := ResolvePath("notes/foo.md"); got != want {
		t.Errorf("ResolvePath = %q, want %q", got, want)
	}
}

// TestResolveDir_EmptyDefaultsToVault locks the headline default-
// switching behaviour: when the caller leaves output_dir blank and a
// vault is configured, the vault root wins over the hard-coded
// fallback.
func TestResolveDir_EmptyDefaultsToVault(t *testing.T) {
	cleanVaultEnv(t)
	t.Setenv(EnvVaultDir, "/vault")
	if got := ResolveDir("", "/tmp/"); got != "/vault" {
		t.Errorf("ResolveDir = %q, want /vault", got)
	}
}

// TestResolveDir_EmptyFallsBack keeps /tmp/ as the default when no
// vault is configured, matching the pre-feature behaviour exactly.
func TestResolveDir_EmptyFallsBack(t *testing.T) {
	cleanVaultEnv(t)
	if got := ResolveDir("", "/tmp/"); got != "/tmp/" {
		t.Errorf("ResolveDir = %q, want /tmp/", got)
	}
}

// TestResolveDir_RelativeJoined behaves exactly like ResolvePath for
// non-empty inputs.
func TestResolveDir_RelativeJoined(t *testing.T) {
	cleanVaultEnv(t)
	t.Setenv(EnvVaultDir, "/vault")
	want := filepath.Join("/vault", "subdir")
	if got := ResolveDir("subdir", "/tmp/"); got != want {
		t.Errorf("ResolveDir = %q, want %q", got, want)
	}
}

// TestResolveDir_AbsoluteVerbatim — same passthrough as ResolvePath.
func TestResolveDir_AbsoluteVerbatim(t *testing.T) {
	cleanVaultEnv(t)
	t.Setenv(EnvVaultDir, "/vault")
	if got := ResolveDir("/abs/dir", "/tmp/"); got != "/abs/dir" {
		t.Errorf("ResolveDir = %q, want /abs/dir", got)
	}
}

// Sanity: make sure os.UserHomeDir resolves against the HOME env var
// on darwin/linux so the tilde test is portable.
func TestExpandTilde_BareTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := expandTilde("~"); got != home {
		t.Errorf("expandTilde(~) = %q, want %q", got, home)
	}
}

// TestExpandTilde_NoOpOnAbsolute leaves absolute paths untouched.
func TestExpandTilde_NoOpOnAbsolute(t *testing.T) {
	if got := expandTilde("/abs"); got != "/abs" {
		t.Errorf("expandTilde = %q, want /abs", got)
	}
}

// silence unused-import vet in case test file is later trimmed.
var _ = os.Getenv
