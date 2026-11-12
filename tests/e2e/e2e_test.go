package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-cms/cms/internal/api"
	"github.com/go-cms/cms/internal/audit"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/content"
	"github.com/go-cms/cms/internal/database/interfaces"
	"github.com/go-cms/cms/internal/fields"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/plugins"
	"github.com/go-cms/cms/internal/storage/local"
	"github.com/go-cms/cms/internal/templates"
	"github.com/go-cms/cms/internal/themes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockE2EDB struct {
	caps interfaces.Capabilities
}

func (m *mockE2EDB) DriverName() string                    { return "postgres" }
func (m *mockE2EDB) Ping(ctx context.Context) error        { return nil }
func (m *mockE2EDB) Close() error                          { return nil }
func (m *mockE2EDB) Capabilities() interfaces.Capabilities { return m.caps }

func setupE2EServer(t *testing.T) (*api.Server, string) {
	tempDir := t.TempDir()
	uploadDir := filepath.Join(tempDir, "uploads")
	require.NoError(t, os.MkdirAll(uploadDir, 0755))

	cfg := config.DefaultConfig()
	cfg.Media.UploadDir = uploadDir
	cfg.Media.SignedURLs.SecretKey = "e2e-super-secret-hmac-key"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 1. Auth Service
	authRepo := auth.NewMemoryAuthRepository()
	authSvc := auth.NewService(authRepo, authRepo, authRepo, authRepo, cfg, logger)

	// Seed super admin
	adminUser, err := authSvc.SeedDefaultRolesAndAdmin(context.Background(), "admin", "admin@cms.test", "Admin@CMS2026!")
	require.NoError(t, err)
	require.NotNil(t, adminUser)

	// 2. Content Service
	contentRepo := content.NewMemoryContentRepository()
	fieldRegistry := fields.NewRegistry()
	eventBus := content.NewEventBus()
	contentSvc := content.NewService(contentRepo, contentRepo, contentRepo, fieldRegistry, eventBus, logger)

	// 3. Storage and Media
	storageProvider, err := local.New(uploadDir, "/uploads")
	require.NoError(t, err)
	mediaRepo := media.NewMemoryRepository()
	mediaSvc := media.NewService(mediaRepo, storageProvider, cfg, logger)

	// 4. Themes and Renderer
	themesDir := filepath.Join("..", "..", "themes")
	if _, err := os.Stat(themesDir); err != nil {
		themesDir = "themes"
	}
	themeMgr, err := themes.NewManager(themesDir, "default", logger)
	var renderer *templates.Renderer
	if err == nil {
		renderer = templates.NewRenderer(cfg, themeMgr, contentSvc, logger)
	}

	// 5. Plugins Manager
	pluginMgr := plugins.NewManager(filepath.Join(tempDir, "plugins"), eventBus, fieldRegistry, nil, logger)

	// 6. Assemble Server
	db := &mockE2EDB{caps: interfaces.NewCapabilitySet(interfaces.CapTransactions)}
	srv := api.NewServer(cfg, db, authSvc, contentSvc, mediaSvc, pluginMgr, renderer, logger)

	return srv, uploadDir
}

