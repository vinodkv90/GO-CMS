package media

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/tenancy"
)

// Handler handles media upload, retrieval, folder management, and file serving HTTP requests.
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

// RegisterRoutes registers all media and folder REST API endpoints.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/media", func(mediaRouter chi.Router) {
		// All media management endpoints require authentication
		mediaRouter.Use(auth.RequireAuth)

		// Folder management routes
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/tree", h.GetTree)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/folders", h.ListFolders)
		mediaRouter.With(auth.RequirePermission("create", "media")).Post("/folders", h.CreateFolder)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/folders/{id}", h.GetFolder)
		mediaRouter.With(auth.RequirePermission("update", "media")).Put("/folders/{id}", h.RenameFolder)
		mediaRouter.With(auth.RequirePermission("delete", "media")).Delete("/folders/{id}", h.DeleteFolder)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/folders/{id}/contents", h.GetFolderContents)
		mediaRouter.With(auth.RequirePermission("update", "media")).Post("/folders/{id}/move", h.MoveFolder)

		// Media assets routes
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/capabilities", h.GetCapabilities)
		mediaRouter.With(auth.RequirePermission("create", "media")).Post("/", h.Upload)
		mediaRouter.With(auth.RequirePermission("create", "media")).Post("/upload", h.Upload)
		mediaRouter.With(auth.RequirePermission("create", "media")).Post("/hls", h.UploadHLS)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/", h.List)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/{id}", h.Get)
		mediaRouter.With(auth.RequirePermission("update", "media")).Patch("/{id}", h.Update)
		mediaRouter.With(auth.RequirePermission("update", "media")).Post("/{id}/replace", h.ReplaceAsset)
		mediaRouter.With(auth.RequirePermission("update", "media")).Post("/{id}/move", h.MoveAsset)
		mediaRouter.With(auth.RequirePermission("delete", "media")).Delete("/{id}", h.Delete)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/{id}/signed-url", h.GetSignedURL)
		mediaRouter.With(auth.RequirePermission("read", "media")).Get("/{id}/transform", h.GetTransformedURL)
	})
}

// MountFileServer mounts the static file server route (e.g. /uploads/*) onto the Chi router.
func (h *Handler) MountFileServer(r chi.Router) {
	r.Get("/uploads/*", h.ServeFile)
}

// Upload handles multipart/form-data single file uploads.
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

	var folderID *string
	fVal := r.FormValue("folder_id")
	if fVal == "" {
		fVal = r.URL.Query().Get("folder_id")
	}
	if fVal != "" && fVal != "null" && fVal != "root" {
		folderID = &fVal
	}

	asset, err := h.svc.Upload(r.Context(), file, header.Filename, header.Size, altText, caption, folderID)
	if err != nil {
		if errors.Is(err, ErrMediaValidation) || errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]any{
		"data": asset,
	})
}

