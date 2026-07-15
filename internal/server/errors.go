package server

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

func providerError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case errors.Is(err, provider.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, provider.ErrBuiltinProvider):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, provider.ErrProviderChanged):
		return connect.NewError(connect.CodeAborted, err)
	default:
		return connect.NewError(connect.CodeInternal, errors.New("provider operation failed"))
	}
}

func refreshError(err error) error {
	if mapped := providerError(err); connect.CodeOf(mapped) != connect.CodeInternal {
		return mapped
	}
	return connect.NewError(connect.CodeUnavailable, errors.New("model refresh failed"))
}

func sessionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case errors.Is(err, store.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, store.ErrUndoConflict):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, errors.New("session operation failed"))
	}
}
