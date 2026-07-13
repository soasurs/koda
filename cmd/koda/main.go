// Command koda manages the local Koda service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	adkskill "github.com/soasurs/adk/skill"

	kodaconfig "github.com/soasurs/koda/internal/config"
	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/provider"
	kodaserver "github.com/soasurs/koda/internal/server"
	kodaskills "github.com/soasurs/koda/internal/skills"
	"github.com/soasurs/koda/internal/store"
	kodastudio "github.com/soasurs/koda/internal/studio"
)

const shutdownTimeout = 10 * time.Second

type serveConfig struct {
	address    string
	explicitly bool
}

type dependencies struct {
	loadConfig   func() (kodaconfig.Config, error)
	openRegistry func() (*provider.Registry, error)
	openStore    func(context.Context) (*store.Store, error)
	loadSkills   func(*slog.Logger) (*adkskill.Catalog, error)
	listen       func(network, address string) (net.Listener, error)
	openBrowser  func(string) error
}

var productionDependencies = dependencies{
	loadConfig:   kodaconfig.LoadDefault,
	openRegistry: provider.OpenDefault,
	openStore:    store.OpenDefault,
	loadSkills:   kodaskills.LoadDefault,
	listen:       net.Listen,
	openBrowser:  openBrowser,
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
	case "studio":
		return runStudio(ctx, args[1:], stdout, stderr, dependencies)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runHelp(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		printRootUsage(stdout)
		return nil
	}
	if len(args) == 1 {
		switch args[0] {
		case "serve":
			printServeUsage(stdout)
			return nil
		case "studio":
			printStudioUsage(stdout)
			return nil
		}
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func runServe(ctx context.Context, args []string, stdout, stderr io.Writer, dependencies dependencies) error {
	config, err := parseServerConfig("serve", args, stderr, printServeUsage)
	if err != nil {
		return err
	}
	return runServer(ctx, config, stdout, stderr, nil, false, dependencies)
}

func runStudio(ctx context.Context, args []string, stdout, stderr io.Writer, dependencies dependencies) error {
	config, err := parseServerConfig("studio", args, stderr, printStudioUsage)
	if err != nil {
		return err
	}
	webHandler, err := kodastudio.NewHandler()
	if err != nil {
		return err
	}
	return runServer(ctx, config, stdout, stderr, webHandler, true, dependencies)
}

func runServer(ctx context.Context, config serveConfig, stdout, stderr io.Writer, webHandler http.Handler, launchBrowser bool, dependencies dependencies) error {
	if dependencies.openRegistry == nil || dependencies.openStore == nil || dependencies.listen == nil {
		return errors.New("command dependencies must not be nil")
	}
	fileConfig := kodaconfig.Config{}
	if dependencies.loadConfig != nil {
		var err error
		fileConfig, err = dependencies.loadConfig()
		if err != nil {
			return fmt.Errorf("load configuration: %w", err)
		}
		config, err = applyFileConfig(config, fileConfig)
		if err != nil {
			return err
		}
	}
	logger, err := logging.New(stderr, fileConfig.Log.Level)
	if err != nil {
		return err
	}
	registry, err := dependencies.openRegistry()
	if err != nil {
		return fmt.Errorf("open provider registry: %w", err)
	}
	catalog, err := provider.NewCatalog(registry, nil)
	if err != nil {
		return fmt.Errorf("create model catalog: %w", err)
	}
	var skillCatalog *adkskill.Catalog
	if dependencies.loadSkills != nil {
		skillCatalog, err = dependencies.loadSkills(logger)
		if err != nil {
			logger.ErrorContext(ctx, "load skills failed", "error", err)
			skillCatalog = nil
		}
	}
	sessionStore, err := dependencies.openStore(ctx)
	if err != nil {
		return fmt.Errorf("open session store: %w", err)
	}
	defer func() {
		if err := sessionStore.Close(); err != nil {
			logger.WarnContext(ctx, "session store close failed", "error", err)
		}
	}()
	handler, err := kodaserver.NewHandler(registry, catalog, sessionStore, skillCatalog, logger)
	if err != nil {
		return fmt.Errorf("create service handler: %w", err)
	}
	listener, fallback, err := listenForServe(config, dependencies.listen)
	if err != nil {
		return err
	}
	if fallback {
		logger.InfoContext(ctx, "default address unavailable; using fallback",
			"default_address", kodaserver.DefaultAddress,
			"address", listener.Addr().String(),
		)
	}
	server, err := kodaserver.NewHTTPServer(handler, kodaserver.HTTPServerConfig{
		Address:         listener.Addr().String(),
		ShutdownTimeout: shutdownTimeout,
		WebHandler:      webHandler,
		Logger:          logger,
	})
	if err != nil {
		listener.Close() //nolint:errcheck // Preserve the server construction error.
		return fmt.Errorf("create HTTP server: %w", err)
	}
	url := "http://" + listener.Addr().String()
	serverMode := "serve"
	if launchBrowser {
		serverMode = "studio"
		fmt.Fprintf(stdout, "Koda Studio listening on %s\n", url)
		if dependencies.openBrowser == nil {
			listener.Close() //nolint:errcheck // Preserve the dependency error.
			return errors.New("browser dependency must not be nil")
		}
		if err := dependencies.openBrowser(url); err != nil {
			logger.WarnContext(ctx, "open browser failed", "error", err)
		}
	} else {
		fmt.Fprintf(stdout, "koda API listening on %s\n", url)
	}
	logger.InfoContext(ctx, "server started", "mode", serverMode, "address", listener.Addr().String())
	if err := server.Serve(ctx, listener); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func parseServeConfig(args []string, stderr io.Writer) (serveConfig, error) {
	return parseServerConfig("serve", args, stderr, printServeUsage)
}

func parseServerConfig(command string, args []string, stderr io.Writer, usage func(io.Writer)) (serveConfig, error) {
	if err := rejectSingleDashOptions(args); err != nil {
		return serveConfig{}, err
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { usage(stderr) }
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

func openBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", url)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	if err := command.Run(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

func rejectSingleDashOptions(args []string) error {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			return fmt.Errorf("option %q must use two dashes", arg)
		}
	}
	return nil
}

func applyFileConfig(cli serveConfig, file kodaconfig.Config) (serveConfig, error) {
	if cli.explicitly || file.Server.Address == "" {
		return cli, nil
	}
	if !loopbackAddress(file.Server.Address) {
		return serveConfig{}, errors.New("server.address in koda.yaml must be a loopback address")
	}
	cli.address = file.Server.Address
	cli.explicitly = true
	return cli, nil
}

func listenForServe(config serveConfig, listen func(network, address string) (net.Listener, error)) (net.Listener, bool, error) {
	if config.explicitly {
		listener, err := listen("tcp", config.address)
		if err != nil {
			return nil, false, fmt.Errorf("listen on %q: %w", config.address, err)
		}
		return listener, false, nil
	}
	listener, err := listen("tcp", kodaserver.DefaultAddress)
	if err == nil {
		return listener, false, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, false, fmt.Errorf("listen on %q: %w", kodaserver.DefaultAddress, err)
	}
	listener, fallbackErr := listen("tcp", "localhost:0")
	if fallbackErr != nil {
		return nil, false, fmt.Errorf("listen on an available loopback port: %w", fallbackErr)
	}
	return listener, true, nil
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
	fmt.Fprintln(output, "  studio  start Koda Studio and open it in a browser")
	fmt.Fprintln(output, "  help    show help for a command")
}

func printStudioUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: koda studio [--addr ADDRESS]")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Start Koda Studio on a loopback address and open it in a browser.")
	fmt.Fprintln(output, "--addr overrides server.address in ~/.koda/koda.yaml.")
	fmt.Fprintln(output, "Without either setting, Koda tries localhost:8080 and uses an")
	fmt.Fprintln(output, "available loopback port if it is occupied.")
}

func printServeUsage(output io.Writer) {
	fmt.Fprintln(output, "Usage: koda serve [--addr ADDRESS]")
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Start the loopback-only Connect API server without opening a browser.")
	fmt.Fprintln(output, "--addr overrides server.address in ~/.koda/koda.yaml.")
	fmt.Fprintln(output, "Without either setting, Koda tries localhost:8080 and uses an")
	fmt.Fprintln(output, "available loopback port if it is occupied.")
}
