package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/miere/atlassian-markdown-mcp/internal/tools"
	"github.com/miere/atlassian-markdown-mcp/internal/tools/ping"
)

// fakeTool is a structured-output tool used to verify JSON wrapping and
// schema exposure end-to-end through the MCP frontend.
type fakeTool struct {
	name   string
	schema *jsonschema.Schema
	result any
}

func (f *fakeTool) Name() string                    { return f.name }
func (f *fakeTool) Description() string             { return "fake tool" }
func (f *fakeTool) InputSchema() *jsonschema.Schema { return f.schema }
func (f *fakeTool) Invoke(_ context.Context, _ map[string]any) (any, error) {
	return f.result, nil
}

// newConnectedClient wires an MCP server built from f to an MCP client over
// an in-memory transport and returns the connected client session.
func newConnectedClient(t *testing.T, f *Frontend) *mcpsdk.ClientSession {
	t.Helper()
	ctx := context.Background()

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	server := f.Server()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "v0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func newPingFrontend() *Frontend {
	reg := tools.NewRegistry()
	reg.Register(ping.New())
	return New(reg)
}

func TestServer_ListsExactlyPing(t *testing.T) {
	session := newConnectedClient(t, newPingFrontend())

	res, err := session.ListTools(context.Background(), &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("ListTools returned %d tools, want 1", len(res.Tools))
	}
	if got := res.Tools[0].Name; got != "ping" {
		t.Fatalf("tool name = %q, want %q", got, "ping")
	}
}

func TestServer_CallPing_ReturnsPong(t *testing.T) {
	session := newConnectedClient(t, newPingFrontend())

	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "ping"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool returned error result: %+v", res)
	}
	if len(res.Content) != 1 {
		t.Fatalf("CallTool returned %d content blocks, want 1", len(res.Content))
	}
	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("CallTool returned %T, want *mcp.TextContent", res.Content[0])
	}
	if text.Text != "pong" {
		t.Fatalf("CallTool text = %q, want %q", text.Text, "pong")
	}
}

func TestServer_StructResult_JSONMarshalledIntoTextContent(t *testing.T) {
	type out struct {
		OK     bool   `json:"ok"`
		Ticket string `json:"ticket"`
	}
	tool := &fakeTool{
		name: "jira.fetch-ticket",
		schema: &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{"ticket": {Type: "string"}},
			Required:   []string{"ticket"},
		},
		result: out{OK: true, Ticket: "JIRA-1"},
	}
	reg := tools.NewRegistry()
	reg.Register(tool)
	session := newConnectedClient(t, New(reg))

	// Wire name uses underscores even though the registry name uses dashes.
	res, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "jira.fetch_ticket",
		Arguments: map[string]any{"ticket": "JIRA-1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool returned error result: %+v", res)
	}
	text := res.Content[0].(*mcpsdk.TextContent).Text
	var got out
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("result text is not valid JSON: %v; text=%q", err, text)
	}
	if got != (out{OK: true, Ticket: "JIRA-1"}) {
		t.Fatalf("decoded result = %+v, want {OK:true Ticket:JIRA-1}", got)
	}
}

func TestServer_ToolWithSchema_ListsSchema(t *testing.T) {
	tool := &fakeTool{
		name: "jira.fetch-ticket",
		schema: &jsonschema.Schema{
			Type: "object",
			Properties: map[string]*jsonschema.Schema{
				"ticket": {Type: "string"},
			},
			Required: []string{"ticket"},
		},
		result: "ok",
	}
	reg := tools.NewRegistry()
	reg.Register(tool)
	session := newConnectedClient(t, New(reg))

	res, err := session.ListTools(context.Background(), &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 1 || res.Tools[0].Name != "jira.fetch_ticket" {
		t.Fatalf("ListTools returned %+v, want one jira.fetch_ticket tool", res.Tools)
	}
	raw, err := json.Marshal(res.Tools[0].InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	var got struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal input schema: %v; raw=%s", err, string(raw))
	}
	if got.Type != "object" {
		t.Fatalf("schema type = %q, want %q", got.Type, "object")
	}
	if _, ok := got.Properties["ticket"]; !ok {
		t.Fatalf("schema properties missing 'ticket'; got %v", got.Properties)
	}
	if len(got.Required) != 1 || got.Required[0] != "ticket" {
		t.Fatalf("schema required = %v, want [ticket]", got.Required)
	}
}
