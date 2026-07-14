package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/permission"
	kodatools "github.com/soasurs/koda/internal/tools"
)

func TestOpenDiscoversAndNamespacesTools(t *testing.T) {
	connection := &fakeConnection{tools: []tool.Tool{fakeTool{name: "web.search", description: "Search the web"}}}
	manager, err := open(t.Context(), config.MCPConfig{Servers: []config.MCPServerConfig{{
		ID: "exa", Name: "Exa", Transport: "http", URL: "https://mcp.exa.ai/mcp", ReadOnly: true,
	}}}, nil, func(context.Context, sdkmcp.Transport) (toolConnection, error) {
		return connection, nil
	})
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	servers := manager.Servers()
	if len(servers) != 1 || servers[0].ID != "exa" || servers[0].Target != "https://mcp.exa.ai/mcp" || len(servers[0].Tools) != 1 ||
		servers[0].Tools[0].Name != "mcp__exa__web_search" || servers[0].Tools[0].OriginalName != "web.search" {
		t.Fatalf("Servers() = %+v", servers)
	}
	tools := manager.BuildTools(nil)
	if len(tools) != 1 || tools[0].Definition().Name != "mcp__exa__web_search" {
		t.Fatalf("Tools() = %+v", tools)
	}
	if len(manager.PlanTools()) != 1 {
		t.Fatalf("PlanTools() = %+v", manager.PlanTools())
	}
	result, err := tools[0].Run(t.Context(), tool.Call{Name: "mcp__exa__web_search", Arguments: json.RawMessage(`{}`)})
	if err != nil || result.Content != "web.search" {
		t.Fatalf("Run() = %+v, %v", result, err)
	}
	if err := manager.Close(); err != nil || !connection.closed {
		t.Fatalf("Close() = %v, closed = %t", err, connection.closed)
	}
}

