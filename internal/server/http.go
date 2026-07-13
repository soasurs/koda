package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/logging"
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
	// WebHandler optionally serves a same-origin web application for requests
	// outside the Connect API route.
	WebHandler http.Handler
	// Logger receives HTTP lifecycle and security diagnostics.
	Logger *slog.Logger
}

// HTTPServer serves Koda's Connect API and coordinates graceful shutdown.
type HTTPServer struct {
	server          *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
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
	logger := logging.OrDiscard(config.Logger)
	path, connectHandler := kodav1connect.NewKodaServiceHandler(handler,
		connect.WithInterceptors(rpcLoggingInterceptor{logger: logger}),
	)
	mux := http.NewServeMux()
	mux.Handle(path, connectHandler)
	if config.WebHandler != nil {
		mux.Handle("/", config.WebHandler)
	}
	return &HTTPServer{
		server: &http.Server{
			Addr:              address,
			Handler:           localRequestOnly(logger, mux),
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
		},
		shutdownTimeout: shutdownTimeout,
		logger:          logger,
	}, nil
}

type rpcLoggingInterceptor struct {
	logger *slog.Logger
}

func (i rpcLoggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
		startedAt := time.Now()
		response, err := next(ctx, request)
		i.logger.DebugContext(ctx, "rpc completed",
			"procedure", request.Spec().Procedure,
			"code", connect.CodeOf(err).String(),
			"duration", time.Since(startedAt),
		)
		return response, err
	}
}

func (i rpcLoggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i rpcLoggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) error {
		startedAt := time.Now()
		err := next(ctx, connection)
		i.logger.DebugContext(ctx, "rpc completed",
			"procedure", connection.Spec().Procedure,
			"code", connect.CodeOf(err).String(),
			"duration", time.Since(startedAt),
		)
		return err
	}
}

func localRequestOnly(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !isLoopbackHTTPHost(request.Host) || !isLocalOrigin(request.Header.Get("Origin")) {
			logger.WarnContext(request.Context(), "rejected non-local HTTP request",
				"host", request.Host,
				"origin_host", originHost(request.Header.Get("Origin")),
				"remote_address", request.RemoteAddr,
			)
			http.Error(response, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(response, request)
	})
}

func originHost(origin string) string {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "invalid"
	}
	return parsed.Host
}

func isLocalOrigin(origin string) bool {
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return isLoopbackHTTPHost(parsed.Host)
}

func isLoopbackHTTPHost(value string) bool {
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	} else if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
		s.logger.ErrorContext(ctx, "HTTP server stopped unexpectedly", "error", err)
		return err
	case <-ctx.Done():
		startedAt := time.Now()
		s.logger.InfoContext(ctx, "HTTP server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := s.server.Shutdown(shutdownCtx); err != nil {
			s.logger.ErrorContext(ctx, "HTTP server graceful shutdown failed", "error", err)
			return fmt.Errorf("server: graceful shutdown: %w", err)
		}
		if err := <-errs; err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.ErrorContext(ctx, "HTTP server failed after shutdown", "error", err)
			return fmt.Errorf("server: serve after shutdown: %w", err)
		}
		s.logger.InfoContext(ctx, "HTTP server shut down", "duration", time.Since(startedAt))
		return ctx.Err()
	}
}
