# Media Library Folders & HLS Video Upload Support

## Overview

Go CMS provides enterprise-grade media asset management with:
1. **Hierarchical Folder Management**: Full directory hierarchies reflected in storage providers (real directories for local filesystem, key prefixes for S3/GCS/Azure/MinIO), nested path synchronization, circular move protection, and safe recursive deletions.
2. **HLS Video Upload Support**: Uploading pre-packaged HTTP Live Streaming (HLS) packages (`.zip`) containing master playlists (`.m3u8`), variant playlists, video/audio segments (`.ts`, `.m4s`, `.mp4`), subtitles (`.vtt`), and keys (`.key`), with bounded-concurrency streaming to storage and dynamic signed-URL playlist rewriting for secure HLS playback.
3. **Admin Dashboard Player**: Native HLS video streaming preview in the React admin console via `hls.js`.

---

## 1. Media Library Folder Management

### 1.1 Database Schema (`cms_media_folders`)

The folder hierarchy is managed in the database and versioned via migration `202611090001` (`media_folders_schema`):

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | `VARCHAR(36)` | PRIMARY KEY | Unique UUID identifier |
| `name` | `VARCHAR(100)` | NOT NULL | Human-readable folder name (e.g., "Marketing Videos") |
| `slug` | `VARCHAR(100)` | NOT NULL | URL-safe slug (e.g., "marketing-videos") |
| `parent_id` | `VARCHAR(36)` | NULLABLE, FK | References `cms_media_folders(id)` ON DELETE CASCADE |
| `path` | `TEXT` | NOT NULL | Canonical path (e.g., `blog/articles`) |
| `storage_path` | `TEXT` | NOT NULL | Key prefix in storage provider (e.g., `blog/articles/`) |
| `created_at` | `TIMESTAMPTZ` | DEFAULT CURRENT_TIMESTAMP | Creation timestamp |
| `updated_at` | `TIMESTAMPTZ` | DEFAULT CURRENT_TIMESTAMP | Last update timestamp |
| `created_by` | `VARCHAR(36)` | NULLABLE | User ID who created the folder |

In `cms_media`:
- `folder_id` column added (`VARCHAR(36)` NULLABLE, REFERENCES `cms_media_folders(id)` ON DELETE SET NULL).
- Indexes: `idx_media_folders_parent`, `idx_media_folders_path`, `idx_cms_media_folder`.

### 1.2 Storage Provider Behavior

- **Local Filesystem**: Creates real directory structures on the host filesystem using `os.MkdirAll`. Moving folders or files utilizes `os.Rename` with recursive fallback, and deletions use `os.RemoveAll`.
- **S3 / MinIO / GCS / Azure**: Object keys use prefix convention `<folder_storage_path><filename>`. Folders require zero empty marker files.
- **Path Traversal Protection**: Folder and file paths are sanitized, strictly disallowing null bytes (`\x00`), leading slashes, and `../` path traversal sequences.

### 1.3 Folder Hierarchy Operations

- **Creation**: Validates unique slug under the parent directory and computes canonical `path` and `storage_path`.
- **Renaming**: Renames the folder, updates all descendant subfolder paths and descendant assets' storage keys, and moves storage files/directories.
- **Moving**: Prevents moving a folder into itself or any of its descendants (`ErrCircularFolder`).
- **Deletion**: 
  - Standard deletion (`recursive=false`) rejects folders containing files or subfolders (`ErrFolderNotEmpty`).
  - Recursive deletion (`recursive=true`) purges the entire storage prefix and cascade-deletes descendant database records.

---

## 2. HLS Video Upload Support

### 2.1 Package Requirements & Extraction

Administrators can upload complete HLS video packages in `.zip` format via `POST /api/v1/media/hls`.

The uploaded archive is inspected in-memory or buffered with strict safety constraints:
- **Max Compressed Size**: 1 GB (`MaxHLSZipSizeBytes`).
- **Max Uncompressed Size**: 2 GB (`MaxHLSExtractedSizeBytes`) to prevent zip-bomb attacks.
- **Max File Count**: 5,000 files (`MaxHLSFileCount`).
- **Max Single File Size**: 500 MB (`MaxHLSSingleFileSize`).
- **Path Sanitization**: Entries with null bytes, path traversal (`../`), absolute paths, or symlinks are rejected immediately.

