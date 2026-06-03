// Package atlassian wraps Atlassian Cloud REST integration (Confluence v2 in
// this scope). Credentials are loaded lazily from the environment, so tools
// that need them only fail when actually invoked — ping and mcp run fine
// without any Atlassian configuration.
package atlassian

import (
	"fmt"
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/userconfig"
)

// Env var names for Atlassian Cloud Basic-auth-with-API-token.
const (
	EnvBaseURL  = "ATLASSIAN_BASE_URL"
	EnvEmail    = "ATLASSIAN_EMAIL"
	EnvAPIToken = "ATLASSIAN_API_TOKEN"
)

// Config holds the credentials needed to talk to Atlassian Cloud.
type Config struct {
	// BaseURL is the workspace base, e.g. https://acme.atlassian.net.
	// Stored with no trailing slash.
	BaseURL string
	// Email is the user's Atlassian Cloud account email.
	Email string
	// APIToken is an Atlassian API token (NOT a password).
	APIToken string
}

// LoadConfig reads Config from the environment, falling back to a per-user
// dotfile at $XDG_CONFIG_HOME/atlassian-mcp/config (or
// ~/.config/atlassian-mcp/config when XDG_CONFIG_HOME is unset) when an env
// var is missing. The dotfile uses `KEY=VALUE` lines; `#` comments and blank
// lines are ignored. Returns *ErrMissingEnv when any required variable
// remains unset after both lookups.
func LoadConfig() (Config, error) {
	fileVals := userconfig.LoadDotfile()
	get := func(key string) string { return userconfig.Lookup(fileVals, key) }
	cfg := Config{
		BaseURL:  strings.TrimRight(get(EnvBaseURL), "/"),
		Email:    get(EnvEmail),
		APIToken: get(EnvAPIToken),
	}
	var missing []string
	if cfg.BaseURL == "" {
		missing = append(missing, EnvBaseURL)
	}
	if cfg.Email == "" {
		missing = append(missing, EnvEmail)
	}
	if cfg.APIToken == "" {
		missing = append(missing, EnvAPIToken)
	}
	if len(missing) > 0 {
		return Config{}, &ErrMissingEnv{Vars: missing}
	}
	return cfg, nil
}

// ErrMissingEnv is returned when one or more required env vars are unset.
type ErrMissingEnv struct {
	Vars []string
}

func (e *ErrMissingEnv) Error() string {
	return fmt.Sprintf("atlassian: missing required env vars: %s",
		strings.Join(e.Vars, ", "))
}
