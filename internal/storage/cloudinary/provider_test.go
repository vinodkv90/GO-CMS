package cloudinary

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-cms/cms/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignatureAndTransforms(t *testing.T) {
	sig := SignParams(map[string]string{
		"public_id":  "sample",
		"timestamp":  "1315060510",
		"api_key":    "should-be-excluded",
		"signature":  "should-be-excluded",
		"empty_skip": "",
	}, "abcd", false)
	// Verified deterministic SHA-1 for "public_id=sample&timestamp=1315060510abcd"
	assert.Len(t, sig, 40)
	assert.Equal(t, sig, SignParams(map[string]string{
		"timestamp": "1315060510",
		"public_id": "sample",
	}, "abcd", false))

	sha256Sig := SignParams(map[string]string{
		"public_id": "sample",
		"timestamp": "1315060510",
	}, "abcd", true)
	assert.Len(t, sha256Sig, 64)

	urlSig := SignDeliveryPath("c_fill,w_400/go-cms/hero.jpg", "abcd")
	assert.True(t, strings.HasPrefix(urlSig, "s--"))
	assert.True(t, strings.HasSuffix(urlSig, "--"))
	assert.Len(t, urlSig, 13)

	seg, err := BuildTransformSegment(storage.ImageTransformOptions{
		Width:   800,
		Height:  600,
		Crop:    "fill",
		Gravity: "auto",
		Format:  "auto",
		Quality: "auto",
		DPR:     "2.0",
	})
	require.NoError(t, err)
	assert.Equal(t, "c_fill,g_auto,w_800,h_600,dpr_2.0,f_auto,q_auto", seg)

	_, err = BuildTransformSegment(storage.ImageTransformOptions{Crop: "fill,l_malicious"})
	assert.Error(t, err)
	_, err = BuildTransformSegment(storage.ImageTransformOptions{Quality: "999"})
	assert.Error(t, err)
}

func TestConfigValidationAndRedaction(t *testing.T) {
	cfg := Config{
		CloudName: "demo-cloud",
		APIKey:    "12345",
		APISecret: "super-secret-value",
	}
	assert.NotContains(t, cfg.String(), "super-secret-value")
	assert.NotContains(t, cfg.String(), "12345")

	bad := Config{CloudName: "bad/cloud", APIKey: "1", APISecret: "2"}
	assert.Error(t, bad.NormalizeAndValidate())

	badFolder := Config{CloudName: "demo", APIKey: "1", APISecret: "2", RootFolder: "../escape"}
	assert.Error(t, badFolder.NormalizeAndValidate())

	badPreset := Config{CloudName: "demo", APIKey: "1", APISecret: "2", DefaultResourceType: "executable"}
	assert.Error(t, badPreset.NormalizeAndValidate())
}

