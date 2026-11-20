package templates_test

import (
	"context"
	"encoding/xml"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/templates"
	"github.com/go-cms/cms/internal/themes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSafeFuncMap_SecurityAndHelpers(t *testing.T) {
	fnMap := templates.SafeFuncMap("default")

	// 1. sanitizeHTML strips scripts but keeps safe markup
	sanitizeFn := fnMap["sanitizeHTML"].(func(any) template.HTML)
	xssPayload := `<script>alert('pwned')</script><p>Safe Paragraph</p><a href="javascript:steal()">bad link</a>`
	cleanHTML := string(sanitizeFn(xssPayload))
	assert.NotContains(t, cleanHTML, "<script>")
	assert.NotContains(t, cleanHTML, "javascript:steal()")
	assert.Contains(t, cleanHTML, "<p>Safe Paragraph</p>")

	// 2. slugify
	slugifyFn := fnMap["slugify"].(func(string) string)
	assert.Equal(t, "hello-world-2026", slugifyFn("Hello World! 2026"))
	assert.Equal(t, "multi-dash-clean", slugifyFn("---Multi---Dash---Clean---"))

	// 3. truncate
	truncateFn := fnMap["truncate"].(func(string, int) string)
	assert.Equal(t, "Hello...", truncateFn("Hello World", 5))
	assert.Equal(t, "Short", truncateFn("Short", 10))

	// 4. formatDate
	formatDateFn := fnMap["formatDate"].(func(any, string) string)
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, "2026-11-08", formatDateFn(now, "2006-01-02"))
	assert.Equal(t, "Nov 08, 2026", formatDateFn("2026-11-08T12:00:00Z", ""))
	assert.Equal(t, "", formatDateFn(nil, ""))

	// 5. timeAgo
	timeAgoFn := fnMap["timeAgo"].(func(any) string)
	assert.Equal(t, "just now", timeAgoFn(time.Now()))
	assert.Equal(t, "yesterday", timeAgoFn(time.Now().Add(-25*time.Hour)))

	// 6. formatCurrency
	currFn := fnMap["formatCurrency"].(func(any, string) string)
	assert.Equal(t, "$49.99", currFn(49.99, "$"))
	assert.Equal(t, "€100.00", currFn(100, "€"))

	// 7. Math helpers
	addFn := fnMap["add"].(func(int, int) int)
	subFn := fnMap["sub"].(func(int, int) int)
	divFn := fnMap["div"].(func(int, int) int)
	assert.Equal(t, 15, addFn(10, 5))
	assert.Equal(t, 5, subFn(10, 5))
	assert.Equal(t, 2, divFn(10, 5))
	assert.Equal(t, 0, divFn(10, 0)) // Zero division safe

	// 8. dict
	dictFn := fnMap["dict"].(func(...any) (map[string]any, error))
	d, err := dictFn("key1", "val1", "key2", 42)
	require.NoError(t, err)
	assert.Equal(t, "val1", d["key1"])
	assert.Equal(t, 42, d["key2"])

	_, err = dictFn("odd")
	assert.Error(t, err)

	// 9. assetPath
	assetFn := fnMap["assetPath"].(func(string) string)
	assert.Equal(t, "/themes/default/assets/css/style.css", assetFn("css/style.css"))
	assert.Equal(t, "/themes/default/assets/images/logo.png", assetFn("/images/logo.png"))
}

func setupTestRenderer(t *testing.T) (*templates.Renderer, *content.Service, *themes.Manager, *config.Config) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.DefaultConfig()

	// Find the project themes directory
	themesDir := filepath.Join("..", "..", "themes")
	if _, err := os.Stat(themesDir); err != nil {
		themesDir = filepath.Join(".", "themes")
	}

	themeMgr, err := themes.NewManager(themesDir, "default", logger)
	require.NoError(t, err)

	contentRepo := content.NewMemoryContentRepository()
	contentSvc := content.NewService(contentRepo, contentRepo, contentRepo, fields.NewRegistry(), content.NewEventBus(), logger)

	renderer := templates.NewRenderer(cfg, themeMgr, contentSvc, logger)
	return renderer, contentSvc, themeMgr, cfg
}