### 2.2 Playlist Integrity & Relative Verification

`InspectAndValidateHLSPackage` verifies:
1. At least one valid `.m3u8` playlist exists in the archive.
2. Identifies the master playlist:
   - Detects `#EXT-X-STREAM-INF` multi-bitrate master playlists and extracts variant resolutions, bandwidths, and codecs.
   - Fallback identifies `master.m3u8`, `index.m3u8`, or the primary playlist.
3. **Reference Validation**: Iterates every `#EXTINF` segment, `#EXT-X-STREAM-INF` child playlist, `#EXT-X-KEY` encryption key, and subtitle track reference inside playlists to verify that the referenced files exist in the zip archive (`ErrHLSMissingSegment` raised if any segment is missing).

### 2.3 Bounded-Concurrency Streaming to Storage

Extracted files are streamed directly into the target storage provider under `<folderStoragePath>hls/<package_slug>/...` using a bounded worker pool (`HLSUploadConcurrency = 5`) to ensure low memory consumption. If any file fails to save, context cancellation halts workers and compensating cleanup deletes the partially uploaded prefix.

RFC MIME types applied:
- `.m3u8`: `application/vnd.apple.mpegurl`
- `.ts`: `video/mp2t`
- `.m4s`: `video/iso.segment`
- `.mp4`: `video/mp4`
- `.aac`: `audio/aac`
- `.vtt`, `.webvtt`: `text/vtt`
- `.key`: `application/octet-stream`

### 2.4 Dynamic Signed URL Playlist Rewriting

When serving `.m3u8` playlists over signed URLs:
The static file server (`ServeFile`) dynamically rewrites relative URIs (`chunk0.ts`, `720p/index.m3u8`, `key.bin`) using `RewriteHLSPlaylistWithSignedParams` to propagate HMAC signature tokens (`signature=...&expires=...&tenant=...`) to child requests, ensuring HLS video players like `hls.js` stream all video segments without authorization errors.

---

## 3. REST API Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/media/tree` | Returns complete hierarchical folder tree with asset counts |
| `GET` | `/api/v1/media/folders` | Lists folders (`?parent_id=...` or `?parent_id=root`) |
| `POST` | `/api/v1/media/folders` | Creates a new folder (`{ "name": "...", "parent_id": "..." }`) |
| `GET` | `/api/v1/media/folders/{id}` | Retrieves folder details |
| `PUT` | `/api/v1/media/folders/{id}` | Renames a folder and synchronizes storage paths |
| `DELETE` | `/api/v1/media/folders/{id}` | Deletes a folder (`?recursive=true` for cascade) |
| `GET` | `/api/v1/media/folders/{id}/contents` | Returns subfolders and assets within a folder |
| `POST` | `/api/v1/media/folders/{id}/move` | Moves a folder (`{ "target_parent_id": "..." }`) |
| `POST` | `/api/v1/media/{id}/move` | Moves a media asset to a target folder (`{ "target_folder_id": "..." }`) |
| `POST` | `/api/v1/media` | Uploads a media asset (`folder_id` form field supported) |
| `POST` | `/api/v1/media/hls` | Uploads an HLS video package (`file`, `title`, `folder_id`) |
| `GET` | `/uploads/*` | Serves media files with dynamic HLS signed playlist rewriting |

---

## 4. Admin Dashboard Integration

The React administration dashboard (`admin/src/views/MediaLibraryView.tsx`) includes:
- **Collapsible Folder Tree Sidebar**: Displays "All Media" and nested directories with live asset count badges.
- **Breadcrumb Navigation**: Path breadcrumb bar for directory traversal.
- **Folder Modals**: Create Folder, Rename Folder, and Recursive Deletion dialogs.
- **Move Asset Dialog**: Relocate any asset to a selected folder.
- **HLS Package Upload Tab**: Drag-and-drop `.zip` upload with title and progress status.
- **Interactive HLS Player**: Embedded HTML5 player powered by `hls.js` with quality switching, variant inspection, and native Safari HLS fallback.
