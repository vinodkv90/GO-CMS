package tenancy

import (
	"net/http"
	"strings"
)

// Middleware intercepts HTTP requests and resolves the active Tenant context.
func Middleware(svc *Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tenant *Tenant

			// 1. Check X-Tenant-ID header (ID or slug)
			tenantHeader := r.Header.Get("X-Tenant-ID")
			if tenantHeader != "" {
				if t, err := svc.Get(tenantHeader); err == nil {
					tenant = t
				} else if t, err := svc.GetBySlug(tenantHeader); err == nil {
					tenant = t
				}
			}

			// 2. Check ?tenant= query parameter
			if tenant == nil {
				tenantParam := r.URL.Query().Get("tenant")
				if tenantParam != "" {
					if t, err := svc.Get(tenantParam); err == nil {
						tenant = t
					} else if t, err := svc.GetBySlug(tenantParam); err == nil {
						tenant = t
					}
				}
			}

			// 3. Check Host domain
			if tenant == nil {
				host := r.Host
				if colonIdx := strings.IndexByte(host, ':'); colonIdx != -1 {
					host = host[:colonIdx]
				}
				if t, err := svc.GetByDomain(host); err == nil {
					tenant = t
				}
			}

			// 4. Fallback to default tenant
			if tenant == nil {
				tenant = svc.DefaultTenant()
			}

			// 5. Guard against suspended tenant access
			if tenant != nil && tenant.Status == StatusSuspended {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"tenant workspace is suspended"}`))
				return
			}

			ctx := WithTenant(r.Context(), tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
