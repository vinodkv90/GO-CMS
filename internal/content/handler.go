package content

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/fields"
)

// RestAPIRuntimeConfig mirrors live REST API settings enforced on content endpoints.
type RestAPIRuntimeConfig struct {
	DefaultLimit       int
	MaxLimit           int
	WithCount          bool
	MaxPopulateDepth   int
	ExposeDraftPreview bool
}

// Handler handles HTTP requests for content types and dynamic content entries.
type Handler struct {
	svc       *Service
	restCfgFn func(context.Context) RestAPIRuntimeConfig
}

// NewHandler creates a new content HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// SetRestAPISettingsProvider injects a live REST API settings resolver.
func (h *Handler) SetRestAPISettingsProvider(fn func(context.Context) RestAPIRuntimeConfig) {
	h.restCfgFn = fn
}

func (h *Handler) restConfig(ctx context.Context) RestAPIRuntimeConfig {
	if h.restCfgFn != nil {
		return h.restCfgFn(ctx)
	}
	return RestAPIRuntimeConfig{
		DefaultLimit:       25,
		MaxLimit:           100,
		WithCount:          true,
		MaxPopulateDepth:   MaxComponentDepth,
		ExposeDraftPreview: true,
	}
}

// RegisterRoutes mounts all Content Type Builder and Content Manager routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Content Type Builder Routes
	r.Route("/content-types", func(ctr chi.Router) {
		ctr.Get("/", h.ListTypes)
		ctr.Get("/field-capabilities", h.FieldCapabilities)
		ctr.Get("/custom-fields", h.CustomFields)
		ctr.Get("/{uid}", h.GetType)

		// Type mutation requires admin permission
		ctr.Group(func(admin chi.Router) {
			admin.Use(auth.RequireAuth)
			admin.Use(auth.RequirePermission("manage", "content_types"))
			admin.Post("/", h.CreateType)
			admin.Put("/{uid}", h.UpdateType)
			admin.Delete("/{uid}", h.DeleteType)
			admin.Post("/{uid}/fields", h.AddField)
			admin.Put("/{uid}/fields/{fieldKey}", h.UpdateField)
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
			protected.With(auth.RequirePermission("create", "content")).Post("/", h.CreateEntry)
			protected.With(auth.RequirePermission("update", "content")).Put("/{id}", h.UpdateEntry)
			protected.With(auth.RequirePermission("delete", "content")).Delete("/{id}", h.DeleteEntry)
			protected.With(auth.RequirePermission("publish", "content")).Post("/{id}/publish", h.PublishEntry)
			protected.With(auth.RequirePermission("publish", "content")).Post("/{id}/unpublish", h.UnpublishEntry)
			protected.With(auth.RequirePermission("read", "content")).Get("/{id}/versions", h.ListVersions)
			protected.With(auth.RequirePermission("update", "content")).Post("/{id}/restore/{version}", h.RestoreVersion)
		})
	})
}

func (h *Handler) FieldCapabilities(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, h.svc.FieldCapabilities())
}

// CustomFields returns descriptors for active plugin-registered custom field types.
func (h *Handler) CustomFields(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, h.svc.CustomFields())
}

