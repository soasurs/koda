package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"net"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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

func TestListenForServeFallsBackWhenDefaultPortIsOccupied(t *testing.T) {
	var addresses []string
	listener, err := listenForServe(serveConfig{}, func(_ string, address string) (net.Listener, error) {
		addresses = append(addresses, address)
		if address == kodaserver.DefaultAddress {
			return nil, &net.OpError{Op: "listen", Net: "tcp", Err: syscall.EADDRINUSE}
		}
		return net.Listen("tcp", address)
	})
	if err != nil {
		t.Fatalf("listenForServe() error = %v", err)
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
	dependencies := dependencies{
		openRegistry: func() (*provider.Registry, error) {
			return provider.Open(filepath.Join(directory, "providers.json"))
		},
		openStore: func(ctx context.Context) (*store.Store, error) {
			return store.Open(ctx, filepath.Join(directory, "koda.db"))
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
}

func TestRunDisplaysRootHelp(t *testing.T) {
	stdout := new(bytes.Buffer)
	err := run(t.Context(), nil, stdout, &bytes.Buffer{})
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(stdout.String(), "serve   start") {
		t.Fatalf("run() = %v, output = %q", err, stdout.String())
	}
}

func TestRunHelpForServeAndUnknownCommand(t *testing.T) {
	stdout := new(bytes.Buffer)
	if err := run(t.Context(), []string{"help", "serve"}, stdout, &bytes.Buffer{}); err != nil || !strings.Contains(stdout.String(), "--addr") {
		t.Fatalf("run(help serve) = %v, output = %q", err, stdout.String())
	}
	if err := runWithDependencies(t.Context(), []string{"studio"}, &bytes.Buffer{}, &bytes.Buffer{}, dependencies{}); err == nil {
		t.Fatal("runWithDependencies(unknown command) error = nil")
	}
}

func TestListenForServeDoesNotFallbackForExplicitAddress(t *testing.T) {
	var attempts int
	_, err := listenForServe(serveConfig{address: "127.0.0.1:8787", explicitly: true}, func(string, string) (net.Listener, error) {
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
