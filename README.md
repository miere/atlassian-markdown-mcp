# atlassian-markdown-mcp

A single Go binary that bridges Atlassian (Jira and Confluence) to AI clients
through the Model Context Protocol, exposing the same tools through two
frontends: a human-facing CLI and an MCP server over stdio. Current scope ships
`ping` and `confluence.publish-obsidian-file`, plus the conventions every
future Jira or Confluence tool will follow.

## Build

```sh
go build -o obsidian-workspace-mcp ./cmd/obsidian-workspace-mcp
```

## Usage

### `obsidian-workspace-mcp ping`

Invokes the `ping` tool through the CLI. Writes exactly `pong` followed by a
newline to stdout, and nothing to stderr. Exits 0 on success.

```sh
$ obsidian-workspace-mcp ping
pong
```

### `obsidian-workspace-mcp confluence publish-obsidian-file --file-path <path>`

Reads a local Obsidian markdown file and publishes it to a Confluence Cloud
page, fully rewriting the page body. `--file-path` follows the path
resolution rules below — absolute paths are taken verbatim, relative paths
are resolved against `OBSIDIAN_VAULT_DIR` when set and against the process
CWD otherwise. The source file's YAML frontmatter drives the sync:

| key                  | role                                                                |
| -------------------- | ------------------------------------------------------------------- |
| `confluence_space`   | space key the page lives in (required on first publish only)        |
| `confluence_title`   | page title (required on first publish only)                         |
| `confluence_page_id` | written back on first publish; once present it is the sole identifier |

Once `confluence_page_id` is bound, `confluence_space` and `confluence_title`
may be omitted — the tool updates the page directly by ID and preserves
whatever title currently exists on Confluence (the local `confluence_title`
is ignored on subsequent publishes).

All other frontmatter keys are rendered as a two-column property table that is
prepended to the published page body, so the Confluence page always shows the
note's metadata above its content.

Credentials are resolved per key in this order: the process environment
first, then a per-user dotfile at `$XDG_CONFIG_HOME/obsidian-workspace-mcp/config`
(default `~/.config/obsidian-workspace-mcp/config`) parsed as `KEY=VALUE` lines.
The dotfile is the recommended source when launching the binary from an
MCP client that does not inherit your shell environment.

The required keys are:

- `ATLASSIAN_BASE_URL` — e.g. `https://acme.atlassian.net`
- `ATLASSIAN_EMAIL` — your Atlassian Cloud account email
- `ATLASSIAN_API_TOKEN` — an Atlassian API token (not a password)

## Path resolution

All sync tools accept either a `file_path` (the file to publish/update) or
an `output_dir` (the directory to write a downloaded note into). Both
follow the same rules:

- An **absolute path** is always used verbatim.
- A **relative path** is resolved against `OBSIDIAN_VAULT_DIR` when that
  variable is set; otherwise it falls back to the process's current
  working directory (today's behaviour).
- An **omitted or empty `output_dir`** defaults to `OBSIDIAN_VAULT_DIR`
  when set, and to `/tmp/` otherwise. An omitted `file_path` is still a
  hard error — there is no implicit file name.

`OBSIDIAN_VAULT_DIR` is read using the same precedence as the Atlassian
credentials above: the process environment first, then the per-user
dotfile at `$XDG_CONFIG_HOME/obsidian-workspace-mcp/config`. The value supports
`~/` expansion (so `OBSIDIAN_VAULT_DIR=~/Obsidian/Vault` resolves
against the current user's home directory); no other shell-style
expansion is performed.

This setting is mainly useful for MCP clients (Claude Desktop, etc.)
that launch the binary with a CWD outside the vault — set it once in
the dotfile and every tool accepts vault-relative paths.

### `obsidian-workspace-mcp mcp`

Starts the MCP server over stdio. This mode is intended for MCP clients, not
direct human use — stdout is reserved for JSON-RPC MCP protocol messages and
must not be parsed as plain text. Any diagnostics are written to stderr.

```sh
$ obsidian-workspace-mcp mcp
# server reads JSON-RPC requests from stdin and writes responses to stdout
```

`tools/list` exposes every tool registered with the binary; result payloads
that are plain strings (e.g. `ping → "pong"`) are passed through verbatim,
everything else is JSON-marshalled into a single `TextContent` block.

Tool names on the MCP wire use `_` instead of `-` (the CLI form is
unchanged). For example:

| CLI subcommand                              | MCP wire name                         |
| ------------------------------------------- | ------------------------------------- |
| `confluence publish-obsidian-file`          | `confluence.publish_obsidian_file`    |
| `confluence download-page`                  | `confluence.download_page`            |
| `jira get-ticket`                           | `jira.get_ticket`                     |
| `jira update-ticket`                        | `jira.update_ticket`                  |
| `ping`                                      | `ping`                                |

Invalid commands exit non-zero and write a short diagnostic to stderr.

## Repository layout

```
atlassian-markdown-mcp/
├── cmd/obsidian-workspace-mcp/   # entry point
├── internal/app/        # composition root
├── internal/frontends/  # cli + mcp adapters
└── internal/tools/      # shared tool interface + one package per tool
```

See `ARCHITECTURE.md` for the full design rationale.

## Development

```sh
gofmt -w .
go vet ./...
go test ./...
```