func writeContentError(w http.ResponseWriter, status int, err error) {
	if fe, ok := asFieldError(err); ok {
		respondJSON(w, status, map[string]any{
			"error":  err.Error(),
			"field":  fe.Field,
			"fields": map[string]string{fe.Field: fe.Message},
		})
		return
	}
	respondJSON(w, status, map[string]string{"error": err.Error()})
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
		writeContentError(w, http.StatusBadRequest, err)
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
		writeContentError(w, http.StatusBadRequest, err)
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
		writeContentError(w, http.StatusBadRequest, err)
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

func (h *Handler) UpdateField(w http.ResponseWriter, r *http.Request) {
	var field ContentField
	if err := json.NewDecoder(r.Body).Decode(&field); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.svc.UpdateField(r.Context(), chi.URLParam(r, "uid"), chi.URLParam(r, "fieldKey"), field); err != nil {
		writeContentError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"message": "field updated successfully"})
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

	previewDraft := h.canPreviewDraft(r)

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
		respondJSON(w, http.StatusOK, h.filterResponse(r.Context(), entry, ct, previewDraft, auth.GetUserFromContext(r.Context())))
		return
	}

	// Collection Type
	restCfg := h.restConfig(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = restCfg.DefaultLimit
	} else if restCfg.MaxLimit > 0 && limit > restCfg.MaxLimit {
		limit = restCfg.MaxLimit
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	sortBy := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	search := r.URL.Query().Get("search")

	filter := EntryFilter{
		Limit:     limit,
		Offset:    offset,
		SortBy:    sortBy,
		SortOrder: order,
		Search:    search,
		Status:    r.URL.Query().Get("status"),
	}

	entries, total, err := h.svc.ListEntries(r.Context(), uid, filter, previewDraft)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	responseList := make([]any, 0, len(entries))
	for _, e := range entries {
		responseList = append(responseList, h.filterResponse(r.Context(), e, ct, previewDraft, auth.GetUserFromContext(r.Context())))
	}

	resp := map[string]any{
		"data": responseList,
	}
	if restCfg.WithCount {
		resp["total"] = total
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")

	previewDraft := h.canPreviewDraft(r)

	entry, err := h.svc.GetEntry(r.Context(), uid, id, previewDraft)
	if err != nil {
		status := http.StatusNotFound
		if err == ErrNotPublished {
			status = http.StatusNotFound
		}
		respondJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	ct, _ := h.svc.GetContentType(r.Context(), uid)
	respondJSON(w, http.StatusOK, h.filterResponse(r.Context(), entry, ct, previewDraft, auth.GetUserFromContext(r.Context())))
}

func (h *Handler) CreateEntry(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	user := auth.GetUserFromContext(r.Context())

	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Unwrap nested "data" wrapper if provided (e.g. Strapi or REST client format)
	if inner, ok := data["data"].(map[string]any); ok {
		data = inner
	}

	entry, err := h.svc.CreateEntry(r.Context(), uid, data, user.ID)
	if err != nil {
		writeContentError(w, http.StatusBadRequest, err)
		return
	}

	ct, _ := h.svc.GetContentType(r.Context(), uid)
	respondJSON(w, http.StatusCreated, h.filterResponse(r.Context(), entry, ct, true, user))
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
		writeContentError(w, status, err)
		return
	}

	ct, _ := h.svc.GetContentType(r.Context(), uid)
	respondJSON(w, http.StatusOK, h.filterResponse(r.Context(), entry, ct, true, user))
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
		writeContentError(w, http.StatusBadRequest, err)
		return
	}

	ct, _ := h.svc.GetContentType(r.Context(), uid)
	respondJSON(w, http.StatusOK, h.filterResponse(r.Context(), entry, ct, true, user))
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

	ct, _ := h.svc.GetContentType(r.Context(), uid)
	respondJSON(w, http.StatusOK, h.filterResponse(r.Context(), entry, ct, true, user))
}

func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	uid := chi.URLParam(r, "uid")
	id := chi.URLParam(r, "id")

	versions, err := h.svc.ListVersions(r.Context(), uid, id)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	ct, _ := h.svc.GetContentType(r.Context(), uid)
	for _, version := range versions {
		version.Snapshot = h.responseData(r.Context(), version.Snapshot, ct, true, auth.GetUserFromContext(r.Context()), 0)
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

	ct, _ := h.svc.GetContentType(r.Context(), uid)
	respondJSON(w, http.StatusOK, h.filterResponse(r.Context(), entry, ct, true, user))
}

// canPreviewDraft checks if draft preview is enabled and the request is permitted to view draft content.
func (h *Handler) canPreviewDraft(r *http.Request) bool {
	if r.URL.Query().Get("preview") != "true" {
		return false
	}
	if !h.restConfig(r.Context()).ExposeDraftPreview {
		return false
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		return false
	}
	return auth.RequestHasPermission(r.Context(), "read", "content")
}

