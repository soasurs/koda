package provider

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

//go:embed catalog/*.json
var catalogFiles embed.FS

var catalogPaths = map[Type]string{
	TypeAnthropic:             "catalog/anthropic.json",
	TypeOpenAIChatCompletions: "catalog/openai_chat_completions.json",
	TypeOpenAIResponses:       "catalog/openai_responses.json",
	TypeGemini:                "catalog/gemini.json",
	TypeDeepSeek:              "catalog/deepseek.json",
}

// ModelCatalog is the effective model list for one provider. RefreshedAt is
// the time of the last successful remote discovery, or zero when the list is
// based only on bundled models and user overrides.
type ModelCatalog struct {
	// Models contains the resolved entries suitable for listing and selection.
	Models []Model
	// RefreshedAt records the last successful remote discovery.
	RefreshedAt time.Time
}

// Discoverer obtains the models currently exposed by one provider API.
type Discoverer interface {
	// Discover returns the models currently exposed by p.
	Discover(context.Context, Provider) ([]Model, error)
}

// Catalog combines bundled model metadata, the last discovered snapshot, and
// provider-specific user overrides.
type Catalog struct {
	registry   *Registry
	discoverer Discoverer
	bundled    map[Type][]Model
	now        func() time.Time
}

// NewCatalog constructs a model catalog. A nil discoverer uses an
// HTTPDiscoverer with its default client.
func NewCatalog(registry *Registry, discoverer Discoverer) (*Catalog, error) {
	if registry == nil {
		return nil, fmt.Errorf("provider catalog: registry must not be nil")
	}
	if discoverer == nil {
		discoverer = NewHTTPDiscoverer(nil)
	}
	bundled, err := loadBundledCatalogs()
	if err != nil {
		return nil, err
	}
	return &Catalog{
		registry:   registry,
		discoverer: discoverer,
		bundled:    bundled,
		now:        time.Now,
	}, nil
}

// List returns the effective local model catalog without accessing the
// network.
func (c *Catalog) List(ctx context.Context, providerID string) (ModelCatalog, error) {
	p, err := c.registry.Get(ctx, providerID)
	if err != nil {
		return ModelCatalog{}, err
	}
	snapshot, err := c.registry.ModelSnapshot(ctx, providerID)
	if err != nil {
		return ModelCatalog{}, err
	}
	return c.resolve(p, snapshot), nil
}

// Refresh discovers models through the provider API, persists the successful
// snapshot, and returns the resulting effective catalog. Failed discovery
// leaves the previous snapshot unchanged.
func (c *Catalog) Refresh(ctx context.Context, providerID string) (ModelCatalog, error) {
	p, err := c.registry.Get(ctx, providerID)
	if err != nil {
		return ModelCatalog{}, err
	}
	models, err := c.discoverer.Discover(ctx, p)
	if err != nil {
		return ModelCatalog{}, fmt.Errorf("provider catalog: refresh %q: %w", p.ID, err)
	}
	models, err = normalizeModels(models)
	if err != nil {
		return ModelCatalog{}, fmt.Errorf("provider catalog: refresh %q: %w", p.ID, err)
	}
	snapshot, err := c.registry.SetModelSnapshot(ctx, p.ID, p.Revision(), ModelSnapshot{
		Models:      models,
		RefreshedAt: c.now().UTC(),
	})
	if err != nil {
		return ModelCatalog{}, err
	}
	p, err = c.registry.Get(ctx, p.ID)
	if err != nil {
		return ModelCatalog{}, err
	}
	return c.resolve(p, snapshot), nil
}

func (c *Catalog) resolve(p Provider, snapshot ModelSnapshot) ModelCatalog {
	bundled := cloneModels(c.bundled[p.Type])
	models := snapshot.Models
	if snapshot.RefreshedAt.IsZero() && p.Builtin() && p.BaseURL == "" {
		models = bundled
	}

	bundledByID := make(map[string]Model, len(bundled))
	for _, model := range bundled {
		bundledByID[model.ID] = model
	}
	effective := make(map[string]Model, len(models)+len(p.ModelOverrides))
	for _, discovered := range models {
		model := discovered
		if known, ok := bundledByID[discovered.ID]; ok {
			model = known
			if discovered.Name != "" {
				model.Name = discovered.Name
			}
		}
		effective[model.ID] = model
	}
	for _, override := range p.ModelOverrides {
		model, ok := effective[override.ID]
		if !ok {
			model = bundledByID[override.ID]
			if model.ID == "" {
				model.ID = override.ID
			}
		}
		if override.Name != "" {
			model.Name = override.Name
		}
		if override.ReasoningEfforts != nil {
			model.ReasoningEfforts = slices.Clone(override.ReasoningEfforts)
			model.DefaultReasoningEffort = ""
		}
		if override.DefaultReasoningEffort != "" {
			model.DefaultReasoningEffort = override.DefaultReasoningEffort
		}
		effective[model.ID] = model
	}

	result := make([]Model, 0, len(effective))
	for _, model := range effective {
		if model.Name == "" {
			model.Name = model.ID
		}
		result = append(result, model)
	}
	slices.SortFunc(result, func(a, b Model) int { return compareModelID(a.ID, b.ID) })
	return ModelCatalog{Models: result, RefreshedAt: snapshot.RefreshedAt}
}

func loadBundledCatalogs() (map[Type][]Model, error) {
	result := make(map[Type][]Model, len(catalogPaths))
	for providerType, path := range catalogPaths {
		data, err := catalogFiles.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("provider catalog: read bundled %q: %w", providerType, err)
		}
		var models []Model
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&models); err != nil {
			return nil, fmt.Errorf("provider catalog: decode bundled %q: %w", providerType, err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return nil, fmt.Errorf("provider catalog: decode bundled %q: %w", providerType, err)
		}
		models, err = normalizeModels(models)
		if err != nil {
			return nil, fmt.Errorf("provider catalog: validate bundled %q: %w", providerType, err)
		}
		if len(models) == 0 {
			return nil, fmt.Errorf("provider catalog: bundled %q is empty", providerType)
		}
		result[providerType] = models
	}
	return result, nil
}

func compareModelID(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