func TestRenderer_RenderPages(t *testing.T) {
	renderer, _, _, _ := setupTestRenderer(t)

	// 1. Render Home
	var buf strings.Builder
	data := &templates.TemplateData{
		Site: templates.SiteInfo{
			Title:   "My Test Blog",
			Tagline: "Coding with Go",
		},
		SEO: templates.SEOMetadata{
			Title:       "Home",
			Description: "Welcome home",
		},
		Preview: false,
	}
	err := renderer.Render(&buf, filepath.Join("pages", "home.html"), data)
	require.NoError(t, err)
	rendered := buf.String()
	assert.Contains(t, rendered, "<!DOCTYPE html>")
	assert.Contains(t, rendered, "My Test Blog")
	assert.Contains(t, rendered, "Coding with Go")
	assert.NotContains(t, rendered, "PREVIEW MODE")

	// 2. Render with Preview Mode
	buf.Reset()
	data.Preview = true
	err = renderer.Render(&buf, filepath.Join("pages", "home.html"), data)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "PREVIEW MODE")

	// 3. Render Article
	buf.Reset()
	articleData := &templates.TemplateData{
		Site: templates.SiteInfo{Title: "My Test Blog"},
		Breadcrumbs: []templates.BreadcrumbItem{
			{Title: "Home", URL: "/"},
			{Title: "Articles", URL: "/articles"},
			{Title: "First Post", URL: "/articles/first-post"},
		},
		Entry: map[string]any{
			"title":   "My First Post",
			"content": "<p>This is <strong>secure</strong> content.</p>",
		},
	}
	err = renderer.Render(&buf, filepath.Join("pages", "article.html"), articleData)
	require.NoError(t, err)
	renderedArticle := buf.String()
	assert.Contains(t, renderedArticle, "My First Post")
	assert.Contains(t, renderedArticle, "This is <strong>secure</strong> content.")
	assert.Contains(t, renderedArticle, "Articles")

	// 4. Render Archive
	buf.Reset()
	archiveData := &templates.TemplateData{
		Site: templates.SiteInfo{Title: "My Test Blog"},
		SEO:  templates.SEOMetadata{Title: "Articles Archive"},
		Posts: []map[string]any{
			{
				"_type": "articles",
				"_slug": "post-1",
				"title": "Post Number 1",
			},
			{
				"_type": "articles",
				"_slug": "post-2",
				"title": "Post Number 2",
			},
		},
	}
	err = renderer.Render(&buf, filepath.Join("pages", "archive.html"), archiveData)
	require.NoError(t, err)
	renderedArchive := buf.String()
	assert.Contains(t, renderedArchive, "Articles Archive")
	assert.Contains(t, renderedArchive, "Post Number 1")
	assert.Contains(t, renderedArchive, "Post Number 2")
}

