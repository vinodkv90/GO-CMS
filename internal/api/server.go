package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/cache"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/database/migrations"
	"github.com/go-cms/cms/internal/email"
	"github.com/go-cms/cms/internal/usersperm"
	"github.com/go-cms/cms/internal/webhooks"
	"github.com/go-cms/cms/internal/jobs"
	"github.com/go-cms/cms/internal/localization"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/metrics"
	"github.com/go-cms/cms/internal/plugins"
	"github.com/go-cms/cms/internal/search"
	"github.com/go-cms/cms/internal/settings"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/templates"
	"github.com/go-cms/cms/internal/tenancy"
)

// Server wraps the HTTP router and http.Server instance.
type Server struct {
	cfg             *config.Config
	db              interfaces.Database
	authSvc         *auth.Service
	contentSvc      *content.Service
	mediaSvc        *media.Service
	pluginMgr       *plugins.Manager
	renderer        *templates.Renderer
	settingsSvc     *settings.Service
	emailSvc        *email.Service
	usersPermSvc    *usersperm.Service
	webhooksSvc     *webhooks.Service
	cache           cache.Cache
	workerPool      *jobs.WorkerPool
	locSvc          *localization.Service
	searchEngine    *search.Engine
	metricsRegistry *metrics.Registry
	auditSvc        *audit.Service
	tenancySvc      *tenancy.Service
	rateLimiter     *IPRateLimiter
	authLimiter     *IPRateLimiter
	dynamicCORS     *DynamicCORS
	logger          *slog.Logger
	router          *chi.Mux
	httpServer      *http.Server
}