// filterResponse shapes output based on whether the view is draft preview or public.
func (h *Handler) filterResponse(ctx context.Context, entry *ContentEntry, ct *ContentType, preview bool, user *auth.User) any {
	if entry == nil {
		return nil
	}
	clean := persistenceClone(entry)
	if preview {
		clean.DraftData = h.responseData(ctx, clean.DraftData, ct, true, user, 0)
		clean.PublishedData = h.responseData(ctx, clean.PublishedData, ct, true, user, 0)
		return clean
	}
	// For public view, expose clean published data
	return map[string]any{
		"id":           entry.ID,
		"slug":         entry.Slug,
		"data":         h.responseData(ctx, entry.PublishedData, ct, false, user, 0),
		"version":      entry.PublishedVersion,
		"published_at": entry.PublishedAt,
	}
}

func (h *Handler) responseData(ctx context.Context, data map[string]any, ct *ContentType, preview bool, user *auth.User, depth int) map[string]any {
	if data == nil {
		return nil
	}
	out := cloneData(data)
	maxDepth := h.restConfig(ctx).MaxPopulateDepth
	if maxDepth <= 0 || maxDepth > MaxComponentDepth {
		maxDepth = MaxComponentDepth
	}
	if ct == nil || depth > maxDepth {
		return out
	}
	for _, field := range ct.Fields {
		canRead := len(field.Configuration.ReadRoles) == 0 || auth.HasAnyRole(user, auth.RoleSuperAdmin) || auth.HasAnyRole(user, field.Configuration.ReadRoles...)
		if field.FieldType == fields.TypeSecret || field.Configuration.Private && (!preview || !canRead) {
			delete(out, field.FieldKey)
			continue
		}
		uid := field.Configuration.Component
		repeat := field.FieldType == fields.TypeRepeatableComponent || field.FieldType == fields.TypeDynamicZone || field.FieldType == fields.TypeComponent && field.Configuration.Repeatable
		value := out[field.FieldKey]
		if field.FieldType == fields.TypeDynamicZone {
			if objects, ok := value.([]any); ok {
				for index, raw := range objects {
					if object, ok := raw.(map[string]any); ok {
						if componentUID, ok := object["__component"].(string); ok {
							objects[index] = h.redactComponent(ctx, object, componentUID, preview, user, depth+1)
						}
					}
				}
				out[field.FieldKey] = objects
			}
		} else if repeat {
			if objects, ok := value.([]any); ok {
				for index, raw := range objects {
					if object, ok := raw.(map[string]any); ok {
						objects[index] = h.redactComponent(ctx, object, uid, preview, user, depth+1)
					}
				}
				out[field.FieldKey] = objects
			}
		} else if object, ok := value.(map[string]any); ok && (field.FieldType == fields.TypeComponent) {
			out[field.FieldKey] = h.redactComponent(ctx, object, uid, preview, user, depth+1)
		}
	}
	return out
}

func (h *Handler) redactComponent(ctx context.Context, data map[string]any, uid string, preview bool, user *auth.User, depth int) map[string]any {
	if uid == "" {
		return data
	}
	maxDepth := h.restConfig(ctx).MaxPopulateDepth
	if maxDepth <= 0 || maxDepth > MaxComponentDepth {
		maxDepth = MaxComponentDepth
	}
	if depth > maxDepth {
		truncated := map[string]any{"__truncated": true}
		if id, ok := data["id"]; ok {
			truncated["id"] = id
		}
		if comp, ok := data["__component"]; ok {
			truncated["__component"] = comp
		}
		return truncated
	}
	component, err := h.svc.GetContentType(ctx, uid)
	if err != nil || component.Kind != KindComponent {
		return data
	}
	clean := h.responseData(ctx, data, component, preview, user, depth)
	if marker, ok := data["__component"]; ok {
		clean["__component"] = marker
	}
	return clean
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
