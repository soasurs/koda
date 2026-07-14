package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	adkskill "github.com/soasurs/adk/skill"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	kodaconfig "github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/provider"
	kodaserver "github.com/soasurs/koda/internal/server"
	"github.com/soasurs/koda/internal/store"
)

func TestParseServeConfig(t *testing.T) {
	config, err := parseServeConfig(nil, &bytes.Buffer{})
	if err != nil || config.explicitly || config.address != "" {
		t.Fatalf("parseServeConfig() = %+v, %v", config, err)
	}
	config, err = parseServeConfig([]string{"--addr", "127.0.0.1:8787"}, &bytes.Buffer{})
	if err != nil || !config.explicitly || config.address != "127.0.0.1:8787" {
		t.Fatalf("parseServeConfig(--addr) = %+v, %v", config, err)
	}
}

func TestParseServeConfigRejectsUnsafeOrSingleDashOptions(t *testing.T) {
	for _, args := range [][]string{
		{"-addr", "127.0.0.1:8787"},
		{"--addr", "0.0.0.0:8787"},
		{"--addr", ""},
	} {
		if _, err := parseServeConfig(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("parseServeConfig(%q) error = nil", args)
		}
	}
}

func TestApplyFileConfig(t *testing.T) {
	file := kodaconfig.Config{Version: 1, Server: kodaconfig.ServerConfig{Address: "127.0.0.1:8787"}}
	got, err := applyFileConfig(serveConfig{}, file)
	if err != nil || !got.explicitly || got.address != file.Server.Address {
		t.Fatalf("applyFileConfig() = %+v, %v", got, err)
	}
	cli := serveConfig{address: "127.0.0.1:9999", explicitly: true}
	got, err = applyFileConfig(cli, file)
	if err != nil || got != cli {
		t.Fatalf("applyFileConfig(CLI override) = %+v, %v", got, err)
	}
	if _, err := applyFileConfig(serveConfig{}, kodaconfig.Config{Server: kodaconfig.ServerConfig{Address: "0.0.0.0:8787"}}); err == nil {
		t.Fatal("applyFileConfig(non-loopback) error = nil")
	}
}

func TestListenForServeFallsBackWhenDefaultPortIsOccupied(t *testing.T) {
	var addresses []string
	listener, fallback, err := listenForServe(serveConfig{}, func(_ string, address string) (net.Listener, error) {
		addresses = append(addresses, address)
		if address == kodaserver.DefaultAddress {
			return nil, &net.OpError{Op: "listen", Net: "tcp", Err: syscall.EADDRINUSE}
		}
		return net.Listen("tcp", address)
	})
	if err != nil {
		t.Fatalf("listenForServe() error = %v", err)
	}
	if !fallback {
		t.Fatal("listenForServe() fallback = false")
	}
	defer listener.Close()
	if len(addresses) != 2 || addresses[1] != "localhost:0" {
		t.Fatalf("listener = %q, addresses = %v", listener.Addr(), addresses)
	}
}

func TestRunStartsAndStopsLocalAPIServer(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	directory := t.TempDir()
	loadSkillsCalls := 0
	dependencies := dependencies{
		openRegistry: func() (*provider.Registry, error) {
			return provider.Open(filepath.Join(directory, "providers.json"))
		},
		openStore: func(ctx context.Context) (*store.Store, error) {
			return store.Open(ctx, filepath.Join(directory, "koda.db"))
		},
		loadSkills: func(*slog.Logger) (*adkskill.Catalog, error) {
			loadSkillsCalls++
			return adkskill.NewCatalog()
		},
		listen: net.Listen,
	}
	stdout := lineWriter{lines: make(chan string, 1)}
	done := make(chan error, 1)
	go func() {
		done <- runWithDependencies(ctx, []string{"serve", "--addr", "127.0.0.1:0"}, stdout, &bytes.Buffer{}, dependencies)
	}()
	select {
	case line := <-stdout.lines:
		if !strings.HasPrefix(line, "koda API listening on http://127.0.0.1:") {
			t.Fatalf("startup output = %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("runWithDependencies() did not start the local API server")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWithDependencies() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runWithDependencies() did not stop after cancellation")
	}
	if loadSkillsCalls != 1 {
		t.Fatalf("loadSkills calls = %d, want 1", loadSkillsCalls)
	}
}

func TestRunLogsSkillLoadFailureAndStarts(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	directory := t.TempDir()
	dependencies := dependencies{
		openRegistry: func() (*provider.Registry, error) {
			return provider.Open(filepath.Join(directory, "providers.json"))
		},
		openStore: func(ctx context.Context) (*store.Store, error) {
			return store.Open(ctx, filepath.Join(directory, "koda.db"))
		},
		loadSkills: func(*slog.Logger) (*adkskill.Catalog, error) {
			return nil, errors.New("skills unavailable")
		},
		listen: net.Listen,
	}
	stdout := lineWriter{lines: make(chan string, 1)}
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runWithDependencies(ctx, []string{"serve", "--addr", "127.0.0.1:0"}, stdout, &stderr, dependencies)
	}()
	select {
	case <-stdout.lines:
	case <-time.After(time.Second):
		t.Fatal("runWithDependencies() did not start after skill load failure")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWithDependencies() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runWithDependencies() did not stop after cancellation")
	}
	if output := stderr.String(); !strings.Contains(output, "level=ERROR msg=\"load skills failed\"") || !strings.Contains(output, "skills unavailable") {
		t.Fatalf("stderr = %q", output)
	}
}

