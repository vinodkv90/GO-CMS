package media

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/storage"
)

// Handler handles media upload, retrieval, and file serving HTTP requests.
type Handler struct {
	svc *Service
	cfg *config.Config
}

// NewHandler creates a new media HTTP handler.
func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{
		svc: svc,
		cfg: cfg,
	}
}

// RegisterRoutes registers the media REST API endpoints.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/media", func(mediaRouter chi.Router) {
		// All media management endpoints require authentication
		mediaRouter.Use(auth.RequireAuth)

		mediaRouter.With(auth.RequirePermission("create", "media")).Post("/", h.Upload)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/", h.List)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/{id}", h.Get)
		mediaRouter.With(auth.RequirePermission("update", "media")).Patch("/{id}", h.Update)
		mediaRouter.With(auth.RequirePermission("delete", "media")).Delete("/{id}", h.Delete)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/{id}/signed-url", h.GetSignedURL)
	})
}

// MountFileServer mounts the static file server route (e.g. /uploads/*) onto the Chi router.
func (h *Handler) MountFileServer(r chi.Router) {
	r.Get("/uploads/*", h.ServeFile)
}

// Upload handles multipart/form-data file uploads.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	// 32MB max memory buffer for multipart form
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Form field 'file' is required")
		return
	}
	defer file.Close()

	altText := r.FormValue("alt_text")
	caption := r.FormValue("caption")

	asset, err := h.svc.Upload(r.Context(), file, header.Filename, header.Size, altText, caption)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]any{
		"data": asset,
	})
}

// List queries media assets with pagination and filters.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))

	filter := MediaFilter{
		Search:   q.Get("search"),
		MIMEType: q.Get("mime_type"),
		Limit:    limit,
		Offset:   offset,
	}

	assets, total, err := h.svc.ListAssets(r.Context(), filter)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": assets,
		"meta": map[string]any{
			"total":  total,
			"limit":  filter.Limit,
			"offset": filter.Offset,
		},
	})
}

// Get retrieves details for a single media asset.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	asset, err := h.svc.GetAsset(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			h.writeError(w, http.StatusNotFound, "Media asset not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": asset,
	})
}

// Update modifies metadata (alt text, caption).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input struct {
		AltText string `json:"alt_text"`
		Caption string `json:"caption"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	asset, err := h.svc.UpdateAsset(r.Context(), id, input.AltText, input.Caption)
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			h.writeError(w, http.StatusNotFound, "Media asset not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": asset,
	})
}

// Delete removes an asset from storage and database.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.DeleteAsset(r.Context(), id); err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			h.writeError(w, http.StatusNotFound, "Media asset not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"message": "Media asset deleted successfully",
	})
}

// GetSignedURL generates a temporary HMAC signed access URL.
func (h *Handler) GetSignedURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ttlSeconds, _ := strconv.Atoi(r.URL.Query().Get("ttl"))
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}

	signedURL, err := h.svc.GenerateSignedURL(r.Context(), id, ttl)
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			h.writeError(w, http.StatusNotFound, "Media asset not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"signed_url": signedURL,
		"expires_in": int(ttl.Seconds()),
	})
}

// ServeFile serves the uploaded media asset, enforcing HMAC signature verification if enabled.
func (h *Handler) ServeFile(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	key = strings.TrimPrefix(key, "/")

	// If signed URLs are enabled, verify HMAC signature
	if h.cfg != nil && h.cfg.Media.SignedURLs.Enabled && h.svc.Signer() != nil {
		if err := h.svc.Signer().VerifyURL(r.URL); err != nil {
			if errors.Is(err, storage.ErrSignatureExpired) {
				http.Error(w, "Access Denied: Signed URL has expired", http.StatusForbidden)
				return
			}
			http.Error(w, "Access Denied: Invalid or missing HMAC signature", http.StatusForbidden)
			return
		}
	}

	// Open file from storage
	reader, err := h.svc.Storage().Open(r.Context(), key)
	if err != nil {
		if errors.Is(err, storage.ErrFileNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Error retrieving file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Sniff MIME type from content
	mimeType, headerBytes, err := SniffMIME(reader)
	if err != nil {
		mimeType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // 24 hour cache
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Write sniffed header bytes then remaining stream
	_, _ = w.Write(headerBytes)
	_, _ = io.Copy(w, reader)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{
		"error": message,
	})
}
