package templates

import (
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/themes"
)

// Renderer coordinates template compiling, caching, and HTML response generation.
type Renderer struct {
	mu           sync.RWMutex
	cfg          *config.Config
	themeMgr     *themes.Manager
	contentSvc   *content.Service
	logger       *slog.Logger
	cache        map[string]*template.Template
	cacheEnabled bool
}

// NewRenderer creates a new HTML template renderer.
func NewRenderer(
	cfg *config.Config,
	themeMgr *themes.Manager,
	contentSvc *content.Service,
	logger *slog.Logger,
) *Renderer {
	if logger == nil {
		logger = slog.Default()
	}

	return &Renderer{
		cfg:          cfg,
		themeMgr:     themeMgr,
		contentSvc:   contentSvc,
		logger:       logger,
		cache:        make(map[string]*template.Template),
		cacheEnabled: cfg.Themes.Cache,
	}
}

// ListThemes returns discovered theme manifests and marks the active theme.
func (r *Renderer) ListThemes() []map[string]any {
	active := r.themeMgr.ActiveThemeSlug()
	manifests := r.themeMgr.List()
	themes := make([]map[string]any, 0, len(manifests))
	for _, manifest := range manifests {
		themes = append(themes, map[string]any{
			"name":              manifest.Name,
			"slug":              manifest.Slug,
			"version":           manifest.Version,
			"description":       manifest.Description,
			"author":            manifest.Author,
			"engine":            manifest.Engine,
			"minimumCmsVersion": manifest.MinimumCMSVersion,
			"settings":          manifest.Settings,
			"is_active":         manifest.Slug == active,
		})
	}
	return themes
}

// GetTemplate compiles or retrieves from cache the template tree for a specific page.
func (r *Renderer) GetTemplate(theme *themes.Theme, pageRelPath string) (*template.Template, error) {
	cacheKey := fmt.Sprintf("%s:%s", theme.Manifest.Slug, pageRelPath)

	if r.cacheEnabled {
		r.mu.RLock()
		tmpl, ok := r.cache[cacheKey]
		r.mu.RUnlock()
		if ok {
			return tmpl, nil
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double check cache
	if r.cacheEnabled {
		if tmpl, ok := r.cache[cacheKey]; ok {
			return tmpl, nil
		}
	}

	// 1. Base Layout
	baseLayoutPath := filepath.Join(theme.RootDir, "templates", "layouts", "base.html")
	if _, err := os.Stat(baseLayoutPath); err != nil {
		return nil, fmt.Errorf("theme %q missing layouts/base.html: %w", theme.Manifest.Slug, err)
	}

	files := []string{baseLayoutPath}

	// 2. Discover Partials
	partialsDir := filepath.Join(theme.RootDir, "templates", "partials")
	if entries, err := os.ReadDir(partialsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".html") {
				files = append(files, filepath.Join(partialsDir, e.Name()))
			}
		}
	}

	// 3. Add Target Page Template
	pageFullPath := filepath.Join(theme.RootDir, "templates", pageRelPath)
	if _, err := os.Stat(pageFullPath); err != nil {
		return nil, fmt.Errorf("page template %q not found in theme: %w", pageRelPath, err)
	}
	files = append(files, pageFullPath)

	// 4. Compile Template with SafeFuncMap
	funcMap := SafeFuncMap(theme.Manifest.Slug)
	tmpl := template.New("base").Funcs(funcMap)

	parsedTmpl, err := tmpl.ParseFiles(files...)
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates %v: %w", files, err)
	}

	if r.cacheEnabled {
		r.cache[cacheKey] = parsedTmpl
	}

	return parsedTmpl, nil
}

// Render executes a template and writes output to the given writer.
func (r *Renderer) Render(w io.Writer, pageRelPath string, data *TemplateData) error {
	theme, err := r.themeMgr.GetActive()
	if err != nil {
		return err
	}

	tmpl, err := r.GetTemplate(theme, pageRelPath)
	if err != nil {
		return err
	}

	if data == nil {
		data = &TemplateData{}
	}
	if data.GeneratedAt.IsZero() {
		data.GeneratedAt = time.Now().UTC()
	}
	if data.Site.ThemeSettings == nil {
		data.Site.ThemeSettings = theme.Manifest.Settings
	}

	return tmpl.ExecuteTemplate(w, "base.html", data)
}