// NewServer configures and initializes a new HTTP Server.
func NewServer(
	cfg *config.Config,
	db interfaces.Database,
	authSvc *auth.Service,
	contentSvc *content.Service,
	mediaSvc *media.Service,
	pluginMgr *plugins.Manager,
	renderer *templates.Renderer,
	logger *slog.Logger,
) *Server {
	r := chi.NewRouter()

	// 1. Basic Middlewares
	r.Use(middleware.RequestID)
	r.Use(TrustedRealIPMiddleware(cfg.Server.TrustedProxies))
	r.Use(StructuredLoggerMiddleware(logger))
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return SecurityHeadersMiddlewareForEnvironment(next, cfg.Server.Env == "production")
	})
	r.Use(MaxBodySizeMiddlewareByPath(32<<20, func(path string) int64 {
		if strings.HasSuffix(path, "/media/hls") {
			return media.MaxHLSRequestBodyBytes
		}
		if mediaSvc != nil && (strings.HasSuffix(path, "/media") || strings.HasSuffix(path, "/media/upload")) {
			return mediaSvc.MaxUploadRequestBodyBytes(context.Background())
		}
		return 32 << 20
	}))

	// Rate Limiting (50 req/sec global, 30 req/min auth endpoints)
	rateLimiter := NewIPRateLimiter(50.0, 100)
	authLimiter := NewIPRateLimiter(30.0/60.0, 20)
	r.Use(rateLimiter.Middleware)

	// 2. Metrics & Observability Middleware
	metricsRegistry := metrics.NewRegistry()
	r.Use(metricsRegistry.HTTPMiddleware)

	// 3. Timeout Middleware
	r.Use(middleware.Timeout(60 * time.Second))

	// 4. Dynamic CORS Configuration
	dynamicCORS := NewDynamicCORS(settings.DefaultCORSSettings(cfg.Server.CORSAllowedOrigins))
	r.Use(dynamicCORS.Middleware)

	// 5. Multi-Tenancy Middleware
	tenancySvc := tenancy.NewService()
	r.Use(tenancy.Middleware(tenancySvc))

	// 6. Auth Context Middleware
	if authSvc != nil {
		r.Use(auth.AuthMiddleware(authSvc))
		r.Use(auth.CSRFMiddleware)
	}

	// 7. Initialize Production Modules (Cache, Jobs, Localization, Search, Audit)
	cacheInst, err := cache.New(cfg.Cache)
	if err != nil {
		logger.Warn("Failed to initialize configured cache, falling back to in-memory", "error", err)
		cacheInst = cache.NewMemoryCache(cfg.Cache.Prefix)
	}

	jobQueue := jobs.NewQueue()
	workerPool := jobs.NewWorkerPool(jobQueue, cfg.Jobs.Concurrency, cfg.Jobs.PollInterval, logger)

	var locRepo localization.Repository = localization.NewMemoryRepository()
	if sqlDB, ok := db.(interfaces.SQLDatabase); ok && db != nil {
		locRepo = localization.NewSQLRepository(sqlDB)
	}
	locSvc := localization.NewServiceWithRepo(locRepo)

	searchEngine := search.NewEngine()
	searchIndexer := search.NewIndexer(searchEngine)

	var auditRepo audit.Repository
	if sqlDB, ok := db.(interfaces.SQLDatabase); ok && db != nil {
		auditRepo = audit.NewSQLRepository(sqlDB)
	}
	auditSvc := audit.NewService(auditRepo)
	if authSvc != nil {
		authSvc.SetAuditRecorder(func(ctx context.Context, meta auth.AuditMeta, action, resourceType, resourceID string, details map[string]any, result string) {
			actorType := "admin"
			if meta.ActorID == "" {
				actorType = "system"
			}
			_, _ = auditSvc.Record(ctx, audit.RecordParams{
				ActorID:   meta.ActorID,
				ActorType: actorType,
				Action:    action,
				Resource:  resourceType,
				TargetID:  resourceID,
				Metadata:  details,
				IPAddress: meta.IP,
				UserAgent: meta.UserAgent,
				Result:    result,
			})
		})
	}
	if pluginMgr != nil {
		pluginMgr.SetAuditRecorder(func(ctx context.Context, action, resourceType, resourceID string, metadata map[string]any) {
			_, _ = auditSvc.Record(ctx, audit.RecordParams{
				ActorID:   "system",
				ActorType: "plugin",
				Action:    action,
				Resource:  resourceType,
				TargetID:  resourceID,
				Metadata:  metadata,
				Result:    "success",
			})
		})
	}

	var settingsRepo settings.Repository = settings.NewMemoryRepository()
	if sqlDB, ok := db.(interfaces.SQLDatabase); ok && db != nil {
		settingsRepo = settings.NewSQLRepository(sqlDB)
	}
	settingsSvc := settings.NewService(settingsRepo, cfg, auditSvc, settings.RuntimeStatusProvider{
		CMSVersion: CMSVersion,
		DatabaseStatus: func(ctx context.Context) (string, string, string) {
			if db == nil {
				return "memory", "in-memory", ""
			}
			if err := db.Ping(ctx); err != nil {
				return db.DriverName(), "unhealthy", err.Error()
			}
			return db.DriverName(), "healthy", ""
		},
		MigrationsStatus: func(ctx context.Context) []settings.MigrationItem {
			if db == nil {
				return nil
			}
			runner := migrations.NewRunner(db)
			runner.Register(migrations.DefaultMigrations(db.DriverName())...)
			applied, pending, err := runner.Status(ctx)
			if err != nil {
				return nil
			}
			out := make([]settings.MigrationItem, 0, len(applied)+len(pending))
			for _, rec := range applied {
				out = append(out, settings.MigrationItem{
					Version:   rec.Version,
					Name:      rec.Name,
					Applied:   true,
					AppliedAt: rec.AppliedAt.UTC().Format(time.RFC3339),
				})
			}
			for _, mig := range pending {
				out = append(out, settings.MigrationItem{
					Version: mig.Version,
					Name:    mig.Name,
					Applied: false,
				})
			}
			return out
		},
		CacheStatus: func(context.Context) string {
			if cacheInst == nil {
				return "disabled"
			}
			return "healthy"
		},
		JobsStats: func() map[string]int {
			return jobQueue.Stats()
		},
		ThemesInfo: func() (string, []string) {
			if renderer == nil {
				return cfg.Themes.ActiveTheme, nil
			}
			list := renderer.ListThemes()
			active := cfg.Themes.ActiveTheme
			installed := make([]string, 0, len(list))
			for _, item := range list {
				if slug, ok := item["slug"].(string); ok {
					installed = append(installed, slug)
					if isAct, ok := item["is_active"].(bool); ok && isAct {
						active = slug
					}
				}
			}
			return active, installed
		},
		PluginsInfo: func() (int, int) {
			if pluginMgr == nil {
				return 0, 0
			}
			list := pluginMgr.List()
			enabled := 0
			for _, p := range list {
				if p.Status == plugins.StatusEnabled {
					enabled++
				}
			}
			return len(list), enabled
		},
	})
	if renderer != nil {
		renderer.SetSiteInfoProvider(func() templates.SiteInfo {
			pub := settingsSvc.GetPublicGeneral(context.Background())
			return templates.SiteInfo{
				Title:    pub.SiteName,
				Tagline:  pub.SiteDescription,
				BaseURL:  pub.PublicURL,
				Language: pub.DefaultLocale,
			}
		})
	}

	if corsCfg, err := settingsSvc.GetCORSSettings(context.Background()); err == nil && corsCfg != nil {
		dynamicCORS.Update(*corsCfg)
	}
	if rlCfg, err := settingsSvc.GetRateLimitSettings(context.Background()); err == nil && rlCfg != nil {
		rateLimiter.UpdateConfig(rlCfg.Enabled, rlCfg.GlobalRequestsPerSec, rlCfg.GlobalBurst)
		authLimiter.UpdateConfig(rlCfg.Enabled, float64(rlCfg.AuthRequestsPerMin)/60.0, rlCfg.AuthBurst)
	}
	settingsSvc.OnCORSUpdated(func(c settings.CORSSettings) {
		dynamicCORS.Update(c)
	})
	settingsSvc.OnRateLimitUpdated(func(rl settings.RateLimitSettings) {
		rateLimiter.UpdateConfig(rl.Enabled, rl.GlobalRequestsPerSec, rl.GlobalBurst)
		authLimiter.UpdateConfig(rl.Enabled, float64(rl.AuthRequestsPerMin)/60.0, rl.AuthBurst)
	})

	if mediaSvc != nil {
		mediaSvc.SetMediaSettingsProvider(func(ctx context.Context) (media.RuntimeConfig, error) {
			ms, err := settingsSvc.GetMediaSettings(ctx)
			if err != nil {
				return media.RuntimeConfig{}, err
			}
			return media.RuntimeConfig{
				MaxFileSizeBytes:       ms.MaxFileSizeBytes,
				AllowedMIMETypes:       ms.AllowedMIMETypes,
				ResponsiveSizesEnabled: ms.ResponsiveSizesEnabled,
				GenerateSmall:          ms.GenerateSmall,
				GenerateMedium:         ms.GenerateMedium,
				GenerateLarge:          ms.GenerateLarge,
				RequireAltText:         ms.RequireAltText,
				DefaultFolderID:        ms.DefaultFolderID,
				DefaultAccess:          ms.DefaultAccess,
				SignedURLTTLSeconds:    ms.SignedURLTTLSeconds,
			}, nil
		})
		if swappable, ok := mediaSvc.Storage().(*storage.SwappableProvider); ok {
			applyStorageRuntime := func(stCfg settings.StorageSettings, sec settings.StorageSecrets) {
				for _, configured := range settings.BuildConfiguredProviders(stCfg, sec) {
					swappable.RegisterProvider(configured)
				}
				if p, err := settings.BuildStorageProviderWithSecrets(stCfg, sec); err == nil {
					swappable.Swap(p)
					logger.Info("Active media storage provider configured", "provider", p.Name())
				} else {
					logger.Warn("Failed to apply persisted storage settings; keeping fallback provider", "error", err)
				}
			}

			if stCfg, secrets, err := settingsSvc.ResolveStorageWithSecrets(context.Background()); err == nil {
				applyStorageRuntime(stCfg, secrets)
			}
			settingsSvc.OnStorageUpdatedWithSecrets(func(st settings.StorageSettings, secrets settings.StorageSecrets) {
				for _, configured := range settings.BuildConfiguredProviders(st, secrets) {
					swappable.RegisterProvider(configured)
				}
				p, err := settings.BuildStorageProviderWithSecrets(st, secrets)
				if err != nil {
					logger.Error("Failed to hot-swap storage provider", "provider", st.ActiveProvider, "error", err)
					return
				}
				swappable.Swap(p)
				logger.Info("Active media storage provider hot-swapped", "provider", p.Name())
			})
		}
	}

	var emailRepo email.TemplateRepository = email.NewMemoryTemplateRepository()
	if sqlDB, ok := db.(interfaces.SQLDatabase); ok && db != nil {
		emailRepo = email.NewSQLTemplateRepository(sqlDB)
	}
	emailSvc := email.NewService(settingsSvc, emailRepo, logger)

	if authSvc != nil {
		authSvc.SetSecurityPolicyProvider(func(ctx context.Context) auth.SecurityRuntimeConfig {
			out := auth.DefaultSecurityRuntimeConfig()
			if sec, err := settingsSvc.GetSecuritySettings(ctx); err == nil && sec != nil {
				out.SessionTimeoutMinutes = sec.SessionTimeoutMinutes
				out.MaxFailedLogins = sec.MaxFailedLogins
				out.LockoutDurationMinutes = sec.LockoutDurationMinutes
				out.EnforceAdminMFA = sec.EnforceAdminMFA
				out.AllowedAdminIPCIDRs = append([]string{}, sec.AllowedAdminIPCIDRs...)
			}
			if pp, err := settingsSvc.GetPasswordPolicySettings(ctx); err == nil && pp != nil {
				out.PasswordPolicy = auth.PasswordPolicy{
					MinLength:        pp.MinLength,
					RequireUppercase: pp.RequireUppercase,
					RequireLowercase: pp.RequireLowercase,
					RequireNumber:    pp.RequireNumber,
					RequireSpecial:   pp.RequireSpecial,
					MaxAgeDays:       pp.MaxAgeDays,
				}
			}
			if sso, err := settingsSvc.GetSSOSettings(ctx); err == nil && sso != nil {
				out.SSOEnabled = sso.Enabled
				if sso.EnforceSSOForDomains {
					out.SSOEnforcedDomains = append([]string{}, sso.AllowedDomains...)
				}
			}
			return out
		})
		authSvc.SetInviteNotifier(func(ctx context.Context, inv *auth.AdminInvitation, rawToken string, actor *auth.User, expiresInHours int) error {
			inviter := ""
			if actor != nil {
				inviter = strings.TrimSpace(actor.FirstName + " " + actor.LastName)
				if inviter == "" {
					inviter = actor.Username
				}
				if inviter == "" {
					inviter = actor.Email
				}
			}
			return emailSvc.SendAdminInvitation(ctx, inv.Email, rawToken, inviter, expiresInHours)
		})
	}

	var upRepo usersperm.Repository = usersperm.NewMemoryRepository()
	if sqlDB, ok := db.(interfaces.SQLDatabase); ok && db != nil {
		upRepo = usersperm.NewSQLRepository(sqlDB)
	}
	usersPermSvc := usersperm.NewService(upRepo, settingsSvc, auditSvc, cfg, logger)
	if err := usersPermSvc.SeedDefaults(context.Background()); err != nil {
		logger.Warn("Failed to seed users-permissions defaults", "error", err)
	}
	usersPermSvc.SetEmailSender(emailSvc.SendTemplate)
	if authSvc != nil {
		usersPermSvc.SetAdminSSOLoginFunc(authSvc.CompleteSSOLogin)
	}

	locSvc.SetInUseChecker(func(ctx context.Context, code string) (bool, error) {
		if gen, err := settingsSvc.GetGeneral(ctx); err == nil && strings.EqualFold(gen.DefaultLocale, code) {
			return true, nil
		}
		sqlDB, ok := db.(interfaces.SQLDatabase)
		if !ok || db == nil {
			return false, nil
		}
		pattern := `%"` + code + `":%`
		query := `SELECT draft_data, published_data FROM content_entries WHERE deleted_at IS NULL AND (CAST(draft_data AS TEXT) LIKE $1 OR CAST(published_data AS TEXT) LIKE $1) LIMIT 50`
		if sqlDB.DriverName() != "postgres" {
			query = `SELECT draft_data, published_data FROM content_entries WHERE deleted_at IS NULL AND (CAST(draft_data AS CHAR) LIKE ? OR CAST(published_data AS CHAR) LIKE ?) LIMIT 50`
		}
		var rows *sql.Rows
		var err error
		if sqlDB.DriverName() == "postgres" {
			rows, err = sqlDB.QueryContext(ctx, query, pattern)
		} else {
			rows, err = sqlDB.QueryContext(ctx, query, pattern, pattern)
		}
		if err != nil {
			return false, nil
		}
		defer rows.Close()
		for rows.Next() {
			var draftRaw, pubRaw sql.NullString
			if err := rows.Scan(&draftRaw, &pubRaw); err != nil {
				continue
			}
			for _, raw := range []sql.NullString{draftRaw, pubRaw} {
				if !raw.Valid || strings.TrimSpace(raw.String) == "" {
					continue
				}
				var m map[string]any
				if err := json.Unmarshal([]byte(raw.String), &m); err == nil && jsonContainsLocaleKey(m, code) {
					return true, nil
				}
			}
		}
		return false, nil
	})

	var whRepo webhooks.Repository = webhooks.NewMemoryRepository()
	if sqlDB, ok := db.(interfaces.SQLDatabase); ok && db != nil {
		whRepo = webhooks.NewSQLRepository(sqlDB)
	}
	webhooksSvc := webhooks.NewService(whRepo, settingsSvc.EncryptionKey, auditSvc, logger)
	if contentSvc != nil && contentSvc.EventBus() != nil {
		webhooksSvc.RegisterContentBus(contentSvc.EventBus())
	}

	// If content service is active, attach lifecycle hooks for cache invalidation & search indexing
	if contentSvc != nil && contentSvc.EventBus() != nil {
		bus := contentSvc.EventBus()
		searchIndexer.AttachLifecycleHooks(bus)

		// Cache invalidation hooks
		bus.Subscribe(content.EventAfterUpdate, func(ctx context.Context, event content.LifecycleEvent, entry *content.ContentEntry) error {
			if entry != nil {
				_ = cacheInst.DeleteByTag(ctx, "content:"+entry.ContentTypeID)
				_ = cacheInst.DeleteByTag(ctx, "content")
			}
			return nil
		})
		bus.Subscribe(content.EventAfterPublish, func(ctx context.Context, event content.LifecycleEvent, entry *content.ContentEntry) error {
			if entry != nil {
				_ = cacheInst.DeleteByTag(ctx, "content:"+entry.ContentTypeID)
				_ = cacheInst.DeleteByTag(ctx, "content")
			}
			return nil
		})
		bus.Subscribe(content.EventAfterDelete, func(ctx context.Context, event content.LifecycleEvent, entry *content.ContentEntry) error {
			if entry != nil {
				_ = cacheInst.DeleteByTag(ctx, "content:"+entry.ContentTypeID)
				_ = cacheInst.DeleteByTag(ctx, "content")
			}
			return nil
		})
	}

	// 6. Mount Metrics, Documentation, Health, Info, and API routes
	r.Handle("/metrics", metricsRegistry.Handler())
	r.Get("/docs", DocsHandler)
	r.Get("/healthz", HealthHandler)
	r.Get("/readyz", ReadinessHandler(db))
	r.Get("/info", InfoHandler(cfg, db))

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					p := strings.TrimSuffix(r.URL.Path, "/")
					if strings.HasSuffix(p, "/auth/login") ||
						strings.HasSuffix(p, "/auth/register") ||
						strings.HasSuffix(p, "/auth/invitations/accept") ||
						strings.HasSuffix(p, "/auth/mfa/verify") ||
						strings.HasSuffix(p, "/auth/reset-expired-password") ||
						strings.HasSuffix(p, "/app-auth/login") ||
						strings.HasSuffix(p, "/app-auth/register") ||
						strings.HasSuffix(p, "/app-auth/forgot-password") ||
						strings.HasSuffix(p, "/app-auth/reset-password") {
						if !authLimiter.Allow(extractClientIP(r)) {
							w.Header().Set("Content-Type", "application/json; charset=utf-8")
							w.Header().Set("Retry-After", "60")
							w.WriteHeader(http.StatusTooManyRequests)
							_, _ = w.Write([]byte(`{"error":"too many authentication attempts, please slow down"}`))
							return
						}
					}
				}
				next.ServeHTTP(w, r)
			})
		})

		api.Get("/ping", PingHandler)
		api.Get("/openapi.yaml", OpenAPIHandler)
		if renderer != nil {
			api.Get("/themes", func(w http.ResponseWriter, r *http.Request) {
				respondJSON(w, http.StatusOK, map[string]any{"data": renderer.ListThemes()})
			})
		}

		if authSvc != nil {
			authHandler := auth.NewHandler(authSvc)
			authHandler.RegisterRoutes(api)
		}

		if contentSvc != nil {
			contentHandler := content.NewHandler(contentSvc)
			contentHandler.SetRestAPISettingsProvider(func(ctx context.Context) content.RestAPIRuntimeConfig {
				cfg, err := settingsSvc.GetRestAPISettings(ctx)
				if err != nil || cfg == nil {
					def := settings.DefaultRestAPISettings()
					cfg = &def
				}
				return content.RestAPIRuntimeConfig{
					DefaultLimit:       cfg.DefaultLimit,
					MaxLimit:           cfg.MaxLimit,
					WithCount:          cfg.WithCount,
					MaxPopulateDepth:   cfg.MaxPopulateDepth,
					ExposeDraftPreview: cfg.ExposeDraftPreview,
				}
			})
			contentHandler.RegisterRoutes(api)
		}

		if mediaSvc != nil {
			mediaHandler := media.NewHandler(mediaSvc, cfg)
			mediaHandler.RegisterRoutes(api)
		}

		if pluginMgr != nil {
			pluginHandler := plugins.NewHandler(pluginMgr)
			pluginHandler.RegisterRoutes(api)
		}

		// Background Jobs Management
		jobsHandler := jobs.NewHTTPHandler(workerPool)
		jobsHandler.RegisterRoutes(api)

		// Content Localization
		locHandler := localization.NewHTTPHandler(locSvc)
		locHandler.RegisterRoutes(api)

		// Full-Text Search
		searchHandler := search.NewHTTPHandler(searchEngine)
		searchHandler.RegisterRoutes(api)

		// System Audit Logging
		auditHandler := audit.NewHTTPHandler(auditSvc)
		auditHandler.RegisterRoutes(api)

		// Multi-Tenancy Management
		tenancyHandler := tenancy.NewHTTPHandler(tenancySvc)
		tenancyHandler.RegisterRoutes(api)

		// Settings & Preferences
		settingsHandler := settings.NewHTTPHandler(settingsSvc)
		settingsHandler.RegisterPublicRoutes(api)
		settingsHandler.RegisterProtectedRoutes(api)

		// Outbound Email & Templates
		emailHandler := email.NewHTTPHandler(emailSvc)
		emailHandler.RegisterProtectedRoutes(api)

		// Isolated Application Users & Permissions + Admin SSO
		usersPermHandler := usersperm.NewHandler(usersPermSvc)
		usersPermHandler.RegisterRoutes(api)

		// Outgoing Webhooks & Delivery Logs
		webhooksHandler := webhooks.NewHandler(webhooksSvc)
		webhooksHandler.RegisterRoutes(api)
	})

	// 7. Mount Static Uploaded Files
	if mediaSvc != nil {
		mediaHandler := media.NewHandler(mediaSvc, cfg)
		mediaHandler.MountFileServer(r)
	}

	// 8. Mount Admin SPA Console (if built)
	mountAdminSPA(r)

	// 9. Mount Traditional Web CMS Routes (Renderer)
	if renderer != nil {
		renderer.MountWebRoutes(r)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	return &Server{
		cfg:             cfg,
		db:              db,
		authSvc:         authSvc,
		contentSvc:      contentSvc,
		mediaSvc:        mediaSvc,
		pluginMgr:       pluginMgr,
		renderer:        renderer,
		settingsSvc:     settingsSvc,
		emailSvc:        emailSvc,
		usersPermSvc:    usersPermSvc,
		webhooksSvc:     webhooksSvc,
		cache:           cacheInst,
		workerPool:      workerPool,
		locSvc:          locSvc,
		searchEngine:    searchEngine,
		metricsRegistry: metricsRegistry,
		auditSvc:        auditSvc,
		tenancySvc:      tenancySvc,
		rateLimiter:     rateLimiter,
		authLimiter:     authLimiter,
		dynamicCORS:     dynamicCORS,
		logger:          logger,
		router:          r,
		httpServer:      httpSrv,
	}
}

