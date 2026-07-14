package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
	kodav1connect "github.com/soasurs/koda/gen/koda/v1/kodav1connect"
	"github.com/soasurs/koda/internal/logging"
	"github.com/soasurs/koda/internal/provider"
	"github.com/soasurs/koda/internal/store"
)

func TestSaveProviderLogsCredentialActionWithoutCredential(t *testing.T) {
	client, _, handler := newTestService(t, staticDiscoverer{})
	var output bytes.Buffer
	logger, err := logging.New(&output, "info", "")
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	handler.logger = logger
	const apiKey = "top-secret-provider-key"
	if _, err := client.SaveProvider(t.Context(), v1.SaveProviderRequest_builder{
		Id: new("custom"), Name: new("Custom"),
		Type: v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES.Enum(), ApiKey: proto.String(apiKey),
	}.Build()); err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	got := output.String()
	if !strings.Contains(got, "msg=\"provider saved\"") || !strings.Contains(got, "credential_action=set") {
		t.Fatalf("logger output = %q", got)
	}
	if strings.Contains(got, apiKey) {
		t.Fatalf("logger output contains API key: %q", got)
	}
}

func TestProviderAndModelHandlers(t *testing.T) {
	client, registry := newTestClient(t, staticDiscoverer{models: []provider.Model{{ID: "discovered-model"}}})

	listed, err := client.ListProviders(t.Context(), v1.ListProvidersRequest_builder{}.Build())
	if err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	providers := listed.GetProviders()
	if got, want := len(providers), 5; got != want {
		t.Fatalf("len(ListProviders().Providers) = %d, want %d", got, want)
	}
	if providers[0].GetId() != "anthropic" || !providers[0].GetBuiltin() {
		t.Fatalf("first provider = %+v, want built-in Anthropic", providers[0])
	}

	apiKey := "test-api-key"
	modelOverride := v1.Model_builder{
		Id:                     new("private-model"),
		ReasoningEfforts:       []string{"low", "max"},
		DefaultReasoningEffort: new("max"),
	}.Build()
	saved, err := client.SaveProvider(t.Context(), v1.SaveProviderRequest_builder{
		Id:             new("custom"),
		Name:           new("Custom"),
		Type:           v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES.Enum(),
		BaseUrl:        new("https://models.example/v1"),
		ApiKey:         new(apiKey),
		ModelOverrides: []*v1.Model{modelOverride},
	}.Build())
	if err != nil {
		t.Fatalf("SaveProvider() error = %v", err)
	}
	savedProvider := saved.GetProvider()
	if savedProvider.GetId() != "custom" || !savedProvider.GetConfigured() || savedProvider.GetBuiltin() {
		t.Fatalf("SaveProvider() = %+v, want configured custom provider", savedProvider)
	}
	stored, err := registry.Get(t.Context(), "custom")
	if err != nil {
		t.Fatalf("Registry.Get() error = %v", err)
	}
	if stored.APIKey() != apiKey {
		t.Fatal("SaveProvider() did not persist the API key")
	}

	updated, err := client.SaveProvider(t.Context(), v1.SaveProviderRequest_builder{
		Id:             new("custom"),
		Name:           new("Custom Updated"),
		Type:           v1.ProviderType_PROVIDER_TYPE_OPENAI_RESPONSES.Enum(),
		BaseUrl:        new("https://models.example/v1"),
		ModelOverrides: []*v1.Model{modelOverride},
	}.Build())
	if err != nil {
		t.Fatalf("SaveProvider(omit API key) error = %v", err)
	}
	updatedProvider := updated.GetProvider()
	if updatedProvider.GetName() != "Custom Updated" || !updatedProvider.GetConfigured() {
		t.Fatalf("SaveProvider(omit API key) = %+v", updatedProvider)
	}
	stored, err = registry.Get(t.Context(), "custom")
	if err != nil {
		t.Fatalf("Registry.Get(updated) error = %v", err)
	}
	if stored.APIKey() != apiKey {
		t.Fatal("omitted API key did not preserve the stored credential")
	}

	models, err := client.ListModels(t.Context(), v1.ListModelsRequest_builder{ProviderId: new("custom")}.Build())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if models.GetRefreshedAt() != 0 || len(models.GetModels()) != 1 || models.GetModels()[0].GetId() != "private-model" {
		t.Fatalf("ListModels() = %+v, want local override only", models)
	}
	if !slices.Equal(models.GetModels()[0].GetReasoningEfforts(), []string{"low", "max"}) {
		t.Fatalf("ListModels().Models[0].ReasoningEfforts = %v", models.GetModels()[0].GetReasoningEfforts())
	}

	refreshed, err := client.RefreshModels(t.Context(), v1.RefreshModelsRequest_builder{ProviderId: new("custom")}.Build())
	if err != nil {
		t.Fatalf("RefreshModels() error = %v", err)
	}
	if refreshed.GetRefreshedAt() == 0 || !containsModel(refreshed.GetModels(), "private-model") || !containsModel(refreshed.GetModels(), "discovered-model") {
		t.Fatalf("RefreshModels() = %+v, want refreshed private and discovered models", refreshed)
	}

	if _, err := client.DeleteProvider(t.Context(), v1.DeleteProviderRequest_builder{ProviderId: new("custom")}.Build()); err != nil {
		t.Fatalf("DeleteProvider() error = %v", err)
	}
	if _, err := client.ListModels(t.Context(), v1.ListModelsRequest_builder{ProviderId: new("custom")}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("ListModels(deleted) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
}

func TestProviderAndModelHandlersMapErrors(t *testing.T) {
	client, _ := newTestClient(t, staticDiscoverer{err: errors.New("offline")})

	if _, err := client.SaveProvider(t.Context(), v1.SaveProviderRequest_builder{
		Id:   new("invalid"),
		Name: new("Invalid"),
		Type: v1.ProviderType_PROVIDER_TYPE_UNSPECIFIED.Enum(),
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SaveProvider(unspecified type) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.SaveProvider(t.Context(), v1.SaveProviderRequest_builder{
		Id:   new("openai"),
		Name: new("OpenAI"),
		Type: v1.ProviderType_PROVIDER_TYPE_ANTHROPIC.Enum(),
	}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("SaveProvider(change built-in type) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.DeleteProvider(t.Context(), v1.DeleteProviderRequest_builder{ProviderId: new("openai")}.Build()); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("DeleteProvider(built-in) code = %v, want failed_precondition; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.ListModels(t.Context(), v1.ListModelsRequest_builder{ProviderId: new("missing")}.Build()); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("ListModels(missing) code = %v, want not_found; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.ListModels(t.Context(), v1.ListModelsRequest_builder{}.Build()); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListModels(empty provider) code = %v, want invalid_argument; error = %v", connect.CodeOf(err), err)
	}
	if _, err := client.RefreshModels(t.Context(), v1.RefreshModelsRequest_builder{ProviderId: new("openai")}.Build()); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("RefreshModels(offline) code = %v, want unavailable; error = %v", connect.CodeOf(err), err)
	}
}

func TestNewHandlerRequiresDependencies(t *testing.T) {
	if _, err := NewHandler(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("NewHandler(nil, nil, nil) error = nil, want error")
	}

	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	if _, err := NewHandler(registry, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("NewHandler(registry, nil, nil) error = nil, want error")
	}
	catalog, err := provider.NewCatalog(registry, staticDiscoverer{})
	if err != nil {
		t.Fatalf("provider.NewCatalog() error = %v", err)
	}
	if _, err := NewHandler(registry, catalog, nil, nil, nil, nil); err == nil {
		t.Fatal("NewHandler(registry, catalog, nil) error = nil, want error")
	}
	sessionStore := openTestStore(t)
	otherRegistry, err := provider.Open(filepath.Join(t.TempDir(), "other-providers.json"))
	if err != nil {
		t.Fatalf("provider.Open(other) error = %v", err)
	}
	otherCatalog, err := provider.NewCatalog(otherRegistry, staticDiscoverer{})
	if err != nil {
		t.Fatalf("provider.NewCatalog(other) error = %v", err)
	}
	if _, err := NewHandler(registry, otherCatalog, sessionStore, nil, nil, nil); err == nil {
		t.Fatal("NewHandler(mismatched dependencies) error = nil, want error")
	}
}

func newTestClient(t *testing.T, discoverer provider.Discoverer) (kodav1connect.KodaServiceClient, *provider.Registry) {
	client, registry, _ := newTestService(t, discoverer)
	return client, registry
}

func newTestService(t *testing.T, discoverer provider.Discoverer) (kodav1connect.KodaServiceClient, *provider.Registry, *Handler) {
	t.Helper()
	registry, err := provider.Open(filepath.Join(t.TempDir(), "providers.json"))
	if err != nil {
		t.Fatalf("provider.Open() error = %v", err)
	}
	catalog, err := provider.NewCatalog(registry, discoverer)
	if err != nil {
		t.Fatalf("provider.NewCatalog() error = %v", err)
	}
	sessionStore := openTestStore(t)
	handler, err := NewHandler(registry, catalog, sessionStore, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	path, serviceHandler := kodav1connect.NewKodaServiceHandler(handler)
	mux := http.NewServeMux()
	mux.Handle(path, serviceHandler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return kodav1connect.NewKodaServiceClient(server.Client(), server.URL), registry, handler
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	sessionStore, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "koda.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := sessionStore.Close(); err != nil {
			t.Errorf("Store.Close() error = %v", err)
		}
	})
	return sessionStore
}

func containsModel(models []*v1.Model, id string) bool {
	for _, model := range models {
		if model.GetId() == id {
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
