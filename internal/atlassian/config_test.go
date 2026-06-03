package atlassian

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// withCleanEnv wipes the three Atlassian vars and HOME-style discovery, then
// points XDG at a fresh temp dir. Returns the dotfile path the caller may
// populate to exercise the fallback branch.
func withCleanEnv(t *testing.T) string {
	t.Helper()
	t.Setenv(EnvBaseURL, "")
	t.Setenv(EnvEmail, "")
	t.Setenv(EnvAPIToken, "")
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

// TestLoadConfig_EnvOnly is the baseline: every var set in the environment,
// no dotfile present. Also locks the trailing-slash trim on BaseURL.
func TestLoadConfig_EnvOnly(t *testing.T) {
	withCleanEnv(t)
	t.Setenv(EnvBaseURL, "https://acme.atlassian.net/")
	t.Setenv(EnvEmail, "[email protected]")
	t.Setenv(EnvAPIToken, "tok")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BaseURL != "https://acme.atlassian.net" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", cfg.BaseURL)
	}
	if cfg.Email != "[email protected]" || cfg.APIToken != "tok" {
		t.Errorf("cfg = %+v", cfg)
	}
}

// TestLoadConfig_DotfileFallback fills every var from the XDG dotfile when
// env is unset. Also exercises the comment/blank-line/whitespace handling
// inside parseDotfile via the LoadConfig path.
func TestLoadConfig_DotfileFallback(t *testing.T) {
	path := withCleanEnv(t)
	writeDotfile(t, path,
		"# header\n\nATLASSIAN_BASE_URL=https://acme.atlassian.net\n"+
			"ATLASSIAN_EMAIL = [email protected] \nATLASSIAN_API_TOKEN=tok\n")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BaseURL != "https://acme.atlassian.net" ||
		cfg.Email != "[email protected]" || cfg.APIToken != "tok" {
		t.Errorf("cfg = %+v", cfg)
	}
}

// TestLoadConfig_EnvOverridesDotfile guards the documented precedence: env
// wins per key, but unset keys still fall through to the file.
func TestLoadConfig_EnvOverridesDotfile(t *testing.T) {
	path := withCleanEnv(t)
	writeDotfile(t, path,
		"ATLASSIAN_BASE_URL=https://file.example/\n"+
			"ATLASSIAN_EMAIL=file\nATLASSIAN_API_TOKEN=file\n")
	t.Setenv(EnvBaseURL, "https://env.example")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.BaseURL != "https://env.example" {
		t.Errorf("BaseURL = %q, want env to win", cfg.BaseURL)
	}
	if cfg.Email != "file" || cfg.APIToken != "file" {
		t.Errorf("dotfile values not used for unset env: %+v", cfg)
	}
}

// TestLoadConfig_MissingAll returns *ErrMissingEnv listing every required
// var when neither env nor dotfile provides them.
func TestLoadConfig_MissingAll(t *testing.T) {
	withCleanEnv(t)
	_, err := LoadConfig()
	var me *ErrMissingEnv
	if !errors.As(err, &me) {
		t.Fatalf("err = %v, want *ErrMissingEnv", err)
	}
	for _, want := range []string{EnvBaseURL, EnvEmail, EnvAPIToken} {
		if !containsStr(me.Vars, want) {
			t.Errorf("missing list = %v, want %q present", me.Vars, want)
		}
	}
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
