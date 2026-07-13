package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	"github.com/soasurs/koda/internal/provider"
)

// ListProviders returns every configured and built-in provider.
func (h *Handler) ListProviders(ctx context.Context, _ *v1.ListProvidersRequest) (*v1.ListProvidersResponse, error) {
	providers, err := h.registry.List(ctx)
	if err != nil {
		return nil, h.providerFailure(ctx, "list providers", err)
	}
	return v1.ListProvidersResponse_builder{Providers: providersToProto(providers)}.Build(), nil
}

// SaveProvider creates or replaces a provider configuration.
func (h *Handler) SaveProvider(ctx context.Context, request *v1.SaveProviderRequest) (*v1.SaveProviderResponse, error) {
	p, err := providerFromProto(request)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if current, err := h.registry.Get(ctx, p.ID); err == nil {
		if current.Builtin() && current.Type != p.Type {
			return nil, connect.NewError(
				connect.CodeInvalidArgument,
				fmt.Errorf("built-in provider %q must retain type %q", p.ID, current.Type),
			)
		}
	} else if !errors.Is(err, provider.ErrNotFound) {
		return nil, h.providerFailure(ctx, "load provider for save", err, slog.String("provider_id", p.ID))
	}

	var apiKey *string
	if request.HasApiKey() {
		value := request.GetApiKey()
		apiKey = &value
	}
	credentialAction := "preserve"
	if apiKey != nil {
		credentialAction = "set"
		if *apiKey == "" {
			credentialAction = "clear"
		}
	}
	saved, err := h.registry.Save(ctx, p, apiKey)
	if err != nil {
		return nil, h.providerFailure(ctx, "save provider", err, slog.String("provider_id", p.ID))
	}
	h.log(ctx, slog.LevelInfo, "provider saved",
		slog.String("provider_id", saved.ID),
		slog.String("provider_type", string(saved.Type)),
		slog.String("credential_action", credentialAction),
	)
	return v1.SaveProviderResponse_builder{Provider: providerToProto(saved)}.Build(), nil
}

// DeleteProvider deletes one user-defined provider.
func (h *Handler) DeleteProvider(ctx context.Context, request *v1.DeleteProviderRequest) (*v1.DeleteProviderResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("delete provider request must not be nil"))
	}
	providerID, err := providerIDFromRequest(request.GetProviderId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := h.registry.Delete(ctx, providerID); err != nil {
		return nil, h.providerFailure(ctx, "delete provider", err, slog.String("provider_id", providerID))
	}
	h.log(ctx, slog.LevelInfo, "provider deleted", slog.String("provider_id", providerID))
	return v1.DeleteProviderResponse_builder{}.Build(), nil
}

// ListModels returns the local effective model catalog for one provider.
func (h *Handler) ListModels(ctx context.Context, request *v1.ListModelsRequest) (*v1.ListModelsResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("list models request must not be nil"))
	}
	providerID, err := providerIDFromRequest(request.GetProviderId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	catalog, err := h.catalog.List(ctx, providerID)
	if err != nil {
		return nil, h.providerFailure(ctx, "list models", err, slog.String("provider_id", providerID))
	}
	return v1.ListModelsResponse_builder{
		ProviderId:  proto.String(providerID),
		Models:      modelsToProto(catalog.Models),
		RefreshedAt: proto.Int64(unixMilli(catalog.RefreshedAt)),
	}.Build(), nil
}

// RefreshModels discovers and persists the effective model catalog for one provider.
func (h *Handler) RefreshModels(ctx context.Context, request *v1.RefreshModelsRequest) (*v1.RefreshModelsResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("refresh models request must not be nil"))
	}
	providerID, err := providerIDFromRequest(request.GetProviderId())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	startedAt := time.Now()
	h.log(ctx, slog.LevelInfo, "model refresh started", slog.String("provider_id", providerID))
	catalog, err := h.catalog.Refresh(ctx, providerID)
	if err != nil {
		return nil, h.refreshFailure(ctx, "refresh models", err,
			slog.String("provider_id", providerID),
			slog.Duration("duration", time.Since(startedAt)),
		)
	}
	h.log(ctx, slog.LevelInfo, "model refresh completed",
		slog.String("provider_id", providerID),
		slog.Int("model_count", len(catalog.Models)),
		slog.Duration("duration", time.Since(startedAt)),
	)
	return v1.RefreshModelsResponse_builder{
		ProviderId:  proto.String(providerID),
		Models:      modelsToProto(catalog.Models),
		RefreshedAt: proto.Int64(unixMilli(catalog.RefreshedAt)),
	}.Build(), nil
}

