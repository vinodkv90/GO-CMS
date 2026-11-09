package tenancy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_TenantLifecycle(t *testing.T) {
	svc := NewService()
	ctx := context.Background()

	// 1. Initial default tenant
	def := svc.DefaultTenant()
	require.NotNil(t, def)
	assert.Equal(t, "default", def.Slug)
	assert.Equal(t, StatusActive, def.Status)

	// 2. Create new tenant
	tenant1, err := svc.Create(ctx, Tenant{
		Name:   "Alpha Org",
		Slug:   "alpha",
		Domain: "alpha.cms.local",
	})
	require.NoError(t, err)
	assert.Equal(t, "alpha", tenant1.Slug)
	assert.NotEmpty(t, tenant1.ID)

	// 3. Duplicate slug fails
	_, err = svc.Create(ctx, Tenant{Name: "Alpha Two", Slug: "alpha"})
	assert.ErrorContains(t, err, "already taken")

	// 4. Duplicate domain fails
	_, err = svc.Create(ctx, Tenant{Name: "Other", Slug: "other", Domain: "alpha.cms.local"})
	assert.ErrorContains(t, err, "already in use")

	// 5. Lookups
	bySlug, err := svc.GetBySlug("alpha")
	require.NoError(t, err)
	assert.Equal(t, tenant1.ID, bySlug.ID)

	byDom, err := svc.GetByDomain("alpha.cms.local")
	require.NoError(t, err)
	assert.Equal(t, tenant1.ID, byDom.ID)

	// 6. Update
	updated, err := svc.Update(ctx, tenant1.ID, Tenant{Name: "Alpha Corporation"})
	require.NoError(t, err)
	assert.Equal(t, "Alpha Corporation", updated.Name)

	// 7. Delete default tenant fails
	err = svc.Delete(ctx, def.ID)
	assert.ErrorContains(t, err, "cannot delete the default tenant")

	// 8. Delete non-default tenant succeeds
	err = svc.Delete(ctx, tenant1.ID)
	require.NoError(t, err)
	_, err = svc.Get(tenant1.ID)
	assert.Error(t, err)
}

func TestMiddleware_ContextResolution(t *testing.T) {
	svc := NewService()
	ctx := context.Background()

	tenantBeta, err := svc.Create(ctx, Tenant{
		Name:   "Beta Team",
		Slug:   "beta",
		Domain: "beta.example.com",
	})
	require.NoError(t, err)

	mw := Middleware(svc)

	var resolvedTenant *Tenant
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolvedTenant = GetTenant(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Header X-Tenant-ID = "beta"
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("X-Tenant-ID", "beta")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	require.NotNil(t, resolvedTenant)
	assert.Equal(t, tenantBeta.ID, resolvedTenant.ID)

	// Case 2: Query param ?tenant=beta
	req2 := httptest.NewRequest("GET", "/test?tenant=beta", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	require.NotNil(t, resolvedTenant)
	assert.Equal(t, tenantBeta.ID, resolvedTenant.ID)

	// Case 3: Host header matches domain
	req3 := httptest.NewRequest("GET", "/test", nil)
	req3.Host = "beta.example.com:8080"
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)
	require.NotNil(t, resolvedTenant)
	assert.Equal(t, tenantBeta.ID, resolvedTenant.ID)

	// Case 4: No identification -> resolves to default tenant
	req4 := httptest.NewRequest("GET", "/test", nil)
	w4 := httptest.NewRecorder()
	handler.ServeHTTP(w4, req4)
	require.NotNil(t, resolvedTenant)
	assert.Equal(t, "default", resolvedTenant.Slug)
}

func TestHTTPHandler_Endpoints(t *testing.T) {
	svc := NewService()
	handler := NewHTTPHandler(svc)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			adminUser := &auth.User{
				ID:       "admin-1",
				Username: "admin",
				Status:   auth.StatusActive,
				Roles: []auth.Role{
					{
						Slug: auth.RoleSuperAdmin,
					},
				},
			}
			ctx := context.WithValue(req.Context(), auth.UserContextKey, adminUser)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	handler.RegisterRoutes(r)

	// 1. GET /tenants
	req := httptest.NewRequest("GET", "/tenants", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 2. POST /tenants
	payload := `{"name":"Gamma Corp","slug":"gamma","domain":"gamma.local"}`
	req = httptest.NewRequest("POST", "/tenants", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var createdResp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &createdResp)
	require.NoError(t, err)
	tenantData := createdResp["data"].(map[string]any)
	tenantID := tenantData["id"].(string)

	// 3. GET /tenants/{id}
	req = httptest.NewRequest("GET", "/tenants/"+tenantID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 4. PATCH /tenants/{id}
	updatePayload := `{"name":"Gamma Enterprise"}`
	req = httptest.NewRequest("PATCH", "/tenants/"+tenantID, bytes.NewBufferString(updatePayload))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 5. DELETE /tenants/{id}
	req = httptest.NewRequest("DELETE", "/tenants/"+tenantID, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

