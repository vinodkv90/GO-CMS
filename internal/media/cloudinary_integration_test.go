package media_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/config"
	"github.com/go-cms/cms/internal/media"
	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/storage/cloudinary"
	"github.com/go-cms/cms/internal/storage/local"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaService_CloudinaryLifecycleSwitchAndReplace(t *testing.T) {
	var failDestroy atomic.Bool
	var destroyedIDs []string
	var renamedPairs []string

	mockCloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1_1/demo-cloud/ping":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload"):
			require.NoError(t, r.ParseMultipartForm(4<<20))
			pid := r.FormValue("public_id")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"asset_id":      "cid-" + pid,
				"public_id":     pid,
				"version":       17000123,
				"format":        "png",
				"resource_type": "image",
				"type":          "upload",
				"width":         320,
				"height":        240,
				"bytes":         120,
				"secure_url":    "https://res.cloudinary.com/demo-cloud/image/upload/v17000123/" + pid + ".png",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rename"):
			require.NoError(t, r.ParseForm())
			from := r.FormValue("from_public_id")
			to := r.FormValue("to_public_id")
			renamedPairs = append(renamedPairs, from+"->"+to)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"public_id":     to,
				"version":       17000124,
				"format":        "png",
				"resource_type": "image",
				"type":          "upload",
				"secure_url":    "https://res.cloudinary.com/demo-cloud/image/upload/v17000124/" + to + ".png",
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/destroy"):
			if failDestroy.Load() {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "cloudinary transient failure"}})
				return
			}
			require.NoError(t, r.ParseForm())
			destroyedIDs = append(destroyedIDs, r.FormValue("public_id"))
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockCloud.Close()

	tempDir := t.TempDir()
	localProv, err := local.New(tempDir, "/uploads")
	require.NoError(t, err)

	cloudProv, err := cloudinary.New(cloudinary.Config{
		CloudName:           "demo-cloud",
		APIKey:              "key-1",
		APISecret:           "sec-1",
		RootFolder:          "cms",
		DeliveryType:        "upload",
		DefaultResourceType: "auto",
		UseSecureURL:        true,
		OptimizationEnabled: true,
		DefaultFormat:       "auto",
		DefaultQuality:      "auto",
		APIBaseURL:          mockCloud.URL,
		DeliveryBaseURL:     mockCloud.URL,
		AllowInsecureHTTP:   true,
	})
	require.NoError(t, err)

	sw := storage.NewSwappable(localProv)
	repo := media.NewMemoryRepository()
	cfg := config.DefaultConfig()
	svc := media.NewService(repo, sw, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := media.NewHandler(svc, cfg)

	router := chi.NewRouter()
	router.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api)
	})
	handler.MountFileServer(router)

	ctx := context.Background()
	admin := createAuthenticatedAdminUser()
	pngBytes := createTestPNG(320, 240)

	// 1. Upload local asset first
	localAsset, err := svc.Upload(ctx, bytes.NewReader(pngBytes), "legacy-local.png", int64(len(pngBytes)), "Local Alt", "Local Cap")
	require.NoError(t, err)
	assert.Equal(t, "local", localAsset.StorageProvider)
	assert.True(t, strings.HasPrefix(localAsset.URL, "/uploads/"))

	// 2. Switch active storage provider to Cloudinary
	sw.Swap(cloudProv)

	// Verify capabilities endpoint reflects Cloudinary
	reqCap := httptest.NewRequest(http.MethodGet, "/api/v1/media/capabilities", nil)
	reqCap = reqCap.WithContext(context.WithValue(reqCap.Context(), auth.UserContextKey, admin))
	recCap := httptest.NewRecorder()
	router.ServeHTTP(recCap, reqCap)
	require.Equal(t, http.StatusOK, recCap.Code)
	assert.Contains(t, recCap.Body.String(), `"supports_hls":false`)
	assert.Contains(t, recCap.Body.String(), `"supports_transforms":true`)

	// Verify HLS zip upload returns clean 400 when Cloudinary is active
	var hlsBody bytes.Buffer
	hlsMW := multipart.NewWriter(&hlsBody)
	hlsPart, _ := hlsMW.CreateFormFile("file", "stream.zip")
	_, _ = hlsPart.Write([]byte("not-even-needed"))
	_ = hlsMW.Close()
	reqHLS := httptest.NewRequest(http.MethodPost, "/api/v1/media/hls", &hlsBody)
	reqHLS.Header.Set("Content-Type", hlsMW.FormDataContentType())
	reqHLS = reqHLS.WithContext(context.WithValue(reqHLS.Context(), auth.UserContextKey, admin))
	recHLS := httptest.NewRecorder()
	router.ServeHTTP(recHLS, reqHLS)
	assert.Equal(t, http.StatusBadRequest, recHLS.Code)
	assert.Contains(t, recHLS.Body.String(), "not supported")

	// Verify legacy local asset STILL resolves its /uploads URL and is STILL servable while Cloudinary is active!
	fetchedLocal, err := svc.GetAsset(ctx, localAsset.ID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(fetchedLocal.URL, "/uploads/"))

	reqServeLocal := httptest.NewRequest(http.MethodGet, fetchedLocal.URL, nil)
	recServeLocal := httptest.NewRecorder()
	router.ServeHTTP(recServeLocal, reqServeLocal)
	assert.Equal(t, http.StatusOK, recServeLocal.Code)

	// 3. Upload asset to Cloudinary
	folder, err := svc.CreateFolder(ctx, "Marketing", nil, nil)
	require.NoError(t, err)
	cloudAsset, err := svc.Upload(ctx, bytes.NewReader(pngBytes), "banner.png", int64(len(pngBytes)), "Hero Banner", "Campaign", &folder.ID)
	require.NoError(t, err)
	assert.Equal(t, "cloudinary", cloudAsset.StorageProvider)
	assert.Contains(t, cloudAsset.URL, "demo-cloud/image/upload")
	assert.Contains(t, cloudAsset.URL, "f_auto,q_auto")
	assert.NotEmpty(t, cloudAsset.Metadata.Thumbnails["small"])
	assert.Contains(t, cloudAsset.Metadata.Thumbnails["small"], "w_245")
	require.NotNil(t, cloudAsset.Metadata.Custom["cloudinary"])

	// Transform endpoint returns validated transform URL & rejects injection
	reqTx := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+cloudAsset.ID+"/transform?w=500&crop=fill&format=webp", nil)
	reqTx = reqTx.WithContext(context.WithValue(reqTx.Context(), auth.UserContextKey, admin))
	recTx := httptest.NewRecorder()
	router.ServeHTTP(recTx, reqTx)
	assert.Equal(t, http.StatusOK, recTx.Code)
	assert.Contains(t, recTx.Body.String(), "c_fill,w_500,f_webp")

	reqBadTx := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+cloudAsset.ID+"/transform?crop=fill,l_evil", nil)
	reqBadTx = reqBadTx.WithContext(context.WithValue(reqBadTx.Context(), auth.UserContextKey, admin))
	recBadTx := httptest.NewRecorder()
	router.ServeHTTP(recBadTx, reqBadTx)
	assert.Equal(t, http.StatusBadRequest, recBadTx.Code)

	// Move Cloudinary asset back to root folder triggers Cloudinary rename
	movedCloud, err := svc.MoveAsset(ctx, cloudAsset.ID, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, renamedPairs)
	assert.Nil(t, movedCloud.FolderID)

	// 4. Switch active storage back to Local; verify Cloudinary asset remains intact, movable & safely deletable!
	sw.Swap(localProv)
	fetchedCloudAfterSwitch, err := svc.GetAsset(ctx, cloudAsset.ID)
	require.NoError(t, err)
	assert.Contains(t, fetchedCloudAfterSwitch.URL, "demo-cloud/image/upload")

	// Operational Cloudinary destroy failure must NOT delete DB record
	failDestroy.Store(true)
	err = svc.DeleteAsset(ctx, cloudAsset.ID)
	assert.Error(t, err)
	_, stillThereErr := svc.GetAsset(ctx, cloudAsset.ID)
	assert.NoError(t, stillThereErr)

	// Succeeding destroy deletes DB record cleanly
	failDestroy.Store(false)
	require.NoError(t, svc.DeleteAsset(ctx, cloudAsset.ID))
	assert.NotEmpty(t, destroyedIDs)

	// 5. Replace legacy local asset while Cloudinary is active -> preserves ID and removes old local file!
	sw.Swap(cloudProv)
	oldLocalKey := localAsset.StorageKey
	existsBefore, _ := localProv.Exists(ctx, oldLocalKey)
	assert.True(t, existsBefore)

	var repBody bytes.Buffer
	repMW := multipart.NewWriter(&repBody)
	repPart, _ := repMW.CreateFormFile("file", "replaced-banner.png")
	_, _ = repPart.Write(pngBytes)
	_ = repMW.Close()

	reqRep := httptest.NewRequest(http.MethodPost, "/api/v1/media/"+localAsset.ID+"/replace", &repBody)
	reqRep.Header.Set("Content-Type", repMW.FormDataContentType())
	reqRep = reqRep.WithContext(context.WithValue(reqRep.Context(), auth.UserContextKey, admin))
	recRep := httptest.NewRecorder()
	router.ServeHTTP(recRep, reqRep)
	require.Equal(t, http.StatusOK, recRep.Code)

	var repResp struct {
		Data media.MediaAsset `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recRep.Body.Bytes(), &repResp))
	assert.Equal(t, localAsset.ID, repResp.Data.ID)
	assert.Equal(t, "cloudinary", repResp.Data.StorageProvider)
	assert.Contains(t, repResp.Data.URL, "demo-cloud/image/upload")

	existsAfter, _ := localProv.Exists(ctx, oldLocalKey)
	assert.False(t, existsAfter)
}

