package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
)

const (
	// DefaultAddress is the loopback-only HTTP listen address used when no
	// explicit address is configured.
	DefaultAddress = "localhost:8080"

	defaultShutdownTimeout = 10 * time.Second
)

// HTTPServerConfig configures the local Connect HTTP server.
type HTTPServerConfig struct {
	// Address is the listen address. An empty value uses DefaultAddress.
	Address string
	// ShutdownTimeout bounds graceful shutdown. A non-positive value uses the
	// default timeout.
	ShutdownTimeout time.Duration
}

// HTTPServer serves Koda's Connect API and coordinates graceful shutdown.
type HTTPServer struct {
	server          *http.Server
	shutdownTimeout time.Duration
}

// NewHTTPServer constructs an HTTPServer for handler. It does not open a
// listener; callers may choose their own listener before calling Serve.
func NewHTTPServer(handler *Handler, config HTTPServerConfig) (*HTTPServer, error) {
	if handler == nil {
		return nil, errors.New("server: handler must not be nil")
	}
	address := strings.TrimSpace(config.Address)
	if address == "" {
		address = DefaultAddress
	}
	shutdownTimeout := config.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultShutdownTimeout
	}
	path, connectHandler := kodav1connect.NewKodaServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(path, connectHandler)
	return &HTTPServer{
		server: &http.Server{
			Addr:              address,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
		},
		shutdownTimeout: shutdownTimeout,
	}, nil
}

// Address returns the configured listen address.
func (s *HTTPServer) Address() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.Addr
}

// Serve accepts requests from listener until the listener fails or ctx is
// canceled. Cancellation gracefully drains active requests before returning
// ctx.Err().
func (s *HTTPServer) Serve(ctx context.Context, listener net.Listener) error {
	if s == nil || s.server == nil {
		return errors.New("server: HTTP server must not be nil")
	}
	if ctx == nil {
		return errors.New("server: context must not be nil")
	}
	if listener == nil {
		return errors.New("server: listener must not be nil")
	}
	// Request contexts inherit ctx so shutdown also releases Runs blocked on an
	// approval or question broker before HTTP shutdown waits for them.
	s.server.BaseContext = func(net.Listener) context.Context {
		return ctx
	}
	errs := make(chan error, 1)
	go func() {
		errs <- s.server.Serve(listener)
	}()

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server: graceful shutdown: %w", err)
		}
		if err := <-errs; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server: serve after shutdown: %w", err)
		}
		return ctx.Err()
	}
}
