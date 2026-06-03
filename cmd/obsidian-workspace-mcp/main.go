// Command obsidian-workspace-mcp is the single entry point for the
// atlassian-markdown-mcp tool. It can run either as a CLI (e.g.
// `obsidian-workspace-mcp ping`) or as an MCP stdio server (`obsidian-workspace-mcp mcp`).
// Both modes are backed by the same underlying tool registry.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/miere/atlassian-markdown-mcp/internal/app"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "obsidian-workspace-mcp:", err)
		os.Exit(1)
	}
}

// run parses the top-level command and delegates to the application layer.
// It is separated from main() so it can be exercised by tests.
func run(args []string) error {
	mode := app.ModeCLI
	rest := args
	if len(args) > 0 && args[0] == "mcp" {
		mode = app.ModeMCP
		rest = args[1:]
	}

	a := app.New(mode, rest)
	if mode == app.ModeCLI && len(args) == 0 {
		return fmt.Errorf("%s", a.UsageLine())
	}
	return a.Run(context.Background())
}
