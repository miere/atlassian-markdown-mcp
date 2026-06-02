# atlassian-markdown-mcp — Architecture

This document is the canonical reference for the structural decisions in
atlassian-markdown-mcp. Drift between code and this document is treated as a
bug. Any change that adds or modifies an architectural element MUST update this
file in the same PR.

## 1. High-level architecture

The project has a single binary (`cmd/atlassian-mcp`) and two frontends backed
by a shared tool registry:

```
                       ┌──────────────────────┐
                       │ cmd/atlassian-mcp    │
                       └──────────┬───────────┘
                                  │
                       ┌──────────▼───────────┐
                       │ internal/app         │  ← composition root
                       └──────────┬───────────┘
                                  │ builds Registry, picks Mode
                  ┌───────────────┴───────────────┐
                  │                               │
         ┌────────▼─────────┐           ┌─────────▼────────┐
         │ frontends/cli    │           │ frontends/mcp    │
         │ (human stdin/out)│           │ (MCP stdio JSON) │
         └────────┬─────────┘           └─────────┬────────┘
                  │                               │
                  └──────────────► Tool ◄─────────┘
                                   (internal/tools)
```

- `internal/app` is the only place tools are wired into the registry.
- Frontends know nothing about each other and reach tools only through
  `tools.Tool`.

## 2. The `Tool` contract

```go
type Tool interface {
    Name() string
    Description() string
    InputSchema() *jsonschema.Schema
    Invoke(ctx context.Context, args map[string]any) (any, error)
}
```

Semantics:

- **`Name`** is the registry key. A `.`-separated name (e.g.
  `jira.fetch-ticket`) declares the tool as belonging to a namespace; the CLI
  frontend resolves `atlassian-mcp jira fetch-ticket` to the registered name
  `jira.fetch-ticket`.
- **`Description`** is a one-line human-readable hint used by MCP clients.
- **`InputSchema`** returns the JSON Schema that documents and validates the
  tool's parameters. The schema MUST have `Type: "object"`. Returning `nil`
  means the tool takes no parameters; frontends pass `nil` (or an empty
  object schema, for MCP) accordingly.
- **`Invoke`** receives args keyed by the JSON-Schema property names declared
  on `InputSchema`. The CLI frontend already maps `--kebab-case` flags to
  `snake_case` keys before invocation, so tools never see flag spellings.

The `Registry` (in `internal/tools`) is constructed in `internal/app.New` and
is the **single source of registered tools**. Tools register themselves there
and only there; frontends iterate it but do not mutate it.

## 3. Frontend conventions

### CLI (`internal/frontends/cli`)

- **stdout** is reserved for tool output. **stderr** is for diagnostics
  (`atlassian-mcp: <error>`). Exit code is `1` on any tool error or usage
  error.
- **Nested namespaces** use `.`-separated registry names
  (`<namespace>.<command>`). The CLI first tries a flat lookup of `args[0]`,
  then a dotted lookup of `args[0] + "." + args[1]`.
- **Flag parsing** maps `--kebab-case` → `snake_case` keys; values are
  coerced to the property type declared in the schema (string, integer,
  number, boolean). Booleans take an explicit value (`--verbose true`); there
  are no positional arguments.
- **Result rendering** dispatches by type via `cli.Render`: `string` and
  `[]string` are written as-is, anything implementing `fmt.Stringer` uses
  `String()`, everything else falls back to `%v`. Tools that own a rich
  result struct provide a `String()` method for the CLI representation and
  let the MCP frontend JSON-marshal the same struct.

### MCP (`internal/frontends/mcp`)

- **stdout** is reserved for MCP protocol traffic. No logs, no raw text.
- Every registered tool is exposed; the tool's `InputSchema()` is published
  verbatim to clients (an empty `{"type":"object"}` schema is substituted
  when the tool returns `nil`).
- **Structured-JSON output convention**: tool results are JSON-marshalled
  and wrapped in a single `TextContent` block. A plain string result is
  passed through as-is so trivial tools (e.g. `ping`) stay uncluttered.
- Tool errors are returned as
  `CallToolResult{IsError: true, Content: [TextContent{err}]}`.

