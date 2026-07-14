package mcp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/soasurs/koda/internal/config"
)

var serverIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type resolvedServer struct {
	ID        string
	Name      string
	Transport Transport
	Target    string
}

func newTransport(value config.MCPServerConfig) (resolvedServer, sdkmcp.Transport, error) {
	value.ID = strings.TrimSpace(value.ID)
	value.Name = strings.TrimSpace(value.Name)
	value.Transport = strings.ToLower(strings.TrimSpace(value.Transport))
	value.URL = strings.TrimSpace(value.URL)
	value.Command = strings.TrimSpace(value.Command)
	value.Workdir = strings.TrimSpace(value.Workdir)
	if !serverIDPattern.MatchString(value.ID) {
		return resolvedServer{}, nil, fmt.Errorf("invalid ID %q", value.ID)
	}
	if value.Name == "" {
		value.Name = value.ID
	}
	switch Transport(value.Transport) {
	case TransportHTTP:
		return newHTTPTransport(value)
	case TransportStdio:
		return newStdioTransport(value)
	default:
		return resolvedServer{}, nil, fmt.Errorf("unsupported transport %q", value.Transport)
	}
}

func newHTTPTransport(value config.MCPServerConfig) (resolvedServer, sdkmcp.Transport, error) {
	if value.URL == "" {
		return resolvedServer{}, nil, errors.New("HTTP URL must not be empty")
	}
	if value.Command != "" || len(value.Args) > 0 || len(value.Env) > 0 || value.Workdir != "" {
		return resolvedServer{}, nil, errors.New("HTTP server must not configure command, args, env, or workdir")
	}
	parsed, err := url.Parse(value.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return resolvedServer{}, nil, fmt.Errorf("invalid HTTP URL %q", value.URL)
	}
	if parsed.User != nil {
		return resolvedServer{}, nil, errors.New("HTTP URL must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return resolvedServer{}, nil, errors.New("HTTP URL must not contain a query or fragment; use headers for credentials")
	}
	if parsed.Scheme == "http" && !loopbackHost(parsed.Hostname()) {
		return resolvedServer{}, nil, errors.New("plaintext HTTP is allowed only for loopback MCP servers")
	}
	headers, err := expandedMap(value.Headers)
	if err != nil {
		return resolvedServer{}, nil, fmt.Errorf("resolve HTTP headers: %w", err)
	}
	for name, headerValue := range headers {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n:") {
			return resolvedServer{}, nil, fmt.Errorf("invalid HTTP header name %q", name)
		}
		if strings.ContainsAny(headerValue, "\r\n") {
			return resolvedServer{}, nil, fmt.Errorf("HTTP header %q contains a newline", name)
		}
	}
	client := &http.Client{}
	if len(headers) > 0 {
		client.Transport = headerTransport{base: http.DefaultTransport, headers: headers}
	}
	server := resolvedServer{ID: value.ID, Name: value.Name, Transport: TransportHTTP, Target: value.URL}
	return server, &sdkmcp.StreamableClientTransport{Endpoint: value.URL, HTTPClient: client}, nil
}

func newStdioTransport(value config.MCPServerConfig) (resolvedServer, sdkmcp.Transport, error) {
	if value.Command == "" {
		return resolvedServer{}, nil, errors.New("stdio command must not be empty")
	}
	if value.URL != "" || len(value.Headers) > 0 {
		return resolvedServer{}, nil, errors.New("stdio server must not configure URL or headers")
	}
	args, err := expandedSlice(value.Args)
	if err != nil {
		return resolvedServer{}, nil, fmt.Errorf("resolve stdio args: %w", err)
	}
	environment, err := expandedMap(value.Env)
	if err != nil {
		return resolvedServer{}, nil, fmt.Errorf("resolve stdio env: %w", err)
	}
	command := exec.Command(value.Command, args...)
	if len(environment) > 0 {
		command.Env = os.Environ()
		for name, envValue := range environment {
			if strings.TrimSpace(name) == "" || strings.ContainsRune(name, '=') {
				return resolvedServer{}, nil, fmt.Errorf("invalid environment variable name %q", name)
			}
			command.Env = append(command.Env, name+"="+envValue)
		}
	}
	if value.Workdir != "" {
		workdir, err := filepath.Abs(value.Workdir)
		if err != nil {
			return resolvedServer{}, nil, fmt.Errorf("resolve stdio workdir: %w", err)
		}
		info, err := os.Stat(workdir)
		if err != nil {
			return resolvedServer{}, nil, fmt.Errorf("stat stdio workdir: %w", err)
		}
		if !info.IsDir() {
			return resolvedServer{}, nil, errors.New("stdio workdir must be a directory")
		}
		command.Dir = workdir
	}
	server := resolvedServer{ID: value.ID, Name: value.Name, Transport: TransportStdio, Target: value.Command}
	return server, &sdkmcp.CommandTransport{Command: command}, nil
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func expandedSlice(values []string) ([]string, error) {
	result := make([]string, len(values))
	for index, value := range values {
		expanded, err := expandEnvironment(value)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		result[index] = expanded
	}
	return result, nil
}

func expandedMap(values map[string]string) (map[string]string, error) {
	if values == nil {
		return nil, nil
	}
	result := make(map[string]string, len(values))
	for name, value := range values {
		expanded, err := expandEnvironment(value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		result[name] = expanded
	}
	return result, nil
}

func expandEnvironment(value string) (string, error) {
	var missing string
	expanded := os.Expand(value, func(name string) string {
		resolved, ok := os.LookupEnv(name)
		if !ok && missing == "" {
			missing = name
		}
		return resolved
	})
	if missing != "" {
		return "", fmt.Errorf("environment variable %q is not set", missing)
	}
	return expanded, nil
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	for name, value := range t.headers {
		clone.Header.Set(name, value)
	}
	return t.base.RoundTrip(clone)
}
