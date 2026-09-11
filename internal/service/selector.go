package service

import (
	"errors"

	"go-micro.dev/v6/registry"
	"go-micro.dev/v6/selector"
)

// The memory registry is already local, synchronized state. A second cache
// adds no discovery benefit, and its concurrent cold-refresh rate limiter can
// report not-found before a successful first lookup has populated the cache.
// Keep the standard selector lifecycle/options, but read local state directly.
type memorySelector struct{ selector.Selector }

func serviceSelector(r registry.Registry) selector.Selector {
	s := selector.NewSelector(selector.Registry(r))
	if r.String() == "memory" {
		return &memorySelector{Selector: s}
	}
	return s
}

func (s *memorySelector) Select(name string, options ...selector.SelectOption) (selector.Next, error) {
	base := s.Options()
	services, err := base.Registry.GetService(name)
	if errors.Is(err, registry.ErrNotFound) {
		return nil, selector.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	opts := selector.SelectOptions{Strategy: base.Strategy}
	for _, option := range options {
		option(&opts)
	}
	for _, filter := range opts.Filters {
		services = filter(services)
	}
	if len(services) == 0 {
		return nil, selector.ErrNoneAvailable
	}
	return opts.Strategy(services), nil
}