// Router returns the underlying Chi router for mounting additional route modules.
func (s *Server) Router() *chi.Mux {
	return s.router
}

// Start begins listening and serving HTTP traffic.
func (s *Server) Start() error {
	s.logger.Info("Starting CMS HTTP server",
		"address", s.httpServer.Addr,
		"env", s.cfg.Server.Env,
		"db_driver", s.cfg.Database.Driver,
	)

	// Start background worker pool if enabled
	if s.cfg.Jobs.Enabled && s.workerPool != nil {
		s.workerPool.Start()
	}

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server failure: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server within the provided context timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down CMS HTTP server...")

	if s.rateLimiter != nil {
		s.rateLimiter.Close()
	}
	if s.authLimiter != nil {
		s.authLimiter.Close()
	}
	if s.workerPool != nil {
		_ = s.workerPool.Stop(ctx)
	}
	if s.cache != nil {
		_ = s.cache.Close()
	}

	return s.httpServer.Shutdown(ctx)
}

// Cache returns the active Cache provider.
func (s *Server) Cache() cache.Cache {
	return s.cache
}

// Jobs returns the background worker pool instance.
func (s *Server) Jobs() *jobs.WorkerPool {
	return s.workerPool
}

// Localization returns the active content localization service.
func (s *Server) Localization() *localization.Service {
	return s.locSvc
}