func TestRunStartsAndStopsStudio(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	directory := t.TempDir()
	opened := make(chan string, 1)
	dependencies := dependencies{
		openRegistry: func() (*provider.Registry, error) {
			return provider.Open(filepath.Join(directory, "providers.json"))
		},
		openStore: func(ctx context.Context) (*store.Store, error) {
			return store.Open(ctx, filepath.Join(directory, "koda.db"))
		},
		listen: net.Listen,
		openBrowser: func(url string) error {
			opened <- url
			return nil
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- runWithDependencies(ctx, []string{"studio", "--addr", "127.0.0.1:0"}, &bytes.Buffer{}, &bytes.Buffer{}, dependencies)
	}()
	var baseURL string
	select {
	case baseURL = <-opened:
	case <-time.After(time.Second):
		t.Fatal("runWithDependencies() did not open Koda Studio")
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Get(baseURL + "/sessions/session-1") //nolint:noctx // The client timeout bounds the local test request.
	if err != nil {
		t.Fatalf("GET Studio route error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET Studio route status = %d", response.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWithDependencies() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runWithDependencies() did not stop after cancellation")
	}
}

func TestIntegrationServeRestartPreservesSessions(t *testing.T) {
	directory := t.TempDir()
	dependencies := dependencies{
		openRegistry: func() (*provider.Registry, error) { return provider.Open(filepath.Join(directory, "providers.json")) },
		openStore: func(ctx context.Context) (*store.Store, error) {
			return store.Open(ctx, filepath.Join(directory, "koda.db"))
		},
		listen: net.Listen,
	}
	start := func() (kodav1connect.KodaServiceClient, context.CancelFunc, <-chan error) {
		ctx, cancel := context.WithCancel(t.Context())
		output := lineWriter{lines: make(chan string, 1)}
		done := make(chan error, 1)
		go func() {
			done <- runWithDependencies(ctx, []string{"serve", "--addr", "127.0.0.1:0"}, output, &bytes.Buffer{}, dependencies)
		}()
		select {
		case line := <-output.lines:
			baseURL := strings.TrimSpace(strings.TrimPrefix(line, "koda API listening on "))
			return kodav1connect.NewKodaServiceClient(http.DefaultClient, baseURL), cancel, done
		case <-time.After(time.Second):
			t.Fatal("serve did not start")
		}
		return nil, cancel, done
	}

	client, cancel, done := start()
	created, err := client.CreateSession(t.Context(), v1.CreateSessionRequest_builder{
		Workdir: new(t.TempDir()), ProviderId: new("openai-responses"), ModelId: new("gpt-5.6"),
	}.Build())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("first serve error = %v", err)
	}

	client, cancel, done = start()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("second serve error = %v", err)
		}
	}()
	got, err := client.GetSession(t.Context(), v1.GetSessionRequest_builder{SessionId: new(created.GetSession().GetId())}.Build())
	if err != nil || got.GetSession().GetId() != created.GetSession().GetId() {
		t.Fatalf("GetSession(after restart) = %+v, %v", got, err)
	}
}

func TestRunDisplaysRootHelp(t *testing.T) {
	stdout := new(bytes.Buffer)
	err := run(t.Context(), nil, stdout, &bytes.Buffer{})
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stdout.String(), "serve   start") || !strings.Contains(stdout.String(), "studio  start") {
		t.Fatalf("run() = %v, output = %q", err, stdout.String())
	}
}

func TestRunHelpForCommandsAndUnknownCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	if err := run(t.Context(), []string{"help", "serve"}, stdout, &bytes.Buffer{}); err != nil || !strings.Contains(stdout.String(), "--addr") {
		t.Fatalf("run(help serve) = %v, output = %q", err, stdout.String())
	}
	stdout.Reset()
	if err := run(t.Context(), []string{"help", "studio"}, stdout, &bytes.Buffer{}); err != nil || !strings.Contains(stdout.String(), "open it in a browser") {
		t.Fatalf("run(help studio) = %v, output = %q", err, stdout.String())
	}
	if err := runWithDependencies(t.Context(), []string{"missing"}, &bytes.Buffer{}, &bytes.Buffer{}, dependencies{}); err == nil {
		t.Fatal("runWithDependencies(unknown command) error = nil")
	}
}

func TestListenForServeDoesNotFallbackForExplicitAddress(t *testing.T) {
	var attempts int
	_, _, err := listenForServe(serveConfig{address: "127.0.0.1:8787", explicitly: true}, func(string, string) (net.Listener, error) {
		attempts++
		return nil, syscall.EADDRINUSE
	})
	if !errors.Is(err, syscall.EADDRINUSE) || attempts != 1 {
		t.Fatalf("listenForServe(explicit) = %v after %d attempts", err, attempts)
	}
}

type lineWriter struct {
	lines chan string
}

func (w lineWriter) Write(value []byte) (int, error) {
	w.lines <- string(value)
	return len(value), nil
}
