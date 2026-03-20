package config

import (
	"context"
	"fmt"
	"sync"

	"webhook-relay/internal/domain"
)

type InMemoryRouteConfigReader struct {
	mu       sync.RWMutex
	channels map[string]domain.Channel
	routes   map[string][]string
}

func NewInMemoryRouteConfigReader(cfg *Config) *InMemoryRouteConfigReader {
	r := &InMemoryRouteConfigReader{}
	r.Update(cfg)
	return r
}

func (r *InMemoryRouteConfigReader) Update(cfg *Config) {
	channels := make(map[string]domain.Channel, len(cfg.Channels))
	for _, c := range cfg.Channels {
		channels[c.ID] = domain.Channel{
			ID: c.ID, Type: domain.ChannelType(c.Type), URL: c.URL,
			Template: c.Template, RetryCount: c.RetryCount,
			RetryDelayMs: c.RetryDelayMs, SkipTLSVerify: c.SkipTLSVerify,
		}
	}
	// sourceID → sourceType mapping (e.g. "beszel" → "BESZEL")
	sourceTypeByID := make(map[string]string, len(cfg.Sources))
	for _, s := range cfg.Sources {
		sourceTypeByID[s.ID] = s.Type
	}
	// Routes keyed by source type so delivery worker can query with alert.Source
	routes := make(map[string][]string, len(cfg.Routes))
	for _, rt := range cfg.Routes {
		key := sourceTypeByID[rt.SourceID]
		if key == "" {
			key = rt.SourceID // fallback
		}
		routes[key] = rt.ChannelIDs
	}
	r.mu.Lock()
	r.channels = channels
	r.routes = routes
	r.mu.Unlock()
}

func (r *InMemoryRouteConfigReader) GetChannels(_ context.Context, sourceID string) ([]domain.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids, ok := r.routes[sourceID]
	if !ok {
		return nil, fmt.Errorf("route for %q: %w", sourceID, domain.ErrSourceNotFound)
	}
	result := make([]domain.Channel, 0, len(ids))
	for _, id := range ids {
		if ch, ok := r.channels[id]; ok {
			result = append(result, ch)
		}
	}
	return result, nil
}