func TestE2E_FullPlatformLifecycle(t *testing.T) {
	srv, _ := setupE2EServer(t)

	// =========================================================================
	// STEP 1: Admin Authentication & Login
	// =========================================================================
	loginPayload := `{"username":"admin","password":"Admin@CMS2026!"}`
	reqLogin := httptest.NewRequest("POST", "/api/v1/auth/login", bytes.NewBufferString(loginPayload))
	reqLogin.Header.Set("Content-Type", "application/json")
	wLogin := httptest.NewRecorder()
	srv.Router().ServeHTTP(wLogin, reqLogin)

	require.Equal(t, http.StatusOK, wLogin.Code)
	var loginResp struct {
		Token string `json:"token"`
		User  struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	}
	err := json.Unmarshal(wLogin.Body.Bytes(), &loginResp)
	require.NoError(t, err)
	require.NotEmpty(t, loginResp.Token)
	assert.Equal(t, "admin", loginResp.User.Username)
	token := loginResp.Token

	// =========================================================================
	// STEP 2: Authenticated /auth/me Profile Check
	// =========================================================================
	reqMe := httptest.NewRequest("GET", "/api/v1/auth/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+token)
	wMe := httptest.NewRecorder()
	srv.Router().ServeHTTP(wMe, reqMe)
	require.Equal(t, http.StatusOK, wMe.Code)

	// =========================================================================
	// STEP 3: Content Type Builder (Create "articles" collection)
	// =========================================================================
	typePayload := `{
		"name": "Articles",
		"uid": "articles",
		"kind": "collection",
		"fields": [
			{"name": "Title", "field_key": "title", "field_type": "short_text", "configuration": {"required": true}},
			{"name": "Content", "field_key": "content", "field_type": "rich_text", "configuration": {}}
		]
	}`
	reqType := httptest.NewRequest("POST", "/api/v1/content-types", bytes.NewBufferString(typePayload))
	reqType.Header.Set("Authorization", "Bearer "+token)
	reqType.Header.Set("Content-Type", "application/json")
	wType := httptest.NewRecorder()
	srv.Router().ServeHTTP(wType, reqType)
	require.Equal(t, http.StatusCreated, wType.Code)

	// =========================================================================
	// STEP 4: Content Entry Creation (Draft)
	// =========================================================================
	entryPayload := `{
		"data": {
			"title": "Scalable Microservices with Go",
			"content": "<p>Go provides native goroutines and lightweight concurrency for enterprise backends.</p>"
		}
	}`
	reqEntry := httptest.NewRequest("POST", "/api/v1/content/articles", bytes.NewBufferString(entryPayload))
	reqEntry.Header.Set("Authorization", "Bearer "+token)
	reqEntry.Header.Set("Content-Type", "application/json")
	wEntry := httptest.NewRecorder()
	srv.Router().ServeHTTP(wEntry, reqEntry)
	require.Equal(t, http.StatusCreated, wEntry.Code)

	var createdEntry content.ContentEntry
	err = json.Unmarshal(wEntry.Body.Bytes(), &createdEntry)
	require.NoError(t, err)
	articleID := createdEntry.ID
	assert.Equal(t, content.StatusDraft, createdEntry.Status)
	assert.Equal(t, 1, createdEntry.DraftVersion)

	// =========================================================================
	// STEP 5: Publish Content Entry
	// =========================================================================
	reqPublish := httptest.NewRequest("POST", "/api/v1/content/articles/"+articleID+"/publish", nil)
	reqPublish.Header.Set("Authorization", "Bearer "+token)
	wPublish := httptest.NewRecorder()
	srv.Router().ServeHTTP(wPublish, reqPublish)
	require.Equal(t, http.StatusOK, wPublish.Code)

	var pubEntry content.ContentEntry
	err = json.Unmarshal(wPublish.Body.Bytes(), &pubEntry)
	require.NoError(t, err)
	assert.Equal(t, content.StatusPublished, pubEntry.Status)
	assert.Equal(t, 1, pubEntry.PublishedVersion)

	// =========================================================================
	// STEP 6: Full-Text Search Across Published Content
	// =========================================================================
	reqSearch := httptest.NewRequest("GET", "/api/v1/search?q=microservices&type=articles", nil)
	wSearch := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSearch, reqSearch)
	require.Equal(t, http.StatusOK, wSearch.Code)

	var searchResp struct {
		Data []struct {
			Document struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"document"`
			Score float64 `json:"score"`
		} `json:"data"`
	}
	err = json.Unmarshal(wSearch.Body.Bytes(), &searchResp)
	require.NoError(t, err)
	require.Len(t, searchResp.Data, 1)
	assert.Equal(t, articleID, searchResp.Data[0].Document.ID)
	assert.Equal(t, "Scalable Microservices with Go", searchResp.Data[0].Document.Title)

	// =========================================================================
	// STEP 7: Media Upload & Signed URL Generation
	// =========================================================================
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "diagram.png")
	require.NoError(t, err)
	// Write dummy PNG header bytes
	_, _ = part.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x10\x00\x00\x00\x10\x08\x06\x00\x00\x00\x1f\xf3\xffa"))
	_ = writer.WriteField("alt_text", "System Diagram")
	_ = writer.Close()

	reqUpload := httptest.NewRequest("POST", "/api/v1/media", body)
	reqUpload.Header.Set("Authorization", "Bearer "+token)
	reqUpload.Header.Set("Content-Type", writer.FormDataContentType())
	wUpload := httptest.NewRecorder()
	srv.Router().ServeHTTP(wUpload, reqUpload)
	require.Equal(t, http.StatusCreated, wUpload.Code)

	var uploadResp struct {
		Data struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"data"`
	}
	err = json.Unmarshal(wUpload.Body.Bytes(), &uploadResp)
	require.NoError(t, err)
	mediaID := uploadResp.Data.ID

	// Generate HMAC-SHA256 Signed URL
	reqSigned := httptest.NewRequest("GET", "/api/v1/media/"+mediaID+"/signed-url?ttl=1800", nil)
	reqSigned.Header.Set("Authorization", "Bearer "+token)
	wSigned := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSigned, reqSigned)
	require.Equal(t, http.StatusOK, wSigned.Code)

	var signedResp struct {
		SignedURL string `json:"signed_url"`
		ExpiresIn int    `json:"expires_in"`
	}
	err = json.Unmarshal(wSigned.Body.Bytes(), &signedResp)
	require.NoError(t, err)
	assert.Contains(t, signedResp.SignedURL, "signature=")
	assert.Contains(t, signedResp.SignedURL, "expires=")

	// Verify accessing file with signed URL succeeds
	reqFile := httptest.NewRequest("GET", signedResp.SignedURL, nil)
	wFile := httptest.NewRecorder()
	srv.Router().ServeHTTP(wFile, reqFile)
	assert.Equal(t, http.StatusOK, wFile.Code)
	assert.Equal(t, "image/png", wFile.Header().Get("Content-Type"))

	// =========================================================================
	// STEP 8: Multi-Tenancy & Default Workspace Lookup
	// =========================================================================
	reqTenants := httptest.NewRequest("GET", "/api/v1/tenants", nil)
	reqTenants.Header.Set("Authorization", "Bearer "+token)
	reqTenants.Header.Set("X-Tenant-ID", "default")
	wTenants := httptest.NewRecorder()
	srv.Router().ServeHTTP(wTenants, reqTenants)
	require.Equal(t, http.StatusOK, wTenants.Code)

	// =========================================================================
	// STEP 9: System Audit Logs Query
	// =========================================================================
	// Record an audit log entry
	_, err = srv.Audit().Record(context.Background(), audit.RecordParams{
		ActorID:   "admin",
		ActorType: "user",
		Action:    "publish",
		Resource:  "content:articles",
		TargetID:  articleID,
	})
	require.NoError(t, err)

	reqAudit := httptest.NewRequest("GET", "/api/v1/audit-logs", nil)
	reqAudit.Header.Set("Authorization", "Bearer "+token)
	wAudit := httptest.NewRecorder()
	srv.Router().ServeHTTP(wAudit, reqAudit)
	require.Equal(t, http.StatusOK, wAudit.Code)

	// =========================================================================
	// STEP 10: Prometheus Metrics & API Docs Scrape
	// =========================================================================
	reqMetrics := httptest.NewRequest("GET", "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	srv.Router().ServeHTTP(wMetrics, reqMetrics)
	assert.Equal(t, http.StatusOK, wMetrics.Code)
	assert.Contains(t, wMetrics.Body.String(), "cms_http_requests_total")

	reqDocs := httptest.NewRequest("GET", "/docs", nil)
	wDocs := httptest.NewRecorder()
	srv.Router().ServeHTTP(wDocs, reqDocs)
	assert.Equal(t, http.StatusOK, wDocs.Code)
	assert.Contains(t, wDocs.Body.String(), "Go CMS - API Documentation")

	// =========================================================================
	// STEP 11: Server-Side Template Rendering & SEO Sitemap
	// =========================================================================
	reqHome := httptest.NewRequest("GET", "/", nil)
	wHome := httptest.NewRecorder()
	srv.Router().ServeHTTP(wHome, reqHome)
	assert.Equal(t, http.StatusOK, wHome.Code)
	assert.Contains(t, wHome.Body.String(), "Scalable Microservices with Go")

	reqSitemap := httptest.NewRequest("GET", "/sitemap.xml", nil)
	wSitemap := httptest.NewRecorder()
	srv.Router().ServeHTTP(wSitemap, reqSitemap)
	assert.Equal(t, http.StatusOK, wSitemap.Code)
	assert.Contains(t, wSitemap.Body.String(), "<urlset")

	// =========================================================================
	// STEP 12: Content Localization & Background Jobs Inspection
	// =========================================================================
	reqLocales := httptest.NewRequest("GET", "/api/v1/locales", nil)
	wLocales := httptest.NewRecorder()
	srv.Router().ServeHTTP(wLocales, reqLocales)
	assert.Equal(t, http.StatusOK, wLocales.Code)
	assert.Contains(t, wLocales.Body.String(), "en")

	reqJobStats := httptest.NewRequest("GET", "/api/v1/jobs/stats", nil)
	reqJobStats.Header.Set("Authorization", "Bearer "+token)
	wJobStats := httptest.NewRecorder()
	srv.Router().ServeHTTP(wJobStats, reqJobStats)
	assert.Equal(t, http.StatusOK, wJobStats.Code)
	assert.Contains(t, wJobStats.Body.String(), "completed")
}