// Webhooks returns the active outgoing webhook service.
func (s *Server) Webhooks() *webhooks.Service {
	return s.webhooksSvc
}

func jsonContainsLocaleKey(v any, code string) bool {
	switch typed := v.(type) {
	case map[string]any:
		for k, val := range typed {
			if strings.EqualFold(k, code) && len(k) >= 2 && len(k) <= 5 {
				return true
			}
			if jsonContainsLocaleKey(val, code) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if jsonContainsLocaleKey(item, code) {
				return true
			}
		}
	}
	return false
}

// Search returns the full-text search engine instance.
func (s *Server) Search() *search.Engine {
	return s.searchEngine
}

// Metrics returns the Prometheus metrics registry.
func (s *Server) Metrics() *metrics.Registry {
	return s.metricsRegistry
}

// Audit returns the system audit logging service.
func (s *Server) Audit() *audit.Service {
	return s.auditSvc
}

// Tenancy returns the multi-tenant service.
func (s *Server) Tenancy() *tenancy.Service {
	return s.tenancySvc
}

// findAdminDist locates the compiled React Admin SPA directory.
func findAdminDist() string {
	candidates := []string{
		filepath.Join("admin", "dist"),
		filepath.Join("..", "admin", "dist"),
		filepath.Join("..", "..", "admin", "dist"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return ""
}

// mountAdminSPA mounts the static built React Admin SPA if admin/dist exists.
func mountAdminSPA(r chi.Router) {
	distPath := findAdminDist()
	if distPath == "" {
		return
	}

	fs := http.StripPrefix("/admin", http.FileServer(http.Dir(distPath)))

	r.Get("/admin", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/admin/", http.StatusMovedPermanently)
	})

	r.Get("/admin/*", func(w http.ResponseWriter, req *http.Request) {
		subpath := chi.URLParam(req, "*")
		if subpath == "" || subpath == "/" {
			http.ServeFile(w, req, filepath.Join(distPath, "index.html"))
			return
		}

		cleanPath := strings.TrimPrefix(filepath.Clean(subpath), string(filepath.Separator))
		targetFile := filepath.Join(distPath, cleanPath)
		if info, err := os.Stat(targetFile); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, req)
			return
		}

		// Client-side SPA routing fallback
		http.ServeFile(w, req, filepath.Join(distPath, "index.html"))
	})
}
