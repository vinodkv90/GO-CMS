package content

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
)

// Handler handles HTTP requests for content types and dynamic content entries.
type Handler struct {
	svc *Service
}

// NewHandler creates a new content HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts all Content Type Builder and Content Manager routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Content Type Builder Routes
	r.Route("/content-types", func(ctr chi.Router) {
		ctr.Get("/", h.ListTypes)
		ctr.Get("/{uid}", h.GetType)

		// Type mutation requires admin permission
		ctr.Group(func(admin chi.Router) {
			admin.Use(auth.RequireAuth)
			admin.Use(auth.RequirePermission("manage", "content_types"))
			admin.Post("/", h.CreateType)
			admin.Put("/{uid}", h.UpdateType)
			admin.Delete("/{uid}", h.DeleteType)
			admin.Post("/{uid}/fields", h.AddField)
			admin.Delete("/{uid}/fields/{fieldKey}", h.DeleteField)
		})
	})

	// Dynamic Content Entry Routes
	r.Route("/content/{uid}", func(cr chi.Router) {
		cr.Get("/", h.ListOrGetSingle)
		cr.Get("/{id}", h.GetEntry)

		// Mutating entries requires content write permissions
		cr.Group(func(protected chi.Router) {
			protected.Use(auth.RequireAuth)
			protected.Post("/", h.CreateEntry)
			protected.Put("/{id}", h.UpdateEntry)
			protected.Delete("/{id}", h.DeleteEntry)
			protected.Post("/{id}/publish", h.PublishEntry)
			protected.Post("/{id}/unpublish", h.UnpublishEntry)
			protected.Get("/{id}/versions", h.ListVersions)
			protected.Post("/{id}/restore/{version}", h.RestoreVersion)
		})
	})
}

// --- Content Types Handlers ---

func (h *Handler) ListTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.svc.ListContentTypes(r.Context())
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"data": types})
}

func (h *Handler) GetType(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	ct, err := h.svc.GetContentType(r.Context(), uid)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "content type not found"})
		return
	}
	respondJSON(w, http.StatusOK, ct)
}

func (h *Handler) CreateType(w http.ResponseWriter, r *http.Request) {
	var req CreateTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ct, err := h.svc.CreateContentType(r.Context(), req)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusCreated, ct)
}

func (h *Handler) UpdateType(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	var req UpdateTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	ct, err := h.svc.UpdateContentType(r.Context(), uid, req)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, ct)
}

func (h *Handler) DeleteType(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	if err := h.svc.DeleteContentType(r.Context(), uid); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "content type not found"})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "content type deleted"})
}

func (h *Handler) AddField(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	var field ContentField
	if err := json.NewDecoder(r.Body).Decode(&field); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if err := h.svc.AddField(r.Context(), uid, field); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"message": "field added successfully"})
}

func (h *Handler) DeleteField(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	fieldKey := chi.URLParam(r, "fieldKey")
	if err := h.svc.DeleteField(r.Context(), uid, fieldKey); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "field removed"})
}

// --- Content Entries Handlers ---

// ListOrGetSingle queries collection entries or retrieves the single type record.
func (h *Handler) ListOrGetSingle(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	ct, err := h.svc.GetContentType(r.Context(), uid)
	if err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": "content type not found"})
		return
	}

	previewDraft := canPreviewDraft(r)

	// Single Type
	if ct.Kind == KindSingle {
		entry, err := h.svc.GetSingleEntry(r.Context(), uid, previewDraft)
		if err != nil {
			if err == ErrNotPublished {
				respondJSON(w, http.StatusNotFound, map[string]string{"error": "content not published"})
				return
			}
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "single entry not created yet"})
			return
		}
		respondJSON(w, http.StatusOK, filterResponse(entry, previewDraft))
		return
	}

	// Collection Type
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")

	filter := EntryFilter{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: order,
	}

	entries, total, err := h.svc.ListEntries(r.Context(), uid, filter, previewDraft)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	responseList := make([]any, 0, len(entries))
	for _, e := range entries {
		responseList = append(responseList, filterResponse(e, previewDraft))
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"data":  responseList,
		"total": total,
	})
}

func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")

	previewDraft := canPreviewDraft(r)

	entry, err := h.svc.GetEntry(r.Context(), uid, id, previewDraft)
	if err != nil {
		status := http.StatusNotFound
		if err == ErrNotPublished {
			status = http.StatusNotFound
		}
		respondJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, filterResponse(entry, previewDraft))
}

func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	user := auth.GetUserFromContext(r.Context())

	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	entry, err := h.svc.CreateEntry(r.Context(), uid, data, user.ID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusCreated, entry)
}

type UpdateEntryRequestBody struct {
	Data                 map[string]any `json:"data"`
	ExpectedDraftVersion int            `json:"expected_draft_version"`
}

func (h *Handler) UpdateEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")
	user := auth.GetUserFromContext(r.Context())

	var body UpdateEntryRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if body.ExpectedDraftVersion <= 0 {
		// Fallback: If not explicitly supplied, read current version to prevent lockup
		current, err := h.svc.GetEntry(r.Context(), uid, id, true)
		if err != nil {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
			return
		}
		body.ExpectedDraftVersion = current.DraftVersion
	}

	entry, err := h.svc.UpdateEntry(r.Context(), uid, id, body.Data, body.ExpectedDraftVersion, user.ID)
	if err != nil {
		status := http.StatusBadRequest
		if err == ErrVersionMismatch {
			status = http.StatusConflict
		}
		respondJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, entry)
}

func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")
	user := auth.GetUserFromContext(r.Context())

	hardDelete := r.URL.Query().Get("hard") == "true"

	if err := h.svc.DeleteEntry(r.Context(), uid, id, hardDelete, user.ID); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "entry deleted successfully"})
}

func (h *Handler) PublishEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")
	user := auth.GetUserFromContext(r.Context())

	entry, err := h.svc.PublishEntry(r.Context(), uid, id, user.ID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, entry)
}

func (h *Handler) UnpublishEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")
	user := auth.GetUserFromContext(r.Context())

	entry, err := h.svc.UnpublishEntry(r.Context(), uid, id, user.ID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, entry)
}

func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")

	versions, err := h.svc.ListVersions(r.Context(), uid, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"data": versions})
}

func (h *Handler) RestoreVersion(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")
	verStr := chi.URLParam(r, "version")
	user := auth.GetUserFromContext(r.Context())

	verNum, err := strconv.Atoi(verStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid version number"})
		return
	}

	entry, err := h.svc.RestoreVersion(r.Context(), uid, id, verNum, user.ID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, entry)
}

// canPreviewDraft checks if the request is permitted to view draft content.
func canPreviewDraft(r *http.Request) bool {
	if r.URL.Query().Get("preview") != "true" {
		return false
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		return false
	}
	return auth.HasPermission(user, "read", "content")
}

// filterResponse shapes output based on whether the view is draft preview or public.
func filterResponse(entry *ContentEntry, preview bool) any {
	if preview {
		return entry
	}
	// For public view, expose clean published data
	return map[string]any{
		"id":           entry.ID,
		"slug":         entry.Slug,
		"data":         entry.PublishedData,
		"version":      entry.PublishedVersion,
		"published_at": entry.PublishedAt,
	}
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
