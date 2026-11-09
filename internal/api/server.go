package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/cache"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/jobs"
	"github.com/go-cms/cms/internal/localization"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/metrics"
	"github.com/go-cms/cms/internal/plugins"
	"github.com/go-cms/cms/internal/search"
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
	cache           cache.Cache
	workerPool      *jobs.WorkerPool
	locSvc          *localization.Service
	searchEngine    *search.Engine
	metricsRegistry *metrics.Registry
	auditSvc        *audit.Service
	tenancySvc      *tenancy.Service
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
	r.Use(middleware.RealIP)
	r.Use(StructuredLoggerMiddleware(logger))
	r.Use(middleware.Recoverer)
	r.Use(SecurityHeadersMiddleware)

	// 2. Metrics & Observability Middleware
	metricsRegistry := metrics.NewRegistry()
	r.Use(metricsRegistry.HTTPMiddleware)

	// 3. Timeout Middleware
	r.Use(middleware.Timeout(60 * time.Second))

	// 4. CORS Configuration
	corsOrigins := cfg.Server.CORSAllowedOrigins
	if len(corsOrigins) == 0 {
		corsOrigins = []string{"*"}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Tenant-ID"},
		ExposedHeaders:   []string{"Link", "X-Total-Count"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 5. Multi-Tenancy Middleware
	tenancySvc := tenancy.NewService()
	r.Use(tenancy.Middleware(tenancySvc))

	// 6. Auth Context Middleware
	if authSvc != nil {
		r.Use(auth.AuthMiddleware(authSvc))
	}

	// 7. Initialize Production Modules (Cache, Jobs, Localization, Search, Audit)
	cacheInst, err := cache.New(cfg.Cache)
	if err != nil {
		logger.Warn("Failed to initialize configured cache, falling back to in-memory", "error", err)
		cacheInst = cache.NewMemoryCache(cfg.Cache.Prefix)
	}

	jobQueue := jobs.NewQueue()
	workerPool := jobs.NewWorkerPool(jobQueue, cfg.Jobs.Concurrency, cfg.Jobs.PollInterval, logger)

	locSvc := localization.NewService()

	searchEngine := search.NewEngine()
	searchIndexer := search.NewIndexer(searchEngine)

	auditSvc := audit.NewService(nil)

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
		api.Get("/ping", PingHandler)
		api.Get("/openapi.yaml", OpenAPIHandler)

		if authSvc != nil {
			authHandler := auth.NewHandler(authSvc)
			authHandler.RegisterRoutes(api)
		}

		if contentSvc != nil {
			contentHandler := content.NewHandler(contentSvc)
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
		cache:           cacheInst,
		workerPool:      workerPool,
		locSvc:          locSvc,
		searchEngine:    searchEngine,
		metricsRegistry: metricsRegistry,
		auditSvc:        auditSvc,
		tenancySvc:      tenancySvc,
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