// UploadHLS handles multipart/form-data ZIP upload containing an HLS video stream package.
func (h *Handler) UploadHLS(w http.ResponseWriter, r *http.Request) {
	// 64MB max memory buffer
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		h.writeError(w, http.StatusBadRequest, "Failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		file, header, err = r.FormFile("hls_package")
	}
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Form field 'file' or 'hls_package' is required")
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
		h.writeError(w, http.StatusBadRequest, "HLS package must be a .zip archive")
		return
	}

	title := r.FormValue("title")
	var folderID *string
	fVal := r.FormValue("folder_id")
	if fVal == "" {
		fVal = r.URL.Query().Get("folder_id")
	}
	if fVal != "" && fVal != "null" && fVal != "root" {
		folderID = &fVal
	}

	asset, err := h.svc.UploadHLSPackage(r.Context(), file, header.Filename, header.Size, title, folderID)
	if err != nil {
		if errors.Is(err, ErrMediaValidation) || errors.Is(err, ErrInvalidHLSPackage) || errors.Is(err, ErrZipBombDetected) || errors.Is(err, ErrHLSMissingSegment) || errors.Is(err, ErrPathTraversal) {
			h.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
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

	var folderID *string
	if q.Has("folder_id") {
		fVal := q.Get("folder_id")
		if fVal == "root" || fVal == "" {
			empty := ""
			folderID = &empty
		} else {
			folderID = &fVal
		}
	}

	filter := MediaFilter{
		FolderID: folderID,
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

// MoveAsset relocates an asset to a target folder.
func (h *Handler) MoveAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var input struct {
		TargetFolderID *string `json:"target_folder_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	asset, err := h.svc.MoveAsset(r.Context(), id, input.TargetFolderID)
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) || errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
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
		ttl = h.svc.DefaultSignedURLTTL(r.Context())
	}
	if ttl > maxSignedURLTTL {
		h.writeError(w, http.StatusBadRequest, "signed URL TTL cannot exceed 7 days")
		return
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

// GetCapabilities reports feature support for the currently active storage backend.
func (h *Handler) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": h.svc.Capabilities(),
	})
}

// ReplaceAsset replaces an existing media file while preserving its ID and entry references.
func (h *Handler) ReplaceAsset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
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

	asset, err := h.svc.ReplaceAsset(r.Context(), id, file, header.Filename, header.Size, altText, caption)
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			h.writeError(w, http.StatusNotFound, "Media asset not found")
			return
		}
		if errors.Is(err, ErrMediaValidation) {
			h.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": asset,
	})
}

// GetTransformedURL builds a validated transformed delivery URL for image/video assets.
func (h *Handler) GetTransformedURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	q := r.URL.Query()
	wVal, _ := strconv.Atoi(q.Get("w"))
	hVal, _ := strconv.Atoi(q.Get("h"))
	opts := storage.ImageTransformOptions{
		Width:   wVal,
		Height:  hVal,
		Crop:    q.Get("crop"),
		Format:  q.Get("format"),
		Quality: q.Get("quality"),
		Gravity: q.Get("gravity"),
		DPR:     q.Get("dpr"),
	}

	transformedURL, err := h.svc.TransformAssetURL(r.Context(), id, opts)
	if err != nil {
		if errors.Is(err, ErrAssetNotFound) {
			h.writeError(w, http.StatusNotFound, "Media asset not found")
			return
		}
		if errors.Is(err, storage.ErrInvalidPath) {
			h.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"url": transformedURL,
	})
}

// ListFolders returns folders under parent_id query param (or root if not provided).
func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var parentID *string
	if q.Has("parent_id") {
		p := q.Get("parent_id")
		if p == "root" || p == "" {
			empty := ""
			parentID = &empty
		} else {
			parentID = &p
		}
	}

	folders, err := h.svc.ListFolders(r.Context(), parentID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": folders,
	})
}

// CreateFolder creates a new folder.
func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	var createdBy *string
	if user := auth.GetUserFromContext(r.Context()); user != nil {
		createdBy = &user.ID
	}

	folder, err := h.svc.CreateFolder(r.Context(), input.Name, input.ParentID, createdBy)
	if err != nil {
		if errors.Is(err, ErrDuplicateFolder) {
			h.writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]any{
		"data": folder,
	})
}

// GetFolder returns folder details by ID.
func (h *Handler) GetFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	folder, err := h.svc.GetFolder(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusNotFound, "Media folder not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": folder,
	})
}

// RenameFolder changes a folder's name and synchronizes its path and storage structure.
func (h *Handler) RenameFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	folder, err := h.svc.RenameFolder(r.Context(), id, input.Name)
	if err != nil {
		if errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusNotFound, "Media folder not found")
			return
		}
		if errors.Is(err, ErrDuplicateFolder) {
			h.writeError(w, http.StatusConflict, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": folder,
	})
}

// DeleteFolder deletes a folder, optionally recursively.
func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	recursive := r.URL.Query().Get("recursive") == "true"

	if err := h.svc.DeleteFolder(r.Context(), id, recursive); err != nil {
		if errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusNotFound, "Media folder not found")
			return
		}
		if errors.Is(err, ErrFolderNotEmpty) {
			h.writeError(w, http.StatusConflict, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"message": "Media folder deleted successfully",
	})
}

// GetFolderContents returns the folder's direct subfolders and media assets.
func (h *Handler) GetFolderContents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	folder, err := h.svc.GetFolder(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusNotFound, "Media folder not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	subfolders, err := h.svc.ListFolders(r.Context(), &id)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	assets, _, err := h.svc.ListAssets(r.Context(), MediaFilter{
		FolderID: &id,
		Limit:    100,
	})
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"folder":     folder,
			"subfolders": subfolders,
			"assets":     assets,
		},
	})
}

// MoveFolder moves a folder to a new parent folder.
func (h *Handler) MoveFolder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input struct {
		TargetParentID *string `json:"target_parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	folder, err := h.svc.MoveFolder(r.Context(), id, input.TargetParentID)
	if err != nil {
		if errors.Is(err, ErrFolderNotFound) {
			h.writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if errors.Is(err, ErrCircularFolder) {
			h.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, ErrDuplicateFolder) {
			h.writeError(w, http.StatusConflict, err.Error())
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": folder,
	})
}

// GetTree returns the entire hierarchical folder structure.
func (h *Handler) GetTree(w http.ResponseWriter, r *http.Request) {
	tree, err := h.svc.GetFolderTree(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": tree,
	})
}

// ServeFile serves the uploaded media asset, enforcing HMAC signature verification if enabled.
// For HLS streams (.m3u8), it transparently injects HMAC signature params into relative URLs.
func (h *Handler) ServeFile(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "*")
	key = strings.TrimPrefix(key, "/")

	tenantID := ""
	if tenant := tenancy.GetTenant(r.Context()); tenant != nil {
		tenantID = tenant.ID
	}

	// If signed URLs are enabled, verify HMAC signature
	if h.svc.ShouldRequireSignedURLs(r.Context()) && h.svc.Signer() != nil {
		if err := h.svc.Signer().VerifyURLForTenant(r.URL, tenantID); err != nil {
			if errors.Is(err, storage.ErrSignatureExpired) {
				http.Error(w, "Access Denied: Signed URL has expired", http.StatusForbidden)
				return
			}
			http.Error(w, "Access Denied: Invalid or missing HMAC signature", http.StatusForbidden)
			return
		}
	}

	// Open file from registered local storage provider when available so legacy local uploads continue serving across driver switches
	prov := h.svc.Storage()
	if resolver, ok := prov.(storage.ProviderResolver); ok {
		if lp, found := resolver.ResolveProvider("local"); found {
			prov = lp
		}
	}
	reader, err := prov.Open(r.Context(), key)
	if err != nil {
		if errors.Is(err, storage.ErrFileNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Error retrieving file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	// Check for HLS playlist or specific HLS formats
	ext := strings.ToLower(filepath.Ext(key))
	if ext == ".m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		content, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, "Error reading playlist: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if h.svc.ShouldRequireSignedURLs(r.Context()) && r.URL.RawQuery != "" {
			if expires, err := strconv.ParseInt(r.URL.Query().Get("expires"), 10, 64); err == nil {
				content = RewriteHLSPlaylistWithPathSignedParams(content, "/uploads/"+key, time.Unix(expires, 0), h.svc.Signer(), tenantID)
			}
		}

		_, _ = w.Write(content)
		return
	}

	// HLS segments and assets
	hlsMIME := GetHLSEntryMIMEType(key)
	if hlsMIME != "application/octet-stream" {
		w.Header().Set("Content-Type", hlsMIME)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = io.Copy(w, reader)
		return
	}

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
