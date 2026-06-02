package app

import (
	"strings"
	"testing"
)

// TestRegistry_ContainsAllExpectedTools is the composition-root smoke test:
// it asserts that New wires up every tool the spec expects, with the right
// names and required-field schemas. Drift here means the binary ships with
// missing or mis-named tools.
func TestRegistry_ContainsAllExpectedTools(t *testing.T) {
	app := New(ModeCLI, nil)

	cases := []struct {
		name     string
		required []string
	}{
		{"ping", nil},
		{"confluence.publish-obsidian-file", []string{"file_path"}},
		{"confluence.download-page", []string{"page"}},
	}

	for _, c := range cases {
		tool, ok := app.registry.Get(c.name)
		if !ok {
			t.Errorf("registry missing %q", c.name)
			continue
		}
		schema := tool.InputSchema()
		if c.required == nil {
			if schema != nil {
				t.Errorf("%s: InputSchema = %+v, want nil", c.name, schema)
			}
			continue
		}
		if schema == nil || schema.Type != "object" {
			t.Errorf("%s: InputSchema type = %+v, want object", c.name, schema)
			continue
		}
		got := map[string]bool{}
		for _, r := range schema.Required {
			got[r] = true
		}
		for _, want := range c.required {
			if !got[want] {
				t.Errorf("%s: required missing %q (have %v)", c.name, want, schema.Required)
			}
		}
	}
}

// TestUsageLine_ListsFlatToolsNamespacesAndMCP guards the bare-invocation
// help line: regressing it (e.g. hard-coding "ping, mcp") would mask missing
// tools in the shipped binary as new namespaces are added.
func TestUsageLine_ListsFlatToolsNamespacesAndMCP(t *testing.T) {
	line := New(ModeCLI, nil).UsageLine()

	for _, want := range []string{
		"ping",
		"confluence <download-page|publish-obsidian-file>",
		"mcp",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("UsageLine missing %q in:\n%s", want, line)
		}
	}
	if !strings.HasPrefix(line, "usage: atlassian-mcp <command>; commands: ") {
		t.Errorf("UsageLine prefix wrong: %q", line)
	}
}