func providerFromProto(request *v1.SaveProviderRequest) (provider.Provider, error) {
	if request == nil {
		return provider.Provider{}, errors.New("save provider request must not be nil")
	}
	providerType, err := providerTypeFromProto(request.GetType())
	if err != nil {
		return provider.Provider{}, err
	}
	modelOverrides := request.GetModelOverrides()
	models := make([]provider.Model, len(modelOverrides))
	for i, model := range modelOverrides {
		if model == nil {
			return provider.Provider{}, fmt.Errorf("model override %d must not be nil", i)
		}
		models[i] = modelFromProto(model)
	}
	p := provider.Provider{
		ID:             request.GetId(),
		Name:           request.GetName(),
		Type:           providerType,
		BaseURL:        request.GetBaseUrl(),
		ModelOverrides: models,
		Enabled:        true,
	}
	if request.HasEnabled() {
		p.Enabled = request.GetEnabled()
	}
	if err := provider.ValidateProvider(p); err != nil {
		return provider.Provider{}, err
	}
	return p, nil
}

func providerIDFromRequest(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", errors.New("provider ID must not be empty")
	}
	return id, nil
}

func providerToProto(p provider.Provider) *v1.Provider {
	return v1.Provider_builder{
		Id:         proto.String(p.ID),
		Name:       proto.String(p.Name),
		Type:       providerTypeToProto(p.Type).Enum(),
		BaseUrl:    proto.String(p.BaseURL),
		Configured: proto.Bool(p.Configured()),
		Builtin:    proto.Bool(p.Builtin()),
		Enabled:    proto.Bool(p.Enabled),
	}.Build()
}

func providersToProto(providers []provider.Provider) []*v1.Provider {
	result := make([]*v1.Provider, len(providers))
	for i, p := range providers {
		result[i] = providerToProto(p)
	}
	return result
}

func modelFromProto(model *v1.Model) provider.Model {
	return provider.Model{
		ID:                     model.GetId(),
		Name:                   model.GetName(),
		ReasoningEfforts:       slices.Clone(model.GetReasoningEfforts()),
		DefaultReasoningEffort: model.GetDefaultReasoningEffort(),
	}
}

func modelsToProto(models []provider.Model) []*v1.Model {
	result := make([]*v1.Model, len(models))
	for i, model := range models {
		result[i] = v1.Model_builder{
			Id:                     proto.String(model.ID),
			Name:                   proto.String(model.Name),
			ReasoningEfforts:       slices.Clone(model.ReasoningEfforts),
			DefaultReasoningEffort: proto.String(model.DefaultReasoningEffort),
		}.Build()
	}
	return result
}

func providerTypeFromProto(providerType v1.ProviderType) (provider.Type, error) {
	switch providerType {
	case v1.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		return provider.TypeAnthropic, nil
	case v1.ProviderType_PROVIDER_TYPE_OPENAI_CHAT_COMPLETIONS:
		return provider.TypeOpenAIChatCompletions, nil
	case v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES:
		return provider.TypeOpenAIResponses, nil
	case v1.ProviderType_PROVIDER_TYPE_GEMINI:
		return provider.TypeGemini, nil
	case v1.ProviderType_PROVIDER_TYPE_DEEPSEEK:
		return provider.TypeDeepSeek, nil
	default:
		return "", fmt.Errorf("unsupported provider type %q", providerType)
	}
}

func providerTypeToProto(providerType provider.Type) v1.ProviderType {
	switch providerType {
	case provider.TypeAnthropic:
		return v1.ProviderType_PROVIDER_TYPE_ANTHROPIC
	case provider.TypeOpenAIChatCompletions:
		return v1.ProviderType_PROVIDER_TYPE_OPENAI_CHAT_COMPLETIONS
	case provider.TypeOpenAIResponses:
		return v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES
	case provider.TypeGemini:
		return v1.ProviderType_PROVIDER_TYPE_GEMINI
	case provider.TypeDeepSeek:
		return v1.ProviderType_PROVIDER_TYPE_DEEPSEEK
	default:
		return v1.ProviderType_PROVIDER_TYPE_UNSPECIFIED
	}
}

func unixMilli(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixMilli()
}
