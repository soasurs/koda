package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"

	"connectrpc.com/connect"
	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/provider"
)

func TestProviderAndModelHandlers(t *testing.T) {
	client, registry := newTestClient(t, staticDiscoverer{models: []provider.Model{{ID: "discovered-model"}}})

	listed, err := client.ListProviders(t.Context(), &v1.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if got, want := len(listed.Providers), 5; got != want {
		t.Fatalf("len(ListProviders().Providers) = %d, want %d", got, want)
	}
	if listed.Providers[0].Id != "anthropic" || !listed.Providers[0].Builtin {
		t.Fatalf("first provider = %+v, want built-in Anthropic", listed.Providers[0])
	}

	apiKey := "test-api-key"
	saved, err := client.SaveProvider(t.Context(), &v1.SaveProviderRequest{
		Id:      "custom",
		Name:    "Custom",
		Type:    v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES,
		BaseUrl: "https://models.example/v1",
		ApiKey:  &apiKey,
		ModelOverrides: []*v1.Model{{
			Id:                     "private-model",
			ReasoningEfforts:       []string{"low", "max"},
			DefaultReasoningEffort: "max",
		}},
	})
	if err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	if saved.Provider.Id != "custom" || !saved.Provider.Configured || saved.Provider.Builtin {
		t.Fatalf("SaveProvider() = %+v, want configured custom provider", saved.Provider)
	}
	stored, err := registry.Get(t.Context(), "custom")
	if err != nil {
		t.Fatalf("Registry.Get() error = %v", err)
	}
	if stored.APIKey() != apiKey {
		t.Fatal("SaveProvider() did not persist the API key")
	}

	updated, err := client.SaveProvider(t.Context(), &v1.SaveProviderRequest{
		Id:      "custom",
		Name:    "Custom Updated",
		Type:    v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES,
		BaseUrl: "https://models.example/v1",
		ModelOverrides: []*v1.Model{{
			Id:                     "private-model",
			ReasoningEfforts:       []string{"low", "max"},
			DefaultReasoningEffort: "max",
		}},
	})
	if err != nil {
		t.Fatalf("SaveProvider(omit API key) error = %v", err)
	}
	if updated.Provider.Name != "Custom Updated" || !updated.Provider.Configured {
		t.Fatalf("SaveProvider(omit API key) = %+v", updated.Provider)
	}
	stored, err = registry.Get(t.Context(), "custom")
	if err != nil {
		t.Fatalf("Registry.Get(updated) error = %v", err)
	}
	if stored.APIKey() != apiKey {
		t.Fatal("omitted API key did not preserve the stored credential")
	}

	models, err := client.ListModels(t.Context(), &v1.ListModelsRequest{ProviderId: "custom"})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if models.RefreshedAt != 0 || len(models.Models) != 1 || models.Models[0].Id != "private-model" {
		t.Fatalf("ListModels() = %+v, want local override only", models)
	}
	if !slices.Equal(models.Models[0].ReasoningEfforts, []string{"low", "max"}) {
		t.Fatalf("ListModels().Models[0].ReasoningEfforts = %v", models.Models[0].ReasoningEfforts)
	}

	refreshed, err := client.RefreshModels(t.Context(), &v1.RefreshModelsRequest{ProviderId: "custom"})
	if err != nil {
		t.Fatalf("RefreshModels() error = %v", err)
	}
	if refreshed.RefreshedAt == 0 || !containsModel(refreshed.Models, "private-model") || !containsModel(refreshed.Models, "discovered-model") {
		t.Fatalf("RefreshModels() = %+v, want refreshed private and discovered models", refreshed)
	}

	if _, err := client.DeleteProvider(t.Context(), &v1.DeleteProviderRequest{ProviderId: "custom"}); err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}
	if _, err := client.ListModels(t.Context(), &v1.ListModelsRequest{ProviderId: "custom"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("ListModels(deleted) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestProviderAndModelHandlersMapErrors(t *testing.T) {
	client, _ := newTestClient(t, staticDiscoverer{err: errors.New("offline")})

	if _, err := client.SaveProvider(t.Context(), &v1.SaveProviderRequest{
		Id:   "invalid",
		Name: "Invalid",
		Type: v1.ProviderType_PROVIDER_TYPE_UNSPECIFIED,
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SaveProvider(unspecified type) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.SaveProvider(t.Context(), &v1.SaveProviderRequest{
		Id:   "openai",
		Name: "OpenAI",
		Type: v1.ProviderType_PROVIDER_TYPE_ANTHROPIC,
	}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SaveProvider(change built-in type) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.DeleteProvider(t.Context(), &v1.DeleteProviderRequest{ProviderId: "openai"}); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("DeleteProvider(built-in) code = %v, want failed_precondition; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.ListModels(t.Context(), &v1.ListModelsRequest{ProviderId: "missing"}); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("ListModels(missing) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.ListModels(t.Context(), &v1.ListModelsRequest{}); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListModels(empty provider) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.RefreshModels(t.Context(), &v1.RefreshModelsRequest{ProviderId: "openai"}); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("RefreshModels(offline) code = %v, want unavailable; error = %v", connect.CodeOf(err), err)
	}
}

func TestNewHandlerRequiresDependencies(t *testing.T) {
	if _, err := NewHandler(nil, nil); err == nil {
		t.Fatal("NewHandler(nil, nil) error = nil, want error")
	}

	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	if _, err := NewHandler(registry, nil); err == nil {
		t.Fatal("NewHandler(registry, nil) error = nil, want error")
	}
	otherRegistry, err := provider.Open(filepath.Join(t.TempDir(), "other-providers.json"))
	if err != nil {
		t.Fatalf("provider.Open(other) error = %v", err)
	}
	catalog, err := provider.NewCatalog(otherRegistry, staticDiscoverer{})
	if err != nil {
		t.Fatalf("provider.NewCatalog(other) error = %v", err)
	}
	if _, err := NewHandler(registry, catalog); err == nil {
		t.Fatal("NewHandler(mismatched dependencies) error = nil, want error")
	}
}

func newTestClient(t *testing.T, discoverer provider.Discoverer) (kodav1connect.KodaServiceClient, *provider.Registry) {
	t.Helper()
	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	catalog, err := provider.NewCatalog(registry, discoverer)
	if err != nil {
		t.Fatalf("provider.NewCatalog() error = %v", err)
	}
	handler, err := NewHandler(registry, catalog)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	path, serviceHandler := kodav1connect.NewKodaServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(path, serviceHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return kodav1connect.NewKodaServiceClient(server.Client(), server.URL), registry
}

func containsModel(models []*v1.Model, id string) bool {
	for _, model := range models {
		if model.Id == id {
			return true
		}
	}
	return false
}

type staticDiscoverer struct {
	models []provider.Model
	err    error
}

func (d staticDiscoverer) Discover(ctx context.Context, _ provider.Provider) ([]provider.Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.err != nil {
		return nil, d.err
	}
	return slices.Clone(d.models), nil
}
