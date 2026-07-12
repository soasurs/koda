package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	defaultDiscoveryTimeout = 15 * time.Second
	maxDiscoveryBodyBytes   = 4 << 20
	maxDiscoveryPages       = 100
)

// HTTPDiscoverer discovers models through provider-native HTTP APIs.
type HTTPDiscoverer struct {
	client *http.Client
}

// NewHTTPDiscoverer constructs a provider model discoverer. A nil client uses
// a client with a 15-second timeout.
func NewHTTPDiscoverer(client *http.Client) *HTTPDiscoverer {
	if client == nil {
		client = &http.Client{Timeout: defaultDiscoveryTimeout}
	}
	return &HTTPDiscoverer{client: client}
}

// Discover lists models using the API selected by p.Type.
func (d *HTTPDiscoverer) Discover(ctx context.Context, p Provider) ([]Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch p.Type {
	case TypeAnthropic:
		return d.discoverAnthropic(ctx, p)
	case TypeOpenAIChatCompletions, TypeOpenAIResponses, TypeDeepSeek:
		return d.discoverOpenAICompatible(ctx, p)
	case TypeGemini:
		return d.discoverGemini(ctx, p)
	default:
		return nil, fmt.Errorf("provider model discovery: unsupported type %q", p.Type)
	}
}

func (d *HTTPDiscoverer) discoverOpenAICompatible(ctx context.Context, p Provider) ([]Model, error) {
	baseURL := p.BaseURL
	if baseURL == "" {
		switch p.Type {
		case TypeOpenAIChatCompletions, TypeOpenAIResponses:
			baseURL = "https://api.openai.com/v1"
		case TypeDeepSeek:
			baseURL = "https://api.deepseek.com"
		}
	}
	endpoint, err := appendURLPath(baseURL, "models")
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	headers := make(http.Header)
	if p.APIKey() != "" {
		headers.Set("Authorization", "Bearer "+p.APIKey())
	}
	if err := d.getJSON(ctx, endpoint, headers, &response); err != nil {
		return nil, err
	}
	models := make([]Model, 0, len(response.Data))
	seen := make(map[string]struct{}, len(response.Data))
	for _, item := range response.Data {
		appendDiscoveredModel(&models, seen, Model{ID: item.ID})
	}
	return models, nil
}

func (d *HTTPDiscoverer) discoverAnthropic(ctx context.Context, p Provider) ([]Model, error) {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	endpoint, err := appendVersionedURLPath(baseURL, "v1", "models")
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	if p.APIKey() != "" {
		headers.Set("x-api-key", p.APIKey())
	}
	headers.Set("anthropic-version", "2023-06-01")

	models := make([]Model, 0)
	seen := make(map[string]struct{})
	afterID := ""
	for range maxDiscoveryPages {
		pageURL, err := withQuery(endpoint, map[string]string{"limit": "1000", "after_id": afterID})
		if err != nil {
			return nil, err
		}
		var response struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := d.getJSON(ctx, pageURL, headers, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Data {
			appendDiscoveredModel(&models, seen, Model{ID: item.ID, Name: item.DisplayName})
		}
		if !response.HasMore {
			return models, nil
		}
		if response.LastID == "" || response.LastID == afterID {
			return nil, fmt.Errorf("provider model discovery: Anthropic returned an invalid pagination cursor")
		}
		afterID = response.LastID
	}
	return nil, fmt.Errorf("provider model discovery: Anthropic exceeded %d pages", maxDiscoveryPages)
}

func (d *HTTPDiscoverer) discoverGemini(ctx context.Context, p Provider) ([]Model, error) {
	baseURL := p.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	endpoint, err := appendVersionedURLPath(baseURL, "v1beta", "models")
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	if p.APIKey() != "" {
		headers.Set("x-goog-api-key", p.APIKey())
	}

	models := make([]Model, 0)
	seen := make(map[string]struct{})
	pageToken := ""
	for range maxDiscoveryPages {
		pageURL, err := withQuery(endpoint, map[string]string{"pageSize": "1000", "pageToken": pageToken})
		if err != nil {
			return nil, err
		}
		var response struct {
			Models []struct {
				Name                       string   `json:"name"`
				DisplayName                string   `json:"displayName"`
				SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := d.getJSON(ctx, pageURL, headers, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Models {
			if !containsString(item.SupportedGenerationMethods, "generateContent") {
				continue
			}
			appendDiscoveredModel(&models, seen, Model{
				ID:   strings.TrimPrefix(item.Name, "models/"),
				Name: item.DisplayName,
			})
		}
		if response.NextPageToken == "" {
			return models, nil
		}
		if response.NextPageToken == pageToken {
			return nil, fmt.Errorf("provider model discovery: Gemini returned an invalid pagination token")
		}
		pageToken = response.NextPageToken
	}
	return nil, fmt.Errorf("provider model discovery: Gemini exceeded %d pages", maxDiscoveryPages)
}

func (d *HTTPDiscoverer) getJSON(ctx context.Context, endpoint string, headers http.Header, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("provider model discovery: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("provider model discovery: send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("provider model discovery: unexpected HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryBodyBytes+1))
	if err != nil {
		return fmt.Errorf("provider model discovery: read response: %w", err)
	}
	if len(body) > maxDiscoveryBodyBytes {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("provider model discovery: response exceeds %d bytes", maxDiscoveryBodyBytes)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("provider model discovery: decode response: %w", err)
	}
	return nil
}

func appendURLPath(baseURL, element string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("provider model discovery: invalid base URL %q", baseURL)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + element
	return u.String(), nil
}

func appendVersionedURLPath(baseURL, version, element string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("provider model discovery: invalid base URL %q", baseURL)
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" || !strings.HasSuffix(path, "/"+version) {
		path += "/" + version
	}
	u.Path = path + "/" + element
	return u.String(), nil
}

func withQuery(endpoint string, values map[string]string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("provider model discovery: parse endpoint: %w", err)
	}
	query := u.Query()
	for name, value := range values {
		if value == "" {
			continue
		}
		query.Set(name, value)
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func appendDiscoveredModel(models *[]Model, seen map[string]struct{}, model Model) {
	model.ID = strings.TrimSpace(model.ID)
	if model.ID == "" {
		return
	}
	if _, ok := seen[model.ID]; ok {
		return
	}
	seen[model.ID] = struct{}{}
	model.Name = strings.TrimSpace(model.Name)
	*models = append(*models, model)
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

var _ Discoverer = (*HTTPDiscoverer)(nil)
