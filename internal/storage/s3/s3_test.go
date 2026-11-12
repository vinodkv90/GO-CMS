package s3_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-cms/cms/internal/storage"
	"github.com/go-cms/cms/internal/storage/s3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockObject struct {
	data        []byte
	contentType string
}

func newMockS3Server(t *testing.T, bucket string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	objects := map[string]mockObject{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify SigV4 headers on every call
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") ||
			!strings.Contains(auth, "Credential=AKIA_TEST/") ||
			r.Header.Get("X-Amz-Date") == "" ||
			r.Header.Get("X-Amz-Content-Sha256") == "" {
			http.Error(w, "Missing AWS SigV4 signature", http.StatusForbidden)
			return
		}

		prefix := "/" + bucket
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")

		mu.Lock()
		defer mu.Unlock()

		// ListObjectsV2
		if r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2" {
			p := r.URL.Query().Get("prefix")
			type contents struct {
				Key string `xml:"Key"`
			}
			type result struct {
				XMLName     xml.Name   `xml:"ListBucketResult"`
				IsTruncated bool       `xml:"IsTruncated"`
				Contents    []contents `xml:"Contents"`
			}
			res := result{}
			for k := range objects {
				if p == "" || strings.HasPrefix(k, p) {
					res.Contents = append(res.Contents, contents{Key: k})
				}
			}
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(res)
			return
		}

		switch r.Method {
		case http.MethodPut:
			if copySrc := r.Header.Get("X-Amz-Copy-Source"); copySrc != "" {
				srcKey := strings.TrimPrefix(copySrc, prefix+"/")
				obj, ok := objects[srcKey]
				if !ok {
					http.NotFound(w, r)
					return
				}
				objects[key] = obj
				w.WriteHeader(http.StatusOK)
				return
			}
			body, _ := io.ReadAll(r.Body)
			objects[key] = mockObject{data: body, contentType: r.Header.Get("Content-Type")}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			obj, ok := objects[key]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", obj.contentType)
			_, _ = w.Write(obj.data)
		case http.MethodHead:
			if _, ok := objects[key]; !ok {
				http.NotFound(w, r)
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(objects, key)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))
}

func TestS3Provider_RealHTTPAndSigV4Operations(t *testing.T) {
	srv := newMockS3Server(t, "cms-media")
	defer srv.Close()

	provider, err := s3.New(s3.Config{
		ProviderName:    "minio",
		Bucket:          "cms-media",
		Region:          "us-east-1",
		Endpoint:        srv.URL,
		AccessKeyID:     "AKIA_TEST",
		SecretAccessKey: "super-secret-signing-key",
		UseSSL:          false,
	})
	require.NoError(t, err)
	assert.Equal(t, "minio", provider.Name())

	ctx := context.Background()
	require.NoError(t, provider.CheckHealth(ctx))

	// Save & Exists & Open
	require.NoError(t, provider.Save(ctx, "2026/11/hero.png", bytes.NewReader([]byte("PNGDATA")), 7, "image/png"))
	exists, err := provider.Exists(ctx, "2026/11/hero.png")
	require.NoError(t, err)
	assert.True(t, exists)

	rc, err := provider.Open(ctx, "2026/11/hero.png")
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	rc.Close()
	require.NoError(t, err)
	assert.Equal(t, []byte("PNGDATA"), got)

	u, err := provider.URL(ctx, "2026/11/hero.png")
	require.NoError(t, err)
	assert.Equal(t, srv.URL+"/cms-media/2026/11/hero.png", u)

	// Move object
	require.NoError(t, provider.Move(ctx, "2026/11/hero.png", "archive/hero.png"))
	oldExists, _ := provider.Exists(ctx, "2026/11/hero.png")
	newExists, _ := provider.Exists(ctx, "archive/hero.png")
	assert.False(t, oldExists)
	assert.True(t, newExists)

	// DeletePrefix
	require.NoError(t, provider.Save(ctx, "archive/thumb.png", bytes.NewReader([]byte("T")), 1, "image/png"))
	require.NoError(t, provider.DeletePrefix(ctx, "archive/"))
	afterDel, _ := provider.Exists(ctx, "archive/hero.png")
	assert.False(t, afterDel)

	// Traversal rejection & 404
	err = provider.Save(ctx, "../../etc/passwd", bytes.NewReader([]byte("x")), 1, "text/plain")
	assert.True(t, errors.Is(err, storage.ErrPathTraversal))
	_, err = provider.Open(ctx, "does-not-exist.png")
	assert.True(t, errors.Is(err, storage.ErrFileNotFound))
}

