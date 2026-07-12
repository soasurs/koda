package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestHTTPDiscovererOpenAICompatible(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		fmt.Fprint(w, `{"data":[{"id":"model-b"},{"id":"model-a"},{"id":"model-a"}]}`)
	}))
	defer server.Close()

	discoverer := NewHTTPDiscoverer(server.Client())
	models, err := discoverer.Discover(t.Context(), Provider{
		Type:    TypeOpenAIResponses,
		BaseURL: server.URL + "/v1",
		apiKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := modelIDs(models), []string{"model-b", "model-a"}; !slices.Equal(got, want) {
		t.Fatalf("model IDs = %v, want %v", got, want)
	}
}

func TestHTTPDiscovererAnthropicPaginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q, want /v1/models", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "anthropic-key" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("Anthropic headers = %v", r.Header)
		}
		switch r.URL.Query().Get("after_id") {
		case "":
			fmt.Fprint(w, `{"data":[{"id":"claude-a","display_name":"Claude A"}],"has_more":true,"last_id":"claude-a"}`)
		case "claude-a":
			fmt.Fprint(w, `{"data":[{"id":"claude-b","display_name":"Claude B"}],"has_more":false,"last_id":"claude-b"}`)
		default:
			t.Errorf("unexpected after_id = %q", r.URL.Query().Get("after_id"))
		}
	}))
	defer server.Close()

	models, err := NewHTTPDiscoverer(server.Client()).Discover(t.Context(), Provider{
		Type:    TypeAnthropic,
		BaseURL: server.URL,
		apiKey:  "anthropic-key",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := modelIDs(models), []string{"claude-a", "claude-b"}; !slices.Equal(got, want) {
		t.Fatalf("model IDs = %v, want %v", got, want)
	}
}

func TestHTTPDiscovererGeminiFiltersAndPaginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Errorf("path = %q, want /v1beta/models", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-key" {
			t.Errorf("x-goog-api-key = %q", got)
		}
		switch r.URL.Query().Get("pageToken") {
		case "":
			fmt.Fprint(w, `{"models":[{"name":"models/gemini-a","displayName":"Gemini A","supportedGenerationMethods":["generateContent"]},{"name":"models/embed-a","displayName":"Embed A","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"next"}`)
		case "next":
			fmt.Fprint(w, `{"models":[{"name":"models/gemini-b","displayName":"Gemini B","supportedGenerationMethods":["generateContent"]}]}`)
		default:
			t.Errorf("unexpected pageToken = %q", r.URL.Query().Get("pageToken"))
		}
	}))
	defer server.Close()

	models, err := NewHTTPDiscoverer(server.Client()).Discover(t.Context(), Provider{
		Type:    TypeGemini,
		BaseURL: server.URL,
		apiKey:  "gemini-key",
	})
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := modelIDs(models), []string{"gemini-a", "gemini-b"}; !slices.Equal(got, want) {
		t.Fatalf("model IDs = %v, want %v", got, want)
	}
}

func TestHTTPDiscovererDoesNotReturnErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"secret response"}`)
	}))
	defer server.Close()

	_, err := NewHTTPDiscoverer(server.Client()).Discover(t.Context(), Provider{
		Type:    TypeDeepSeek,
		BaseURL: server.URL,
	})
	if err == nil {
		t.Fatal("Discover() error = nil")
	}
	if got := err.Error(); got != "provider model discovery: unexpected HTTP status 401" {
		t.Fatalf("error = %q", got)
	}
}

func modelIDs(models []Model) []string {
	result := make([]string, len(models))
	for i, model := range models {
		result[i] = model.ID
	}
	return result
}
