package storage

import (
	"context"
	"io"
	"sync"
)

// SwappableProvider delegates StorageProvider calls to an atomically swappable active backend
// while keeping registered providers available for per-asset lookups.
type SwappableProvider struct {
	mu        sync.RWMutex
	provider  StorageProvider
	providers map[string]StorageProvider
}

var (
	_ ProviderResolver   = (*SwappableProvider)(nil)
	_ CapabilityReporter = (*SwappableProvider)(nil)
)

// NewSwappable creates a SwappableProvider initialized with initial.
func NewSwappable(initial StorageProvider) *SwappableProvider {
	s := &SwappableProvider{
		provider:  initial,
		providers: make(map[string]StorageProvider),
	}
	if initial != nil {
		s.providers[initial.Name()] = initial
	}
	return s
}

// Swap replaces the active underlying StorageProvider at runtime and registers it for per-asset lookups.
func (s *SwappableProvider) Swap(next StorageProvider) {
	if next == nil {
		return
	}
	s.mu.Lock()
	s.provider = next
	if s.providers == nil {
		s.providers = make(map[string]StorageProvider)
	}
	s.providers[next.Name()] = next
	s.mu.Unlock()
}

// RegisterProvider registers a named storage provider without changing the active driver.
func (s *SwappableProvider) RegisterProvider(p StorageProvider) {
	if p == nil {
		return
	}
	s.mu.Lock()
	if s.providers == nil {
		s.providers = make(map[string]StorageProvider)
	}
	s.providers[p.Name()] = p
	s.mu.Unlock()
}

// ResolveProvider returns a previously registered provider by name, or the active provider if names match.
func (s *SwappableProvider) ResolveProvider(name string) (StorageProvider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.providers[name]; ok && p != nil {
		return p, true
	}
	if s.provider != nil && s.provider.Name() == name {
		return s.provider, true
	}
	return nil, false
}

// Capabilities reports the active provider's capabilities, or defaults for basic drivers.
func (s *SwappableProvider) Capabilities() ProviderCapabilities {
	p, err := s.current()
	if err != nil || p == nil {
		return ProviderCapabilities{}
	}
	if reporter, ok := p.(CapabilityReporter); ok {
		return reporter.Capabilities()
	}
	return ProviderCapabilities{
		Provider:              p.Name(),
		SupportsHLS:           true,
		SupportsSignedURLs:    true,
		SupportsTransforms:    false,
		SupportsFolderMapping: true,
	}
}

func (s *SwappableProvider) current() (StorageProvider, error) {
	s.mu.RLock()
	p := s.provider
	s.mu.RUnlock()
	if p == nil {
		return nil, ErrStorageOperation
	}
	return p, nil
}

func (s *SwappableProvider) Name() string {
	p, err := s.current()
	if err != nil {
		return ""
	}
	return p.Name()
}

func (s *SwappableProvider) Save(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error {
	p, err := s.current()
	if err != nil {
		return err
	}
	return p.Save(ctx, key, r, size, mimeType)
}

func (s *SwappableProvider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	p, err := s.current()
	if err != nil {
		return nil, err
	}
	return p.Open(ctx, key)
}

func (s *SwappableProvider) Delete(ctx context.Context, key string) error {
	p, err := s.current()
	if err != nil {
		return err
	}
	return p.Delete(ctx, key)
}

func (s *SwappableProvider) Exists(ctx context.Context, key string) (bool, error) {
	p, err := s.current()
	if err != nil {
		return false, err
	}
	return p.Exists(ctx, key)
}

func (s *SwappableProvider) URL(ctx context.Context, key string) (string, error) {
	p, err := s.current()
	if err != nil {
		return "", err
	}
	return p.URL(ctx, key)
}

func (s *SwappableProvider) Move(ctx context.Context, srcKey, dstKey string) error {
	p, err := s.current()
	if err != nil {
		return err
	}
	return p.Move(ctx, srcKey, dstKey)
}

func (s *SwappableProvider) DeletePrefix(ctx context.Context, prefix string) error {
	p, err := s.current()
	if err != nil {
		return err
	}
	return p.DeletePrefix(ctx, prefix)
}

// CheckHealth forwards health checks to the active provider if supported.
func (s *SwappableProvider) CheckHealth(ctx context.Context) error {
	p, err := s.current()
	if err != nil {
		return err
	}
	if hc, ok := p.(HealthChecker); ok {
		return hc.CheckHealth(ctx)
	}
	return nil
}