## 4. Package layout

```
cmd/atlassian-mcp/               # main; top-level mode parsing
internal/
  app/                           # composition root + Registry wiring
  frontends/
    cli/                         # human frontend
    mcp/                         # MCP stdio frontend
  tools/
    tool.go                      # Tool interface + Registry
    <tool>/                      # flat tool (e.g. ping)
    <namespace>/<command>/       # nested tool (e.g. jira/fetchticket)
  <domain>/                      # protocol/SDK wrappers and shared helpers
                                 # (e.g. internal/atlassian)
```

Rules:

- Each tool lives in its own package under `internal/tools/`. Nested tools
  are grouped under a namespace directory and named as
  `<namespace>.<command>` (planned: `jira.*`, `confluence.*`).
- External-SDK wrappers and cross-cutting helpers live under
  `internal/<domain>/` (e.g. `internal/atlassian/`), never inside a tool
  package.
- Tool packages depend on the domain packages, not the other way around.

## 5. Configuration sourcing

Tools that need credentials follow an env-first lookup:

1. Read from `os.Getenv`.
2. If unset, fall back to a per-user dotfile at
   `$XDG_CONFIG_HOME/atlassian-mcp/config` (or
   `~/.config/atlassian-mcp/config` when `XDG_CONFIG_HOME` is unset), parsed
   as `KEY=VALUE` lines. Blank lines and `#` comments are ignored; no
   quoting or interpolation is performed, and a missing file is not an
   error. The fallback is per-key, so env can set some vars while the
   dotfile supplies the rest.
3. If still unset, return a typed error (`*atlassian.ErrMissingEnv`) from
   the tool's `Invoke` so the frontend can surface it.

Clients backed by external SDKs MUST be constructed lazily on first
invocation — no tool may fail registry boot due to missing credentials.
`atlassian-mcp ping` and `atlassian-mcp mcp` keep working in environments
where Atlassian credentials are not configured.

## 6. Testing conventions

- Any external SDK is wrapped behind a small interface inside the domain
  package; tests substitute a fake implementation. No live external calls
  in tests.
- When several tool packages share the same domain seam, the in-memory fake
  is promoted to a sibling `<domain>test` package (e.g.
  `internal/atlassian/atlassiantest`) so each tool can import it instead of
  re-implementing the interface in every `_test.go`.
- Pattern reference: `internal/tools/ping/ping_test.go` for the simplest tool
  shape, and `internal/frontends/{cli,mcp}` tests for frontend behaviour.
- Each tool ships its own `_test.go` that covers happy-path invocation, input
  schema declarations, and the human/JSON output shapes its frontend emits.

## 7. Change log

- **0.0.0** — Initial Go MCP + CLI structure with the `ping` tool.
- **0.1.0** — Added `confluence.publish-obsidian-file` together with the
  supporting `internal/atlassian` (Confluence v2 REST client + lazy env
  config), `internal/markdown` (markdown→ADF wrapper + property table
  helper), and `internal/obsidian` (frontmatter parse + in-place key
  update) domain packages.
- **0.1.1** — Implemented the per-user dotfile fallback for credential
  lookup at `$XDG_CONFIG_HOME/atlassian-mcp/config`
  (default `~/.config/atlassian-mcp/config`). Env still wins per key.
- **0.1.2** — Relaxed `confluence.publish-obsidian-file` frontmatter
  requirements: `confluence_space` and `confluence_title` are required
  only when `confluence_page_id` is absent. Once a page ID is bound, the
  tool updates by ID and preserves the live Confluence title, ignoring
  any local `confluence_title` value.
- **0.2.0** — Added `confluence.download-page`: fetches a Confluence page
  by numeric ID or full page URL, converts the ADF body to markdown via
  the new ADF→markdown renderer in `internal/markdown` (parity with the
  publish path plus panels, expand, taskList, mention, emoji, status),
  and writes an Obsidian-friendly markdown file. The property table that
  the publish tool emits is detected and lifted back into YAML
  frontmatter so a download → publish round-trip preserves
  publish-controlled metadata. The `atlassian.Page` struct now binds the
  `body.atlas_doc_format` field returned by `GetPage`.
