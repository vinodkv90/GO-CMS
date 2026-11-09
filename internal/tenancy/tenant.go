package tenancy

import (
	"context"
	"time"
)

type contextKey string

const (
	TenantContextKey contextKey = "cms_tenant"
)

// TenantStatus represents the operational status of a tenant.
type TenantStatus string

const (
	StatusActive    TenantStatus = "active"
	StatusSuspended TenantStatus = "suspended"
	StatusInactive  TenantStatus = "inactive"
)

// Tenant represents an isolated workspace / organization in multi-tenant mode.
type Tenant struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Slug      string         `json:"slug"`
	Domain    string         `json:"domain,omitempty"`
	Status    TenantStatus   `json:"status"`
	Settings  map[string]any `json:"settings,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// GetTenant retrieves the active Tenant from context if present.
func GetTenant(ctx context.Context) *Tenant {
	if ctx == nil {
		return nil
	}
	if t, ok := ctx.Value(TenantContextKey).(*Tenant); ok {
		return t
	}
	return nil
}

// WithTenant attaches a Tenant to context.
func WithTenant(ctx context.Context, tenant *Tenant) context.Context {
	return context.WithValue(ctx, TenantContextKey, tenant)
}

