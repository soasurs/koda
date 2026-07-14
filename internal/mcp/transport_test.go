package mcp

import (
	"net/http"
	"os"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/soasurs/koda/internal/config"
)

func TestNewHTTPTransport(t *testing.T) {
	t.Setenv("MCP_TOKEN", "secret")
	server, transport, err := newTransport(config.MCPServerConfig{
		ID: "search", Transport: "http", URL: "https://example.com/mcp",
		Headers: map[string]string{"Authorization": "Bearer ${MCP_TOKEN}"},
	})
	if err != nil {
		t.Fatalf("newTransport() error = %v", err)
	}
	value, ok := transport.(*sdkmcp.StreamableClientTransport)
	if !ok || value.Endpoint != server.Target || value.HTTPClient == nil {
		t.Fatalf("newTransport() = %+v, %T", server, transport)
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, value.Endpoint, nil)
	base := value.HTTPClient.Transport.(headerTransport)
	base.base = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})
	if _, err := base.RoundTrip(request); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
}

func TestNewStdioTransport(t *testing.T) {
	workdir := t.TempDir()
	t.Setenv("MCP_VALUE", "resolved")
	server, transport, err := newTransport(config.MCPServerConfig{
		ID: "local", Name: "Local", Transport: "stdio", Command: "node",
		Args: []string{"server.js", "${MCP_VALUE}"}, Env: map[string]string{"TOKEN": "${MCP_VALUE}"}, Workdir: workdir,
	})
	if err != nil {
		t.Fatalf("newTransport() error = %v", err)
	}
	value, ok := transport.(*sdkmcp.CommandTransport)
	if !ok || server.Target != "node" || value.Command.Dir != workdir || len(value.Command.Args) != 3 || value.Command.Args[2] != "resolved" ||
		!contains(value.Command.Env, "TOKEN=resolved") {
		t.Fatalf("newTransport() = %+v, %#v", server, transport)
	}
}

func TestNewTransportRejectsUnsafeConfiguration(t *testing.T) {
	tests := []config.MCPServerConfig{
		{ID: "", Transport: "http", URL: "https://example.com/mcp"},
		{ID: "remote", Transport: "http", URL: "http://example.com/mcp"},
		{ID: "credential", Transport: "http", URL: "https://user:pass@example.com/mcp"},
		{ID: "query", Transport: "http", URL: "https://example.com/mcp?token=secret"},
		{ID: "missing", Transport: "stdio"},
		{ID: "mixed", Transport: "stdio", Command: "server", URL: "https://example.com/mcp"},
		{ID: "env", Transport: "stdio", Command: "server", Args: []string{"${MISSING_MCP_TEST_VALUE}"}},
	}
	os.Unsetenv("MISSING_MCP_TEST_VALUE")
	for _, test := range tests {
		if _, _, err := newTransport(test); err == nil {
			t.Fatalf("newTransport(%+v) error = nil", test)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
