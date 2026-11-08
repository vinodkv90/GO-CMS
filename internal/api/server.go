package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/templates"
)

// Server wraps the HTTP router and http.Server instance.
type Server struct {
	cfg        *config.Config
	db         interfaces.Database
	authSvc    *auth.Service
	contentSvc *content.Service
	mediaSvc   *media.Service
	renderer   *templates.Renderer
	logger     *slog.Logger
	router     *chi.Mux
	httpServer *http.Server
}

// NewServer configures and initializes a new HTTP Server.
func NewServer(
	cfg *config.Config,
	db interfaces.Database,
	authSvc *auth.Service,
	contentSvc *content.Service,
	mediaSvc *media.Service,
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

	// 2. Timeout Middleware
	r.Use(middleware.Timeout(60 * time.Second))

	// 3. CORS Configuration
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

	// 4. Auth Context Middleware
	if authSvc != nil {
		r.Use(auth.AuthMiddleware(authSvc))
	}

	// 5. Mount Health, Info, and API routes
	r.Get("/healthz", HealthHandler)
	r.Get("/readyz", ReadinessHandler(db))
	r.Get("/info", InfoHandler(cfg, db))

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/ping", PingHandler)

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
	})

	// 6. Mount Static Uploaded Files
	if mediaSvc != nil {
		mediaHandler := media.NewHandler(mediaSvc, cfg)
		mediaHandler.MountFileServer(r)
	}

	// 7. Mount Traditional Web CMS Routes (Renderer)
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
		cfg:        cfg,
		db:         db,
		authSvc:    authSvc,
		contentSvc: contentSvc,
		mediaSvc:   mediaSvc,
		renderer:   renderer,
		logger:     logger,
		router:     r,
		httpServer: httpSrv,
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

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server failure: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the HTTP server within the provided context timeout.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down CMS HTTP server...")
	return s.httpServer.Shutdown(ctx)
}