func TestProviderMockLifecycle(t *testing.T) {
	var seenPingAuth bool
	var seenUploadSignature string
	var seenPublicID string
	var deletedPrefix string
	var renamedTo string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1_1/test-cloud/ping":
			user, pass, ok := r.BasicAuth()
			seenPingAuth = ok && user == "key-1" && pass == "sec-1"
			if !seenPingAuth {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/upload"):
			require.NoError(t, r.ParseMultipartForm(4<<20))
			seenUploadSignature = r.FormValue("signature")
			seenPublicID = r.FormValue("public_id")
			resp := map[string]any{
				"asset_id":      "asset-123",
				"public_id":     seenPublicID,
				"version":       17000001,
				"format":        "png",
				"resource_type": "image",
				"type":          "upload",
				"width":         640,
				"height":        480,
				"bytes":         12,
				"secure_url":    "https://res.cloudinary.com/test-cloud/image/upload/v17000001/" + seenPublicID + ".png",
				"url":           "http://res.cloudinary.com/test-cloud/image/upload/v17000001/" + seenPublicID + ".png",
			}
			_ = json.NewEncoder(w).Encode(resp)

		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1_1/test-cloud/resources/image/upload/"):
			_ = json.NewEncoder(w).Encode(map[string]string{"public_id": seenPublicID})

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rename"):
			require.NoError(t, r.ParseForm())
			renamedTo = r.FormValue("to_public_id")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"public_id":     renamedTo,
				"version":       17000002,
				"format":        "png",
				"resource_type": "image",
				"type":          "upload",
				"secure_url":    "https://res.cloudinary.com/test-cloud/image/upload/v17000002/" + renamedTo + ".png",
			})

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/destroy"):
			require.NoError(t, r.ParseForm())
			if r.FormValue("public_id") == "go-cms/missing" {
				_ = json.NewEncoder(w).Encode(map[string]string{"result": "not found"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "ok"})

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1_1/test-cloud/resources/"):
			deletedPrefix = r.URL.Query().Get("prefix")
			_ = json.NewEncoder(w).Encode(map[string]any{"deleted": map[string]string{}})

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/test-cloud/image/upload/"):
			_, _ = w.Write([]byte("fake-png-bytes"))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, err := New(Config{
		CloudName:           "test-cloud",
		APIKey:              "key-1",
		APISecret:           "sec-1",
		RootFolder:          "go-cms",
		DeliveryType:        "upload",
		DefaultResourceType: "auto",
		UseSecureURL:        true,
		OptimizationEnabled: true,
		DefaultFormat:       "auto",
		DefaultQuality:      "auto",
		APIBaseURL:          srv.URL,
		DeliveryBaseURL:     srv.URL,
		AllowInsecureHTTP:   true,
	})
	require.NoError(t, err)

	assert.Equal(t, "cloudinary", p.Name())
	assert.False(t, p.Capabilities().SupportsHLS)
	assert.True(t, p.Capabilities().SupportsTransforms)

	// 1. Ping
	require.NoError(t, p.CheckHealth(context.Background()))
	assert.True(t, seenPingAuth)

	// 2. Upload
	res, err := p.SaveWithOptions(context.Background(), "2026/11/hero.png", strings.NewReader("fake-png-bytes"), 14, "image/png", storage.UploadOptions{
		Filename:  "hero.png",
		Overwrite: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, seenUploadSignature)
	assert.Equal(t, "go-cms/2026/11/hero", seenPublicID)
	assert.Equal(t, "asset-123", res.AssetID)
	assert.Equal(t, "image", res.ResourceType)
	assert.Contains(t, res.Variants["small"], "w_245")
	assert.Contains(t, res.Variants["small"], "f_auto")

	// 3. Path traversal rejection
	err = p.Save(context.Background(), "../../evil.png", strings.NewReader("x"), 1, "image/png")
	assert.ErrorIs(t, err, storage.ErrPathTraversal)

	// 4. Exists & Open
	exists, err := p.Exists(context.Background(), "2026/11/hero.png")
	require.NoError(t, err)
	assert.True(t, exists)

	rc, err := p.OpenAsset(context.Background(), storage.AssetRef{
		Key:          "2026/11/hero.png",
		PublicID:     res.PublicID,
		ResourceType: "image",
		DeliveryType: "upload",
		Version:      res.Version,
		Format:       "png",
	})
	require.NoError(t, err)
	data, err := io.ReadAll(rc)
	rc.Close()
	require.NoError(t, err)
	assert.Equal(t, "fake-png-bytes", string(data))

	// 5. Transform & Signed URL
	txURL, err := p.TransformURL(context.Background(), storage.AssetRef{
		Key:          "2026/11/hero.png",
		PublicID:     res.PublicID,
		ResourceType: "image",
		DeliveryType: "upload",
		Version:      res.Version,
		Format:       "png",
	}, storage.ImageTransformOptions{Width: 400, Crop: "fill", Format: "webp"})
	require.NoError(t, err)
	assert.Contains(t, txURL, "/c_fill,w_400,f_webp/")

	signedURL, err := p.SignedURL(context.Background(), storage.AssetRef{
		Key:          "2026/11/hero.png",
		PublicID:     res.PublicID,
		ResourceType: "image",
		DeliveryType: "authenticated",
		Version:      res.Version,
		Format:       "png",
	}, 300)
	require.NoError(t, err)
	assert.Contains(t, signedURL, "/image/authenticated/s--")

	// 6. Rename / Move
	moved, err := p.MoveAsset(context.Background(), storage.AssetRef{
		Key:          "2026/11/hero.png",
		PublicID:     res.PublicID,
		ResourceType: "image",
		DeliveryType: "upload",
	}, "articles/hero.png")
	require.NoError(t, err)
	assert.Equal(t, "go-cms/articles/hero", renamedTo)
	assert.Equal(t, "articles/hero.png", moved.StorageKey)

	// 7. Delete & DeletePrefix
	require.NoError(t, p.Delete(context.Background(), "articles/hero.png"))
	err = p.Delete(context.Background(), "missing.png")
	assert.ErrorIs(t, err, storage.ErrFileNotFound)

	require.NoError(t, p.DeletePrefix(context.Background(), "articles/"))
	assert.Equal(t, "go-cms/articles/", deletedPrefix)
}

func TestProviderLiveIntegration(t *testing.T) {
	cloudName := os.Getenv("CLOUDINARY_CLOUD_NAME")
	apiKey := os.Getenv("CLOUDINARY_API_KEY")
	apiSecret := os.Getenv("CLOUDINARY_API_SECRET")
	if cloudName == "" || apiKey == "" || apiSecret == "" {
		t.Skip("Skipping live Cloudinary integration test: CLOUDINARY_* env vars not set")
	}

	p, err := New(Config{
		CloudName:    cloudName,
		APIKey:       apiKey,
		APISecret:    apiSecret,
		RootFolder:   "go-cms-test",
		UseSecureURL: true,
	})
	require.NoError(t, err)
	require.NoError(t, p.CheckHealth(context.Background()))
}