func TestRenderer_WebRoutesAndContentIntegration(t *testing.T) {
	renderer, contentSvc, _, _ := setupTestRenderer(t)
	ctx := context.Background()

	// 1. Create a content type: "articles"
	articleType, err := contentSvc.CreateContentType(ctx, content.CreateTypeRequest{
		Name: "Articles",
		UID:  "articles",
		Kind: content.KindCollection,
		Fields: []content.ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
			{Name: "Content", FieldKey: "content", FieldType: fields.TypeRichText},
			{Name: "Excerpt", FieldKey: "excerpt", FieldType: fields.TypeLongText},
		},
	})
	require.NoError(t, err)

	// 2. Create and publish an article entry
	entry, err := contentSvc.CreateEntry(ctx, articleType.UID, map[string]any{
		"title":   "Hello from Go Template Engine",
		"content": "<p>Server side rendering with standard library html/template!</p>",
		"excerpt": "A brief summary of native Go templates.",
	}, "user-1")
	require.NoError(t, err)

	// Publish the article
	publishedEntry, err := contentSvc.PublishEntry(ctx, articleType.UID, entry.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, content.StatusPublished, publishedEntry.Status)

	// 3. Mount Web Routes on Chi router
	router := chi.NewRouter()
	renderer.MountWebRoutes(router)

	// Test 1: GET / (Homepage)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Go CMS")
	assert.Contains(t, body, "Hello from Go Template Engine")

	// Test 2: GET /articles (Archive Page)
	req = httptest.NewRequest("GET", "/articles", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	body = w.Body.String()
	assert.Contains(t, body, "Articles")
	assert.Contains(t, body, "Hello from Go Template Engine")

	// Test 3: GET /articles/hello-from-go-template-engine (Single Article Page)
	req = httptest.NewRequest("GET", "/articles/"+publishedEntry.Slug, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	body = w.Body.String()
	assert.Contains(t, body, "Hello from Go Template Engine")
	assert.Contains(t, body, "Server side rendering with standard library html/template!")

	// Test 4: Static Asset Serving GET /themes/default/assets/css/style.css
	req = httptest.NewRequest("GET", "/themes/default/assets/css/style.css", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/css")
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=3600")
	assert.Contains(t, w.Body.String(), "--primary")

	// Test 5: SEO Sitemap GET /sitemap.xml
	req = httptest.NewRequest("GET", "/sitemap.xml", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))

	type URLSet struct {
		XMLName xml.Name `xml:"urlset"`
		URLs    []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	var sitemap URLSet
	err = xml.Unmarshal(w.Body.Bytes(), &sitemap)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(sitemap.URLs), 2)

	// Test 6: RSS Feed GET /rss.xml
	req = httptest.NewRequest("GET", "/rss.xml", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/xml; charset=utf-8", w.Header().Get("Content-Type"))

	type RSS struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title string `xml:"title"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	var feed RSS
	err = xml.Unmarshal(w.Body.Bytes(), &feed)
	require.NoError(t, err)
	assert.NotEmpty(t, feed.Channel.Items)
	assert.Equal(t, "Hello from Go Template Engine", feed.Channel.Items[0].Title)

	// Test 7: Page Builder Routable Page GET /about-us
	pagesType, err := contentSvc.CreateContentType(ctx, content.CreateTypeRequest{
		Name: "Pages",
		UID:  "pages",
		Kind: content.KindCollection,
		Fields: []content.ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText},
			{Name: "Template", FieldKey: "template", FieldType: fields.TypeShortText},
			{Name: "Blocks", FieldKey: "blocks", FieldType: fields.TypeJSON},
			{Name: "SEO", FieldKey: "seo", FieldType: fields.TypeJSON},
		},
	})
	require.NoError(t, err)

	pageEntry, err := contentSvc.CreateEntry(ctx, pagesType.UID, map[string]any{
		"title":    "About Us",
		"template": "default",
		"blocks": []map[string]any{
			{
				"id":   "sec-1",
				"type": "core/section",
				"attributes": map[string]any{
					"padding":         "py-16",
					"backgroundColor": "#f8fafc",
				},
				"children": []map[string]any{
					{
						"id":   "h-1",
						"type": "core/heading",
						"attributes": map[string]any{
							"level":   1,
							"content": "About Go CMS Team",
							"align":   "center",
						},
					},
					{
						"id":   "p-1",
						"type": "core/paragraph",
						"attributes": map[string]any{
							"content": "Crafted with native Go performance.",
							"align":   "center",
						},
					},
					{
						"id":   "btn-1",
						"type": "core/button",
						"attributes": map[string]any{
							"text":    "Contact Us",
							"url":     "/contact",
							"variant": "primary",
						},
					},
				},
			},
		},
	}, "user-1")
	require.NoError(t, err)

	// Before publishing: /about-us should be 404 (draft only)
	req = httptest.NewRequest("GET", "/about-us", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Publish page
	publishedPage, err := contentSvc.PublishEntry(ctx, pagesType.UID, pageEntry.ID, "user-1")
	require.NoError(t, err)
	assert.Equal(t, content.StatusPublished, publishedPage.Status)

	// Now /about-us should render 200 with server-rendered blocks
	req = httptest.NewRequest("GET", "/about-us", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	body = w.Body.String()
	assert.Contains(t, body, "About Go CMS Team")
	assert.Contains(t, body, "Crafted with native Go performance.")
	assert.Contains(t, body, "cms-block-section")
	assert.Contains(t, body, "Contact Us")
	assert.Contains(t, body, "/contact")

	// Test 8: 404 Not Found Page
	req = httptest.NewRequest("GET", "/non-existent-page", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "404")
}

func TestRenderer_PreviewAuthentication(t *testing.T) {
	renderer, contentSvc, _, _ := setupTestRenderer(t)
	ctx := context.Background()

	// Create content type and a draft-only entry (not published)
	_, err := contentSvc.CreateContentType(ctx, content.CreateTypeRequest{
		Name: "Pages",
		UID:  "pages",
		Kind: content.KindCollection,
		Fields: []content.ContentField{
			{Name: "Title", FieldKey: "title", FieldType: fields.TypeShortText, Configuration: fields.ValidationRules{Required: true}},
		},
	})
	require.NoError(t, err)

	draftEntry, err := contentSvc.CreateEntry(ctx, "pages", map[string]any{
		"title": "Unpublished Secret Draft",
	}, "user-1")
	require.NoError(t, err)

	router := chi.NewRouter()
	renderer.MountWebRoutes(router)

	// 1. Unauthenticated request to draft item -> 404 Not Found
	req := httptest.NewRequest("GET", "/pages/"+draftEntry.Slug, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 2. Unauthenticated request with ?preview=true -> Still 404 Not Found
	req = httptest.NewRequest("GET", "/pages/"+draftEntry.Slug+"?preview=true", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// 3. Authenticated admin request with ?preview=true -> 200 OK + Preview Banner
	adminUser := &auth.User{
		ID:       "admin-123",
		Username: "editor",
		Status:   auth.StatusActive,
		Roles: []auth.Role{
			{
				Slug: "admin",
				Permissions: []auth.Permission{
					{Action: "read", Resource: "content"},
				},
			},
		},
	}
	req = httptest.NewRequest("GET", "/pages/"+draftEntry.Slug+"?preview=true", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, adminUser))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Unpublished Secret Draft")
	assert.Contains(t, body, "PREVIEW MODE")
}
