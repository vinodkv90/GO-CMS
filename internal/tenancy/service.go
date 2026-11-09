package tenancy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Service provides multi-tenant management and lookup functions.
type Service struct {
	mu      sync.RWMutex
	tenants map[string]*Tenant // ID -> Tenant
	bySlug  map[string]string  // Slug -> ID
	byDom   map[string]string  // Domain -> ID
	defID   string
}

// NewService creates a new tenancy Service seeded with the default tenant.
func NewService() *Service {
	s := &Service{
		tenants: make(map[string]*Tenant),
		bySlug:  make(map[string]string),
		byDom:   make(map[string]string),
	}

	defID := "tenant-default"
	now := time.Now().UTC()
	defaultTenant := &Tenant{
		ID:        defID,
		Name:      "Default Workspace",
		Slug:      "default",
		Domain:    "",
		Status:    StatusActive,
		Settings:  map[string]any{"tier": "enterprise"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.tenants[defID] = defaultTenant
	s.bySlug["default"] = defID
	s.defID = defID

	return s
}

// DefaultTenant returns the fallback default tenant.
func (s *Service) DefaultTenant() *Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tenants[s.defID]
}

// Create registers a new tenant.
func (s *Service) Create(ctx context.Context, t Tenant) (*Tenant, error) {
	name := strings.TrimSpace(t.Name)
	slug := strings.ToLower(strings.TrimSpace(t.Slug))
	domain := strings.ToLower(strings.TrimSpace(t.Domain))

	if name == "" {
		return nil, errors.New("tenancy: tenant name cannot be empty")
	}
	if slug == "" {
		return nil, errors.New("tenancy: tenant slug cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.bySlug[slug]; exists {
		return nil, fmt.Errorf("tenancy: slug %q is already taken", slug)
	}

	if domain != "" {
		if _, exists := s.byDom[domain]; exists {
			return nil, fmt.Errorf("tenancy: domain %q is already in use", domain)
		}
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	status := t.Status
	if status == "" {
		status = StatusActive
	}

	newTenant := &Tenant{
		ID:        id,
		Name:      name,
		Slug:      slug,
		Domain:    domain,
		Status:    status,
		Settings:  t.Settings,
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.tenants[id] = newTenant
	s.bySlug[slug] = id
	if domain != "" {
		s.byDom[domain] = id
	}

	return newTenant, nil
}

// Get finds a tenant by ID.
func (s *Service) Get(id string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, exists := s.tenants[id]
	if !exists {
		return nil, fmt.Errorf("tenancy: tenant with ID %q not found", id)
	}
	return t, nil
}

// GetBySlug finds a tenant by slug.
func (s *Service) GetBySlug(slug string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, exists := s.bySlug[strings.ToLower(slug)]
	if !exists {
		return nil, fmt.Errorf("tenancy: tenant with slug %q not found", slug)
	}
	return s.tenants[id], nil
}

// GetByDomain finds a tenant by custom host/domain.
func (s *Service) GetByDomain(domain string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, exists := s.byDom[strings.ToLower(domain)]
	if !exists {
		return nil, fmt.Errorf("tenancy: tenant with domain %q not found", domain)
	}
	return s.tenants[id], nil
}

// List returns all registered tenants.
func (s *Service) List() []*Tenant {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]*Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		res = append(res, t)
	}
	return res
}

// Update updates tenant properties.
func (s *Service) Update(ctx context.Context, id string, updates Tenant) (*Tenant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.tenants[id]
	if !exists {
		return nil, fmt.Errorf("tenancy: tenant %q not found", id)
	}

	if updates.Name != "" {
		existing.Name = updates.Name
	}
	if updates.Status != "" {
		existing.Status = updates.Status
	}
	if updates.Settings != nil {
		existing.Settings = updates.Settings
	}
	existing.UpdatedAt = time.Now().UTC()

	return existing, nil
}

// Delete removes a non-default tenant.
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id == s.defID {
		return errors.New("tenancy: cannot delete the default tenant")
	}

	t, exists := s.tenants[id]
	if !exists {
		return fmt.Errorf("tenancy: tenant %q not found", id)
	}

	delete(s.bySlug, t.Slug)
	if t.Domain != "" {
		delete(s.byDom, t.Domain)
	}
	delete(s.tenants, id)

	return nil
}
