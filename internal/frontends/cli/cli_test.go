package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/miere/atlassian-markdown-mcp/internal/tools"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/ping"
)

// fakeTool is a parameterised tool used to exercise nested routing, flag
// parsing and arg passthrough from a single place.
type fakeTool struct {
	name   string
	schema *jsonschema.Schema
	got    map[string]any
	result any
}

func (f *fakeTool) Name() string                    { return f.name }
func (f *fakeTool) Description() string             { return "fake tool" }
func (f *fakeTool) InputSchema() *jsonschema.Schema { return f.schema }
func (f *fakeTool) Invoke(_ context.Context, args map[string]any) (any, error) {
	f.got = args
	return f.result, nil
}

func newTestFrontend() (*Frontend, *bytes.Buffer, *bytes.Buffer) {
	reg := tools.NewRegistry()
	reg.Register(ping.New())
	var stdout, stderr bytes.Buffer
	f := New(reg).WithOutput(&stdout, &stderr)
	return f, &stdout, &stderr
}

func TestRun_Ping_WritesPongLineAndNoStderr(t *testing.T) {
	f, stdout, stderr := newTestFrontend()
	if err := f.Run(context.Background(), []string{"ping"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := stdout.String(); got != "pong\n" {
		t.Fatalf("stdout = %q, want %q", got, "pong\n")
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRun_NoArgs_ReturnsUsageError(t *testing.T) {
	f, stdout, _ := newTestFrontend()
	err := f.Run(context.Background(), nil)
	if err == nil {
		t.Fatal("Run returned nil, want error")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRun_UnknownCommand_ReturnsError(t *testing.T) {
	f, stdout, _ := newTestFrontend()
	err := f.Run(context.Background(), []string{"nope"})
	if err == nil {
		t.Fatal("Run returned nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, want it to mention 'unknown command'", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRun_NestedNamespace_ResolvesDottedTool(t *testing.T) {
	tool := &fakeTool{
		name: "jira.fetch-ticket",
		schema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{"ticket": {Type: "string"}},
		},
		result: "JIRA-1: Some summary",
	}
	reg := tools.NewRegistry()
	reg.Register(tool)
	var stdout, stderr bytes.Buffer
	f := New(reg).WithOutput(&stdout, &stderr)

	if err := f.Run(context.Background(), []string{"jira", "fetch-ticket", "--ticket", "JIRA-1"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := stdout.String(); got != "JIRA-1: Some summary\n" {
		t.Fatalf("stdout = %q, want %q", got, "JIRA-1: Some summary\n")
	}
	if got, want := tool.got["ticket"], "JIRA-1"; got != want {
		t.Fatalf("tool got ticket=%v, want %v", got, want)
	}
}

func TestRun_PassesParsedArgs_ToTool(t *testing.T) {
	tool := &fakeTool{
		name: "echo",
		schema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"attachment_type": {Type: "string"},
			},
		},
		result: "ok",
	}
	reg := tools.NewRegistry()
	reg.Register(tool)
	var stdout, stderr bytes.Buffer
	f := New(reg).WithOutput(&stdout, &stderr)

	if err := f.Run(context.Background(), []string{"echo", "--attachment-type", "markdown"}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got, want := tool.got["attachment_type"], "markdown"; got != want {
		t.Fatalf("tool got attachment_type=%v, want %v", got, want)
	}
}