// MountWebRoutes registers traditional CMS routes on the Chi router.
func (r *Renderer) MountWebRoutes(router chi.Router) {
	// 1. Static Theme Assets
	router.Get("/themes/{theme}/assets/*", r.serveThemeAssets)

	// 2. SEO & Feeds
	router.Get("/sitemap.xml", r.serveSitemap)
	router.Get("/rss.xml", r.serveRSS)
	router.Get("/feed.xml", r.serveRSS)

	// 3. Website Pages
	router.Get("/", r.handleHome)
	router.Get("/{slug}", r.handleSingleOrArchive)
	router.Get("/{type}/{slug}", r.handleCollectionItem)
}

func (r *Renderer) serveThemeAssets(w http.ResponseWriter, req *http.Request) {
	themeSlug := chi.URLParam(req, "theme")
	assetPath := chi.URLParam(req, "*")

	theme, err := r.themeMgr.Get(themeSlug)
	if err != nil {
		http.NotFound(w, req)
		return
	}

	fullPath, err := theme.ResolveAssetPath(assetPath)
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Cache static assets for 1 hour in browser
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, req, fullPath)
}

func (r *Renderer) handleHome(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	previewDraft := canPreview(req)

	siteData := r.buildSiteInfo()
	templateData := &TemplateData{
		Site: siteData,
		SEO: SEOMetadata{
			Title:       siteData.Title,
			Description: siteData.Tagline,
			Canonical:   siteData.BaseURL + "/",
		},
		Breadcrumbs: []BreadcrumbItem{{Title: "Home", URL: "/"}},
		Preview:     previewDraft,
		Page:        make(map[string]any),
	}

	// Check if a single content type "homepage" exists
	if r.contentSvc != nil {
		if singleHome, err := r.contentSvc.GetSingleEntry(ctx, "homepage", previewDraft); err == nil {
			if previewDraft {
				templateData.Page = singleHome.DraftData
			} else {
				templateData.Page = singleHome.PublishedData
			}
		}

		// Load recent published posts/articles if available
		contentTypes, _ := r.contentSvc.ListContentTypes(ctx)
		for _, ct := range contentTypes {
			if ct.Kind == content.KindCollection {
				entries, _, err := r.contentSvc.ListEntries(ctx, ct.UID, content.EntryFilter{Limit: 10}, previewDraft)
				if err == nil {
					for _, e := range entries {
						itemData := e.PublishedData
						if previewDraft || itemData == nil {
							itemData = e.DraftData
						}
						itemData["_id"] = e.ID
						itemData["_slug"] = e.Slug
						itemData["_type"] = ct.UID
						templateData.Posts = append(templateData.Posts, itemData)
					}
				}
				break
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.Render(w, filepath.Join("pages", "home.html"), templateData); err != nil {
		r.logger.Error("Error rendering home template", "error", err)
		r.renderError(w, http.StatusInternalServerError, "Internal Server Error")
	}
}

func (r *Renderer) handleSingleOrArchive(w http.ResponseWriter, req *http.Request) {
	slug := chi.URLParam(req, "slug")
	ctx := req.Context()
	previewDraft := canPreview(req)

	// Ignore static assets or system paths
	if strings.Contains(slug, ".") || slug == "api" || slug == "healthz" || slug == "readyz" {
		http.NotFound(w, req)
		return
	}

	if r.contentSvc == nil {
		r.renderError(w, http.StatusNotFound, "Page Not Found")
		return
	}

	// Case 1: Check if slug matches a Content Type
	ct, err := r.contentSvc.GetContentType(ctx, slug)
	if err == nil {
		if ct.Kind == content.KindCollection {
			// Render Archive
			entries, total, err := r.contentSvc.ListEntries(ctx, ct.UID, content.EntryFilter{Limit: 20}, previewDraft)
			if err != nil {
				r.renderError(w, http.StatusInternalServerError, err.Error())
				return
			}

			var postList []map[string]any
			for _, e := range entries {
				d := e.PublishedData
				if previewDraft || d == nil {
					d = e.DraftData
				}
				d["_id"] = e.ID
				d["_slug"] = e.Slug
				d["_type"] = ct.UID
				postList = append(postList, d)
			}

			siteInfo := r.buildSiteInfo()
			data := &TemplateData{
				Site: siteInfo,
				SEO: SEOMetadata{
					Title:       ct.Name,
					Description: fmt.Sprintf("%s Archive", ct.Name),
					Canonical:   fmt.Sprintf("%s/%s", siteInfo.BaseURL, ct.UID),
				},
				Breadcrumbs: []BreadcrumbItem{
					{Title: "Home", URL: "/"},
					{Title: ct.Name, URL: "/" + ct.UID},
				},
				Pagination: &Pagination{
					CurrentPage: 1,
					TotalPages:  1,
					TotalItems:  total,
					PerPage:     20,
				},
				Posts:   postList,
				Preview: previewDraft,
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := r.Render(w, filepath.Join("pages", "archive.html"), data); err != nil {
				r.logger.Error("Failed to render archive", "type", ct.UID, "error", err)
				r.renderError(w, http.StatusInternalServerError, err.Error())
			}
			return
		} else {
			// Single Type page (e.g. /about, /contact)
			singleEntry, err := r.contentSvc.GetSingleEntry(ctx, ct.UID, previewDraft)
			if err != nil {
				r.renderError(w, http.StatusNotFound, "Page not found")
				return
			}

			pageData := singleEntry.PublishedData
			if previewDraft || pageData == nil {
				pageData = singleEntry.DraftData
			}

			siteInfo := r.buildSiteInfo()
			title, _ := pageData["title"].(string)
			if title == "" {
				title = ct.Name
			}

			data := &TemplateData{
				Site: siteInfo,
				SEO: SEOMetadata{
					Title:     title,
					Canonical: fmt.Sprintf("%s/%s", siteInfo.BaseURL, ct.UID),
				},
				Breadcrumbs: []BreadcrumbItem{
					{Title: "Home", URL: "/"},
					{Title: title, URL: "/" + ct.UID},
				},
				Page:    pageData,
				Preview: previewDraft,
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if err := r.Render(w, filepath.Join("pages", "article.html"), data); err != nil {
				r.renderError(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
	}

	// Case 2: Check if slug matches an individual entry across collections
	contentTypes, _ := r.contentSvc.ListContentTypes(ctx)
	for _, cType := range contentTypes {
		if cType.Kind == content.KindCollection {
			entry, err := r.contentSvc.GetEntryBySlug(ctx, cType.UID, slug, previewDraft)
			if err == nil {
				r.renderEntryPage(w, req, cType, entry, previewDraft)
				return
			}
		}
	}

	r.renderError(w, http.StatusNotFound, "Page Not Found")
}

func (r *Renderer) handleCollectionItem(w http.ResponseWriter, req *http.Request) {
	if r.contentSvc == nil {
		r.renderError(w, http.StatusNotFound, "Content Type Not Found")
		return
	}

	typeUID := chi.URLParam(req, "type")
	slug := chi.URLParam(req, "slug")
	ctx := req.Context()
	previewDraft := canPreview(req)

	ct, err := r.contentSvc.GetContentType(ctx, typeUID)
	if err != nil {
		r.renderError(w, http.StatusNotFound, "Content Type Not Found")
		return
	}

	entry, err := r.contentSvc.GetEntryBySlug(ctx, typeUID, slug, previewDraft)
	if err != nil {
		// Try fallback by ID
		entry, err = r.contentSvc.GetEntry(ctx, typeUID, slug, previewDraft)
		if err != nil {
			r.renderError(w, http.StatusNotFound, "Entry Not Found")
			return
		}
	}

	r.renderEntryPage(w, req, ct, entry, previewDraft)
}

func (r *Renderer) renderEntryPage(w http.ResponseWriter, req *http.Request, ct *content.ContentType, entry *content.ContentEntry, previewDraft bool) {
	itemData := entry.PublishedData
	if previewDraft || itemData == nil {
		itemData = entry.DraftData
	}
	itemData["_id"] = entry.ID
	itemData["_slug"] = entry.Slug
	itemData["_type"] = ct.UID

	title, _ := itemData["title"].(string)
	if title == "" {
		title = fmt.Sprintf("%s Item", ct.Name)
	}

	excerpt, _ := itemData["excerpt"].(string)

	siteInfo := r.buildSiteInfo()
	canonicalURL := fmt.Sprintf("%s/%s/%s", siteInfo.BaseURL, ct.UID, entry.Slug)

	data := &TemplateData{
		Site: siteInfo,
		SEO: SEOMetadata{
			Title:       title,
			Description: excerpt,
			Canonical:   canonicalURL,
		},
		Breadcrumbs: []BreadcrumbItem{
			{Title: "Home", URL: "/"},
			{Title: ct.Name, URL: "/" + ct.UID},
			{Title: title, URL: canonicalURL},
		},
		Entry:   itemData,
		Page:    itemData,
		Preview: previewDraft,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.Render(w, filepath.Join("pages", "article.html"), data); err != nil {
		r.logger.Error("Failed to render article template", "slug", entry.Slug, "error", err)
		r.renderError(w, http.StatusInternalServerError, err.Error())
	}
}

func (r *Renderer) serveSitemap(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	siteInfo := r.buildSiteInfo()

	type URLTag struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod,omitempty"`
	}
	type URLSet struct {
		XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
		URLs    []URLTag `xml:"url"`
	}

	urlset := URLSet{
		URLs: []URLTag{
			{Loc: siteInfo.BaseURL + "/", LastMod: time.Now().UTC().Format("2006-01-02")},
		},
	}

	if r.contentSvc != nil {
		contentTypes, _ := r.contentSvc.ListContentTypes(ctx)
		for _, ct := range contentTypes {
			if ct.Kind == content.KindCollection {
				urlset.URLs = append(urlset.URLs, URLTag{
					Loc: fmt.Sprintf("%s/%s", siteInfo.BaseURL, ct.UID),
				})

				entries, _, err := r.contentSvc.ListEntries(ctx, ct.UID, content.EntryFilter{Limit: 500}, false)
				if err == nil {
					for _, e := range entries {
						slug := e.Slug
						if slug == "" {
							slug = e.ID
						}
						urlset.URLs = append(urlset.URLs, URLTag{
							Loc:     fmt.Sprintf("%s/%s/%s", siteInfo.BaseURL, ct.UID, slug),
							LastMod: e.UpdatedAt.Format("2006-01-02"),
						})
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(urlset)
}

func (r *Renderer) serveRSS(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	siteInfo := r.buildSiteInfo()

	type Item struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		PubDate     string `xml:"pubDate"`
	}

	type Channel struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		Items       []Item `xml:"item"`
	}

	type RSS struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Channel Channel  `xml:"channel"`
	}

	feed := RSS{
		Version: "2.0",
		Channel: Channel{
			Title:       siteInfo.Title,
			Link:        siteInfo.BaseURL,
			Description: siteInfo.Tagline,
		},
	}

	if r.contentSvc != nil {
		contentTypes, _ := r.contentSvc.ListContentTypes(ctx)
		for _, ct := range contentTypes {
			if ct.Kind == content.KindCollection {
				entries, _, err := r.contentSvc.ListEntries(ctx, ct.UID, content.EntryFilter{Limit: 20}, false)
				if err == nil {
					for _, e := range entries {
						title, _ := e.PublishedData["title"].(string)
						desc, _ := e.PublishedData["content"].(string)
						if desc == "" {
							desc, _ = e.PublishedData["excerpt"].(string)
						}

						pubTime := e.CreatedAt
						if e.PublishedAt != nil {
							pubTime = *e.PublishedAt
						}

						slug := e.Slug
						if slug == "" {
							slug = e.ID
						}

						feed.Channel.Items = append(feed.Channel.Items, Item{
							Title:       title,
							Link:        fmt.Sprintf("%s/%s/%s", siteInfo.BaseURL, ct.UID, slug),
							Description: desc,
							PubDate:     pubTime.Format(time.RFC1123Z),
						})
					}
				}
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(feed)
}

func (r *Renderer) renderError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!DOCTYPE html><html><head><title>%d - Error</title></head><body style=\"font-family:sans-serif;padding:2rem;\"><h1>%d</h1><p>%s</p><a href=\"/\">&larr; Back to Home</a></body></html>", status, status, template.HTMLEscapeString(msg))
}

func (r *Renderer) buildSiteInfo() SiteInfo {
	return SiteInfo{
		Title:    "Go CMS",
		Tagline:  "Modern Extensible CMS Built in Go",
		BaseURL:  fmt.Sprintf("http://%s:%d", r.cfg.Server.Host, r.cfg.Server.Port),
		Language: "en",
	}
}

func canPreview(r *http.Request) bool {
	if r.URL.Query().Get("preview") != "true" {
		return false
	}
	user := auth.GetUserFromContext(r.Context())
	if user == nil {
		return false
	}
	return auth.HasPermission(user, "read", "content")
}
