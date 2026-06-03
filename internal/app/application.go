// Package app is the composition root. It owns the tool registry, picks the
// frontend based on the parsed mode, and starts it. Frontends know nothing
// about each other; the application knows about both but delegates execution
// entirely.
package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/miere/atlassian-markdown-mcp/internal/frontends/cli"
	"github.com/miere/atlassian-markdown-mcp/internal/frontends/mcp"
	"github.com/miere/atlassian-markdown-mcp/internal/tools"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/confluence/downloadpage"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/confluence/publishobsidianfile"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/jira/getticket"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/jira/updateticket"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/ping"
)

// Mode selects which frontend Run starts.
type Mode int

const (
	// ModeCLI runs the human-facing CLI frontend.
	ModeCLI Mode = iota
	// ModeMCP runs the MCP stdio server frontend.
	ModeMCP
)

// Application is the composition root for a single atlassian-mcp invocation.
type Application struct {
	mode     Mode
	args     []string
	registry *tools.Registry
}

// New constructs an Application configured for the given mode. args is the
// list of positional arguments passed to the selected frontend (the
// top-level mode token is stripped by the caller before this point).
func New(mode Mode, args []string) *Application {
	reg := tools.NewRegistry()
	reg.Register(ping.New())
	reg.Register(publishobsidianfile.New())
	reg.Register(downloadpage.New())
	reg.Register(getticket.New())
	reg.Register(updateticket.New())
	return &Application{mode: mode, args: args, registry: reg}
}

// Run starts the selected frontend and blocks until it returns.
func (a *Application) Run(ctx context.Context) error {
	switch a.mode {
	case ModeMCP:
		return mcp.New(a.registry).Serve(ctx)
	default:
		return cli.New(a.registry).Run(ctx, a.args)
	}
}

// UsageLine renders a human-readable usage string built from the registered
// tools. Flat tool names (e.g. `ping`) are listed first; namespaced tools
// (e.g. `jira.fetch-ticket`) are grouped by their namespace and rendered as
// `<ns> <sub>`. The built-in `mcp` mode (handled by main.go) is appended
// last so callers see every entry point in one line.
func (a *Application) UsageLine() string {
	var flat []string
	groups := map[string][]string{}
	var groupOrder []string

	for _, t := range a.registry.All() {
		name := t.Name()
		if i := strings.Index(name, "."); i >= 0 {
			ns, sub := name[:i], name[i+1:]
			if _, seen := groups[ns]; !seen {
				groupOrder = append(groupOrder, ns)
			}
			groups[ns] = append(groups[ns], sub)
			continue
		}
		flat = append(flat, name)
	}

	parts := append([]string{}, flat...)
	for _, ns := range groupOrder {
		subs := groups[ns]
		sort.Strings(subs)
		parts = append(parts, fmt.Sprintf("%s <%s>", ns, strings.Join(subs, "|")))
	}
	parts = append(parts, "mcp")
	return "usage: atlassian-mcp <command>; commands: " + strings.Join(parts, ", ")
}