func TestOpenConnectsStreamableHTTPServer(t *testing.T) {
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "v1"}, nil)
	server.AddTool(&sdkmcp.Tool{
		Name: "search", Description: "Search", InputSchema: &jsonschema.Schema{Type: "object"},
	}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "result"}}}, nil
	})
	httpServer := httptest.NewServer(sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server { return server }, nil))
	defer httpServer.Close()

	manager, err := Open(t.Context(), config.MCPConfig{Servers: []config.MCPServerConfig{{
		ID: "search", Transport: "http", URL: httpServer.URL, ReadOnly: true,
	}}}, nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer manager.Close() //nolint:errcheck // The explicit call below verifies shutdown.
	values := manager.BuildTools(nil)
	if len(values) != 1 {
		t.Fatalf("Tools() = %+v", values)
	}
	result, err := values[0].Run(t.Context(), tool.Call{Arguments: json.RawMessage(`{}`)})
	if err != nil || result.Content != "result" {
		t.Fatalf("Run() = %+v, %v", result, err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestOpenConnectsStdioServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	manager, err := Open(ctx, config.MCPConfig{Servers: []config.MCPServerConfig{{
		ID: "stdio", Transport: "stdio", Command: os.Args[0],
		Args: []string{"-test.run=^TestMCPStdioHelperProcess$"},
		Env:  map[string]string{"KODA_MCP_HELPER_PROCESS": "1"}, ReadOnly: true,
	}}}, nil)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	values := manager.BuildTools(nil)
	if len(values) != 1 || values[0].Definition().Name != "mcp__stdio__lookup" {
		t.Fatalf("BuildTools() = %+v", values)
	}
	result, err := values[0].Run(ctx, tool.Call{Arguments: json.RawMessage(`{}`)})
	if err != nil || result.Content != "stdio result" {
		t.Fatalf("Run() = %+v, %v", result, err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestMCPStdioHelperProcess(t *testing.T) {
	if os.Getenv("KODA_MCP_HELPER_PROCESS") != "1" {
		return
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "stdio-test", Version: "v1"}, nil)
	server.AddTool(&sdkmcp.Tool{
		Name: "lookup", Description: "Lookup", InputSchema: &jsonschema.Schema{Type: "object"},
	}, func(context.Context, *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		return &sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: "stdio result"}}}, nil
	})
	session, err := server.Connect(context.Background(), &sdkmcp.StdioTransport{}, nil)
	if err != nil {
		os.Exit(2)
	}
	if err := session.Wait(); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestBuildToolsRequiresApprovalForNonReadOnlyServer(t *testing.T) {
	connection := &fakeConnection{tools: []tool.Tool{fakeTool{name: "publish"}}}
	manager, err := open(t.Context(), config.MCPConfig{Servers: []config.MCPServerConfig{{
		ID: "remote", Name: "Remote", Transport: "http", URL: "https://example.com/mcp",
	}}}, nil, func(context.Context, sdkmcp.Transport) (toolConnection, error) {
		return connection, nil
	})
	if err != nil {
		t.Fatalf("open() error = %v", err)
	}
	defer manager.Close() //nolint:errcheck // Test cleanup.
	var approval kodatools.Approval
	values := manager.BuildTools(authorizerFunc(func(_ context.Context, value kodatools.Approval) error {
		approval = value
		return nil
	}))
	result, err := values[0].Run(t.Context(), tool.Call{ID: "call-1", Name: values[0].Definition().Name, Arguments: json.RawMessage(`{"draft":true}`)})
	if err != nil || result.Content != "publish" {
		t.Fatalf("Run() = %+v, %v", result, err)
	}
	if approval.ToolCallID != "call-1" || approval.ToolName != "mcp__remote__publish" || approval.Kind != permission.KindMCP ||
		approval.Scope != permission.ScopeGlobal || string(approval.Arguments) != `{"draft":true}` {
		t.Fatalf("approval = %+v", approval)
	}
	for _, value := range []tool.Tool{
		manager.BuildTools(nil)[0],
		manager.BuildTools(authorizerFunc(func(context.Context, kodatools.Approval) error {
			return kodatools.ErrApprovalRejected
		}))[0],
	} {
		if _, err := value.Run(t.Context(), tool.Call{Name: value.Definition().Name}); err == nil {
			t.Fatal("Run(without approval) error = nil")
		} else {
			var handled *tool.HandledError
			if !errors.As(err, &handled) {
				t.Fatalf("Run(without approval) error = %T, want HandledError", err)
			}
		}
	}
}

func TestOpenRejectsDuplicateServersAndToolAliases(t *testing.T) {
	t.Run("server IDs", func(t *testing.T) {
		config := config.MCPConfig{Servers: []config.MCPServerConfig{
			{ID: "same", Transport: "http", URL: "https://one.example/mcp"},
			{ID: "same", Transport: "http", URL: "https://two.example/mcp"},
		}}
		if _, err := open(t.Context(), config, nil, func(context.Context, sdkmcp.Transport) (toolConnection, error) {
			return &fakeConnection{}, nil
		}); err == nil {
			t.Fatal("open() error = nil")
		}
	})

	t.Run("normalized tool names", func(t *testing.T) {
		connection := &fakeConnection{tools: []tool.Tool{fakeTool{name: "web.search"}, fakeTool{name: "web_search"}}}
		if _, err := open(t.Context(), config.MCPConfig{Servers: []config.MCPServerConfig{{
			ID: "exa", Transport: "http", URL: "https://mcp.exa.ai/mcp",
		}}}, nil, func(context.Context, sdkmcp.Transport) (toolConnection, error) {
			return connection, nil
		}); err == nil {
			t.Fatal("open() error = nil")
		}
	})
}

func TestOpenClosesConnectedServersAfterFailure(t *testing.T) {
	connection := &fakeConnection{}
	calls := 0
	_, err := open(t.Context(), config.MCPConfig{Servers: []config.MCPServerConfig{
		{ID: "first", Transport: "http", URL: "https://first.example/mcp"},
		{ID: "second", Transport: "http", URL: "https://second.example/mcp"},
	}}, nil, func(context.Context, sdkmcp.Transport) (toolConnection, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("unavailable")
		}
		return connection, nil
	})
	if err == nil || !connection.closed {
		t.Fatalf("open() error = %v, first closed = %t", err, connection.closed)
	}
}

func TestNamespacedToolLimitsUTF8Results(t *testing.T) {
	value := strings.Repeat("界", defaultMaxResultBytes)
	wrapped := newNamespacedTool(resultTool{content: value}, "mcp__test__large", "large", "Test")
	result, err := wrapped.Run(t.Context(), tool.Call{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(result.Content) >= len(value) || !utf8.ValidString(result.Content) || !strings.Contains(result.Content, "truncated") {
		t.Fatalf("Run().Content length = %d, valid = %t", len(result.Content), utf8.ValidString(result.Content))
	}
}

type fakeConnection struct {
	tools  []tool.Tool
	closed bool
}

func (c *fakeConnection) Tools(context.Context) ([]tool.Tool, error) {
	return append([]tool.Tool(nil), c.tools...), nil
}

func (c *fakeConnection) Close() error {
	c.closed = true
	return nil
}

type fakeTool struct {
	name        string
	description string
}

type resultTool struct {
	content string
}

type authorizerFunc func(context.Context, kodatools.Approval) error

func (f authorizerFunc) Authorize(ctx context.Context, approval kodatools.Approval) error {
	return f(ctx, approval)
}

func (t resultTool) Definition() tool.Definition {
	return tool.Definition{Name: "large", InputSchema: &jsonschema.Schema{Type: "object"}}
}

func (t resultTool) Run(context.Context, tool.Call) (*tool.Result, error) {
	return &tool.Result{Content: t.content}, nil
}

func (t fakeTool) Definition() tool.Definition {
	return tool.Definition{Name: t.name, Description: t.description, InputSchema: &jsonschema.Schema{Type: "object"}}
}

func (t fakeTool) Run(_ context.Context, call tool.Call) (*tool.Result, error) {
	return &tool.Result{Content: call.Name}, nil
}
