package server

import (
	"context"
	"log/slog"

	"connectrpc.com/connect"

	"github.com/soasurs/koda/internal/logging"
)

func (h *Handler) log(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	requestID := logging.RequestID(ctx)
	if requestID != "" {
		attrs = append([]slog.Attr{slog.String("request_id", requestID)}, attrs...)
	}
	logging.OrDiscard(h.logger).LogAttrs(ctx, level, message, attrs...)
}

func (h *Handler) logMappedError(ctx context.Context, operation string, original, mapped error, attrs ...slog.Attr) error {
	attrs = append(attrs,
		slog.String("operation", operation),
		slog.String("code", connect.CodeOf(mapped).String()),
		slog.Any("error", original),
	)
	switch connect.CodeOf(mapped) {
	case connect.CodeInternal, connect.CodeUnavailable:
		h.log(ctx, slog.LevelError, "rpc operation failed", attrs...)
	case connect.CodeAborted:
		h.log(ctx, slog.LevelWarn, "rpc operation aborted", attrs...)
	case connect.CodeCanceled, connect.CodeDeadlineExceeded:
		h.log(ctx, slog.LevelInfo, "rpc operation canceled", attrs...)
	}
	return mapped
}

func (h *Handler) providerFailure(ctx context.Context, operation string, err error, attrs ...slog.Attr) error {
	return h.logMappedError(ctx, operation, err, providerError(err), attrs...)
}

func (h *Handler) refreshFailure(ctx context.Context, operation string, err error, attrs ...slog.Attr) error {
	return h.logMappedError(ctx, operation, err, refreshError(err), attrs...)
}

func (h *Handler) sessionFailure(ctx context.Context, operation string, err error, attrs ...slog.Attr) error {
	return h.logMappedError(ctx, operation, err, sessionError(err), attrs...)
}

func (h *Handler) runtimeFailure(ctx context.Context, operation string, err error, attrs ...slog.Attr) error {
	return h.logMappedError(ctx, operation, err, runtimeError(err), attrs...)
}

func (h *Handler) filesystemFailure(ctx context.Context, operation string, err error, attrs ...slog.Attr) error {
	return h.logMappedError(ctx, operation, err, filesystemError(err), attrs...)
}

func (h *Handler) internalFailure(ctx context.Context, operation string, public, cause error, attrs ...slog.Attr) error {
	mapped := connect.NewError(connect.CodeInternal, public)
	return h.logMappedError(ctx, operation, cause, mapped, attrs...)
}
