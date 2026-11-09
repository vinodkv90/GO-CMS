package media_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-cms/cms/internal/auth"
	"github.com/go-cms/cms/internal/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestZip creates an in-memory zip archive from filename -> content map
func createTestZip(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestMediaFolder_Lifecycle(t *testing.T) {
	svc, repo, storageProvider, _ := setupTestMediaService(t, true)
	ctx := context.Background()

	// 1. Create Top-Level Folder
	rootFolder, err := svc.CreateFolder(ctx, "Blog", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, "Blog", rootFolder.Name)
	assert.Equal(t, "blog", rootFolder.Slug)
	assert.Equal(t, "blog", rootFolder.Path)
	assert.Equal(t, "blog/", rootFolder.StoragePath)

	// 2. Prevent Duplicate Folder Under Same Parent
	_, err = svc.CreateFolder(ctx, "Blog", nil, nil)
	assert.ErrorIs(t, err, media.ErrDuplicateFolder)

	// 3. Create Subfolder
	subFolder, err := svc.CreateFolder(ctx, "Articles", &rootFolder.ID, nil)
	require.NoError(t, err)
	assert.Equal(t, "Articles", subFolder.Name)
	assert.Equal(t, "articles", subFolder.Slug)
	assert.Equal(t, "blog/articles", subFolder.Path)
	assert.Equal(t, "blog/articles/", subFolder.StoragePath)

	// 4. Upload Media Directly into Subfolder
	content := []byte("fake image data")
	asset, err := svc.Upload(ctx, bytes.NewReader(content), "go-cms.jpg", int64(len(content)), "CMS Architecture", "Diagram", &subFolder.ID)
	require.NoError(t, err)
	require.NotNil(t, asset.FolderID)
	assert.Equal(t, subFolder.ID, *asset.FolderID)
	assert.True(t, strings.HasPrefix(asset.StorageKey, "blog/articles/"))

	// Verify file physically exists on storage provider
	exists, err := storageProvider.Exists(ctx, asset.StorageKey)
	require.NoError(t, err)
	assert.True(t, exists)

	// 5. Get Folder Tree
	tree, err := svc.GetFolderTree(ctx)
	require.NoError(t, err)
	require.Len(t, tree, 1)
	assert.Equal(t, "Blog", tree[0].Name)
	require.Len(t, tree[0].Children, 1)
	assert.Equal(t, "Articles", tree[0].Children[0].Name)
	assert.Equal(t, 1, tree[0].Children[0].AssetCount)

	// 6. Move Asset to Root
	movedAsset, err := svc.MoveAsset(ctx, asset.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, movedAsset.FolderID)
	assert.False(t, strings.HasPrefix(movedAsset.StorageKey, "blog/articles/"))

	// 7. Non-Recursive Deletion Rejected When Folder Has Children
	err = svc.DeleteFolder(ctx, rootFolder.ID, false)
	assert.ErrorIs(t, err, media.ErrFolderNotEmpty)

	// 8. Recursive Deletion Succeeds
	err = svc.DeleteFolder(ctx, rootFolder.ID, true)
	require.NoError(t, err)

	_, err = svc.GetFolder(ctx, rootFolder.ID)
	assert.ErrorIs(t, err, media.ErrFolderNotFound)
	_, err = svc.GetFolder(ctx, subFolder.ID)
	assert.ErrorIs(t, err, media.ErrFolderNotFound)
	_ = repo
}

func TestMediaFolder_MoveAndCircularCheck(t *testing.T) {
	svc, _, _, _ := setupTestMediaService(t, true)
	ctx := context.Background()

	folderA, err := svc.CreateFolder(ctx, "Folder A", nil, nil)
	require.NoError(t, err)

	folderB, err := svc.CreateFolder(ctx, "Folder B", &folderA.ID, nil)
	require.NoError(t, err)

	folderC, err := svc.CreateFolder(ctx, "Folder C", &folderB.ID, nil)
	require.NoError(t, err)

	// 1. Moving folder into itself rejected
	_, err = svc.MoveFolder(ctx, folderA.ID, &folderA.ID)
	assert.ErrorIs(t, err, media.ErrCircularFolder)

	// 2. Moving folder into its own descendant rejected (A into C)
	_, err = svc.MoveFolder(ctx, folderA.ID, &folderC.ID)
	assert.ErrorIs(t, err, media.ErrCircularFolder)

	// 3. Moving B to Root succeeds
	movedB, err := svc.MoveFolder(ctx, folderB.ID, nil)
	require.NoError(t, err)
	assert.Nil(t, movedB.ParentID)
	assert.Equal(t, "folder-b", movedB.Path)
	assert.Equal(t, "folder-b/", movedB.StoragePath)
}

func TestMediaFolder_Rename(t *testing.T) {
	svc, _, storageProvider, _ := setupTestMediaService(t, true)
	ctx := context.Background()

	parent, err := svc.CreateFolder(ctx, "Guides", nil, nil)
	require.NoError(t, err)

	child, err := svc.CreateFolder(ctx, "Drafts", &parent.ID, nil)
	require.NoError(t, err)

	// Upload asset into child
	content := []byte("guide content")
	asset, err := svc.Upload(ctx, bytes.NewReader(content), "doc.txt", int64(len(content)), "", "", &child.ID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(asset.StorageKey, "guides/drafts/"))

	// Rename child folder
	renamed, err := svc.RenameFolder(ctx, child.ID, "Published")
	require.NoError(t, err)
	assert.Equal(t, "Published", renamed.Name)
	assert.Equal(t, "published", renamed.Slug)
	assert.Equal(t, "guides/published", renamed.Path)
	assert.Equal(t, "guides/published/", renamed.StoragePath)

	// Verify asset in DB and storage has updated prefix
	updatedAsset, err := svc.GetAsset(ctx, asset.ID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(updatedAsset.StorageKey, "guides/published/"))

	exists, err := storageProvider.Exists(ctx, updatedAsset.StorageKey)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestHLSPackage_ValidationAndUpload(t *testing.T) {
	svc, _, storageProvider, _ := setupTestMediaService(t, true)
	ctx := context.Background()

	// 1. Valid HLS Package with Master Playlist, Variant Playlist, and Segments
	masterM3U8 := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=1280000,RESOLUTION=1280x720,CODECS="avc1.64001f,mp4a.40.2"
720p/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2560000,RESOLUTION=1920x1080,CODECS="avc1.640028,mp4a.40.2"
1080p/index.m3u8
`
	variant720p := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:9.009,
segment0.ts
#EXTINF:8.500,
segment1.ts
#EXT-X-ENDLIST
`
	variant1080p := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:10
#EXTINF:9.009,
segment0.ts
#EXT-X-ENDLIST
`
	validFiles := map[string]string{
		"master.m3u8":       masterM3U8,
		"720p/index.m3u8":   variant720p,
		"720p/segment0.ts":  "binary ts 720p 0",
		"720p/segment1.ts":  "binary ts 720p 1",
		"1080p/index.m3u8":  variant1080p,
		"1080p/segment0.ts": "binary ts 1080p 0",
	}

	zipBytes := createTestZip(t, validFiles)

	asset, err := svc.UploadHLSPackage(ctx, bytes.NewReader(zipBytes), "demo-stream.zip", int64(len(zipBytes)), "Demo Stream", nil)
	require.NoError(t, err)
	require.NotNil(t, asset)

	assert.Equal(t, "application/vnd.apple.mpegurl", asset.MIMEType)
	assert.Equal(t, "hls", asset.Metadata.Format)
	assert.True(t, asset.Metadata.Custom["is_hls"].(bool))
	assert.Equal(t, 6, int(asset.Metadata.Custom["total_files"].(int)))

	// Check that master playlist and segment files are saved in storage
	masterExists, err := storageProvider.Exists(ctx, asset.StorageKey)
	require.NoError(t, err)
	assert.True(t, masterExists)

	// 2. Reject Package Missing Referenced Segment
	brokenFiles := map[string]string{
		"master.m3u8": `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=500000
playlist.m3u8
`,
		"playlist.m3u8": `#EXTM3U
#EXTINF:10.0,
missing_segment.ts
#EXT-X-ENDLIST
`,
	}
	brokenZip := createTestZip(t, brokenFiles)
	_, err = svc.UploadHLSPackage(ctx, bytes.NewReader(brokenZip), "broken.zip", int64(len(brokenZip)), "Broken", nil)
	assert.ErrorIs(t, err, media.ErrHLSMissingSegment)

	// 3. Reject Package Missing Any Playlist
	noPlaylistFiles := map[string]string{
		"segment0.ts": "ts data",
		"readme.txt":  "text data",
	}
	noPlaylistZip := createTestZip(t, noPlaylistFiles)
	_, err = svc.UploadHLSPackage(ctx, bytes.NewReader(noPlaylistZip), "no-playlist.zip", int64(len(noPlaylistZip)), "No Playlist", nil)
	assert.ErrorIs(t, err, media.ErrInvalidHLSPackage)

	// 4. Reject Path Traversal Entry
	traversalFiles := map[string]string{
		"master.m3u8":   "#EXTM3U\n#EXT-X-ENDLIST",
		"../escape.txt": "evil",
	}
	traversalZip := createTestZip(t, traversalFiles)
	_, err = svc.UploadHLSPackage(ctx, bytes.NewReader(traversalZip), "evil.zip", int64(len(traversalZip)), "Evil", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "escapes archive root")
}

func TestHLS_SignedURLPlaylistRewriting(t *testing.T) {
	rawPlaylist := []byte(`#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=1280000,RESOLUTION=1280x720
720p/index.m3u8
#EXT-X-KEY:METHOD=AES-128,URI="crypt.key"
#EXTINF:10.0,
segment0.ts
#EXTINF:10.0,
http://cdn.external.com/segment1.ts
#EXT-X-ENDLIST
`)

	rawQuery := "signature=abc123sig&expires=1794200000&tenant=default"
	rewritten := media.RewriteHLSPlaylistWithSignedParams(rawPlaylist, rawQuery)
	result := string(rewritten)

	assert.Contains(t, result, "720p/index.m3u8?signature=abc123sig&expires=1794200000&tenant=default")
	assert.Contains(t, result, `URI="crypt.key?signature=abc123sig&expires=1794200000&tenant=default"`)
	assert.Contains(t, result, "segment0.ts?signature=abc123sig&expires=1794200000&tenant=default")
	// External URL is untouched
	assert.Contains(t, result, "http://cdn.external.com/segment1.ts\n")
}

func TestMediaHandler_FolderAndHLSEndpoints(t *testing.T) {
	svc, _, _, cfg := setupTestMediaService(t, false)
	handler := media.NewHandler(svc, cfg)

	router := chi.NewRouter()
	router.Route("/api/v1", func(api chi.Router) {
		handler.RegisterRoutes(api)
	})
	handler.MountFileServer(router)

	admin := createAuthenticatedAdminUser()

	// 1. POST /api/v1/media/folders - Create Folder
	createBody := `{"name":"Marketing Videos"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/folders", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var createResp struct {
		Data media.MediaFolder `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	folderID := createResp.Data.ID
	assert.Equal(t, "marketing-videos", createResp.Data.Slug)

	// 2. GET /api/v1/media/folders - List Folders
	req = httptest.NewRequest(http.MethodGet, "/api/v1/media/folders", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 3. GET /api/v1/media/tree - Get Folder Tree
	req = httptest.NewRequest(http.MethodGet, "/api/v1/media/tree", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var treeResp struct {
		Data []*media.MediaFolderTreeNode `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &treeResp))
	require.Len(t, treeResp.Data, 1)
	assert.Equal(t, "Marketing Videos", treeResp.Data[0].Name)

	// 4. POST /api/v1/media/hls - Upload HLS Zip Package into Folder
	hlsFiles := map[string]string{
		"master.m3u8": `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000
stream.m3u8
`,
		"stream.m3u8": `#EXTM3U
#EXTINF:5.0,
chunk0.ts
#EXT-X-ENDLIST
`,
		"chunk0.ts": "fake ts payload",
	}
	zipData := createTestZip(t, hlsFiles)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "campaign.zip")
	require.NoError(t, err)
	_, _ = part.Write(zipData)
	_ = writer.WriteField("title", "Campaign Video")
	_ = writer.WriteField("folder_id", folderID)
	_ = writer.Close()

	req = httptest.NewRequest(http.MethodPost, "/api/v1/media/hls", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var hlsResp struct {
		Data media.MediaAsset `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &hlsResp))
	assert.Equal(t, "application/vnd.apple.mpegurl", hlsResp.Data.MIMEType)
	assert.Equal(t, "hls", hlsResp.Data.Metadata.Format)
	require.NotNil(t, hlsResp.Data.FolderID)
	assert.Equal(t, folderID, *hlsResp.Data.FolderID)

	// 5. GET /api/v1/media/folders/{id}/contents
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/media/folders/%s/contents", folderID), nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// 6. DELETE /api/v1/media/folders/{id}?recursive=true
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/media/folders/%s?recursive=true", folderID), nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserContextKey, admin))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
