// Command koda manages the local Koda service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/soasurs/koda/internal/provider"
	kodaserver "github.com/soasurs/koda/internal/server"
	"github.com/soasurs/koda/internal/store"
)

const shutdownTimeout = 10 * time.Second

type serveConfig struct {
	address    string
	explicitly bool
}

type dependencies struct {
	openRegistry func() (*provider.Registry, error)
	openStore    func(context.Context) (*store.Store, error)
	listen       func(network, address string) (net.Listener, error)
}

var productionDependencies = dependencies{
	openRegistry: provider.OpenDefault,
	openStore:    store.OpenDefault,
	listen:       net.Listen,
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "koda: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	return runWithDependencies(ctx, args, stdout, stderr, productionDependencies)
}

func runWithDependencies(ctx context.Context, args []string, stdout, stderr io.Writer, dependencies dependencies) error {
	if ctx == nil {
		return errors.New("context must not be nil")
	}
	if len(args) == 0 || args[0] == "--help" {
		printRootUsage(stdout)
		return flag.ErrHelp
	}
	switch args[0] {
	case "help":
		return runHelp(args[1:], stdout)
	case "serve":
		return runServe(ctx, args[1:], stdout, stderr, dependencies)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runHelp(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		printRootUsage(stdout)
		return nil
	}
	if len(args) == 1 && args[0] == "serve" {
		printServeUsage(stdout)
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer, dependencies dependencies) error {
	config, err := parseServeConfig(args, stderr)
	if err != nil {
		return err
	}
	if dependencies.openRegistry == nil || dependencies.openStore == nil || dependencies.listen == nil {
		return errors.New("command dependencies must not be nil")
	}
	registry, err := dependencies.openRegistry()
	if err != nil {
		return fmt.Errorf("open provider registry: %w", err)
	}
	catalog, err := provider.NewCatalog(registry, nil)
	if err != nil {
		return fmt.Errorf("create model catalog: %w", err)
	}
	sessionStore, err := dependencies.openStore(ctx)
	if err != nil {
		return fmt.Errorf("open session store: %w", err)
	}
	defer sessionStore.Close() //nolint:errcheck // Serve errors remain more actionable.
	handler, err := kodaserver.NewHandler(registry, catalog, sessionStore)
	if err != nil {
		return fmt.Errorf("create service handler: %w", err)
	}
	listener, err := listenForServe(config, dependencies.listen)
	if err != nil {
		return err
	}
	server, err := kodaserver.NewHTTPServer(handler, kodaserver.HTTPServerConfig{
		Address:         listener.Addr().String(),
		ShutdownTimeout: shutdownTimeout,
	})
	if err != nil {
		listener.Close() //nolint:errcheck // Preserve the server construction error.
		return fmt.Errorf("create HTTP server: %w", err)
	}
	fmt.Fprintf(stdout, "koda API listening on http://%s\n", listener.Addr())
	if err := server.Serve(ctx, listener); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func parseServeConfig(args []string, stderr io.Writer) (serveConfig, error) {
	if err := rejectSingleDashOptions(args); err != nil {
		return serveConfig{}, err
	}
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { printServeUsage(stderr) }
	value := serveConfig{}
	flags.StringVar(&value.address, "addr", "", "")
	if err := flags.Parse(args); err != nil {
		return serveConfig{}, err
	}
	if flags.NArg() != 0 {
		return serveConfig{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	flags.Visit(func(item *flag.Flag) {
		if item.Name == "addr" {
			value.explicitly = true
		}
	})
	value.address = strings.TrimSpace(value.address)
	if value.explicitly {
		if value.address == "" {
			return serveConfig{}, errors.New("--addr must not be empty")
		}
		if !loopbackAddress(value.address) {
			return serveConfig{}, errors.New("--addr must be a loopback address")
		}
	}
	return value, nil
}

func rejectSingleDashOptions(args []string) error {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			return fmt.Errorf("option %q must use two dashes", arg)
		}
	}
	return nil
}

func listenForServe(config serveConfig, listen func(network, address string) (net.Listener, error)) (net.Listener, error) {
	if config.explicitly {
		listener, err := listen("tcp", config.address)
		if err != nil {
			return nil, fmt.Errorf("listen on %q: %w", config.address, err)
		}
		return listener, nil
	}
	listener, err := listen("tcp", kodaserver.DefaultAddress)
	if err == nil {
		return listener, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("listen on %q: %w", kodaserver.DefaultAddress, err)
	}
	listener, fallbackErr := listen("tcp", "localhost:0")
	if fallbackErr != nil {
		return nil, fmt.Errorf("listen on an available loopback port: %w", fallbackErr)
	}
	return listener, nil
}

func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" {
		return true
	}
	addressIP := net.ParseIP(host)
	return addressIP != nil && addressIP.IsLoopback()
}

func printRootUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: koda <command>")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Commands:")
	fmt.Fprintln(output, "  serve   start the local Connect API server")
	fmt.Fprintln(output, "  help    show help for a command")
}

func printServeUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: koda serve [--addr ADDRESS]")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Start the loopback-only Connect API server without opening a browser.")
	fmt.Fprintln(output, "Without --addr, Koda tries 127.0.0.1:8080 and uses an available")
	fmt.Fprintln(output, "loopback port if it is occupied.")
}
