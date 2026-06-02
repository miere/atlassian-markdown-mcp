# atlassian-markdown-mcp

A single Go binary that bridges Atlassian (Jira and Confluence) to AI clients
through the Model Context Protocol, exposing the same tools through two
frontends: a human-facing CLI and an MCP server over stdio. Current scope ships
`ping` and `confluence.publish-obsidian-file`, plus the conventions every
future Jira or Confluence tool will follow.

## Build

```sh
go build -o atlassian-mcp ./cmd/atlassian-mcp
```

## Usage

### `atlassian-mcp ping`

Invokes the `ping` tool through the CLI. Writes exactly `pong` followed by a
newline to stdout, and nothing to stderr. Exits 0 on success.

```sh
$ atlassian-mcp ping
pong
```

### `atlassian-mcp confluence publish-obsidian-file --file-path <path>`

Reads a local Obsidian markdown file and publishes it to a Confluence Cloud
page, fully rewriting the page body. The source file's YAML frontmatter drives
the sync:

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
first, then a per-user dotfile at `$XDG_CONFIG_HOME/atlassian-mcp/config`
(default `~/.config/atlassian-mcp/config`) parsed as `KEY=VALUE` lines.
The dotfile is the recommended source when launching the binary from an
MCP client that does not inherit your shell environment.

The required keys are:

- `ATLASSIAN_BASE_URL` — e.g. `https://acme.atlassian.net`
- `ATLASSIAN_EMAIL` — your Atlassian Cloud account email
- `ATLASSIAN_API_TOKEN` — an Atlassian API token (not a password)

### `atlassian-mcp mcp`

Starts the MCP server over stdio. This mode is intended for MCP clients, not
direct human use — stdout is reserved for JSON-RPC MCP protocol messages and
must not be parsed as plain text. Any diagnostics are written to stderr.

```sh
$ atlassian-mcp mcp
# server reads JSON-RPC requests from stdin and writes responses to stdout
```

`tools/list` exposes every tool registered with the binary; result payloads
that are plain strings (e.g. `ping → "pong"`) are passed through verbatim,
everything else is JSON-marshalled into a single `TextContent` block.

Invalid commands exit non-zero and write a short diagnostic to stderr.

## Repository layout

```
atlassian-markdown-mcp/
├── cmd/atlassian-mcp/   # entry point
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
