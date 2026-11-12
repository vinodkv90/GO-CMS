package storage

import (
	"context"
	"io"
	"sync"
)

// SwappableProvider delegates StorageProvider calls to an atomically swappable active backend.
type SwappableProvider struct {
	mu       sync.RWMutex
	provider StorageProvider
}

// NewSwappable creates a SwappableProvider initialized with initial.
func NewSwappable(initial StorageProvider) *SwappableProvider {
	return &SwappableProvider{provider: initial}
}

// Swap replaces the active underlying StorageProvider at runtime.
func (s *SwappableProvider) Swap(next StorageProvider) {
	if next == nil {
		return
	}
	s.mu.Lock()
	s.provider = next
	s.mu.Unlock()
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
