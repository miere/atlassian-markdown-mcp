# atlassian-markdown-mcp

A single Go binary that bridges Atlassian (Jira and Confluence) to AI clients
through the Model Context Protocol, exposing the same tools through two
frontends: a human-facing CLI and an MCP server over stdio. The initial scope
ships the structural skeleton with one capability — `ping` — and the
conventions every future Jira or Confluence tool will follow.

## Build

```sh
go build -o atlassian-mcp ./cmd/atlassian-mcp
```

## Usage

The binary exposes exactly two top-level commands in this scope:

### `atlassian-mcp ping`

Invokes the `ping` tool through the CLI. Writes exactly `pong` followed by a
newline to stdout, and nothing to stderr. Exits 0 on success.

```sh
$ atlassian-mcp ping
pong
```

### `atlassian-mcp mcp`

Starts the MCP server over stdio. This mode is intended for MCP clients, not
direct human use — stdout is reserved for JSON-RPC MCP protocol messages and
must not be parsed as plain text. Any diagnostics are written to stderr.

```sh
$ atlassian-mcp mcp
# server reads JSON-RPC requests from stdin and writes responses to stdout
```

In this initial scope `tools/list` returns exactly one tool, `ping`, whose
result content is the text `pong`.

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
