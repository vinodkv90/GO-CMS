# Cloudinary Storage Provider

Go CMS integrates **Cloudinary** directly through the existing `storage.StorageProvider`, `media.Service`, and `settings.Service` abstractions. No separate Cloudinary-only media table or duplicate media service is required.

## Configuration Options

Configure Cloudinary in **Admin -> Settings -> System -> Storage Providers**:

| Setting | Description | Default |
|---|---|---|
| **Cloud Name** | Cloudinary product environment name (`^[a-zA-Z0-9_-]{1,64}$`). | Required |
| **API Key** | Cloudinary API key for Upload & Admin API signatures. | Required |
| **API Secret** | Encrypted at rest via AES-256-GCM; masked as `__PRESERVE__` on read. | Required |
| **Root Folder** | Base Cloudinary folder prefix mapped cleanly onto CMS paths. | `""` (account root) |
| **Delivery Type** | `upload` (public), `authenticated` (signed delivery), or `private`. | `upload` |
| **Default Resource Type** | `auto` (recommended), `image`, `video`, or `raw`. | `auto` |
| **Asset Naming Strategy** | `cms_key` (deterministic), `original_filename`, or `random_id`. | `cms_key` |
| **Upload Preset** | Optional Cloudinary upload preset name. | `""` |
| **Use Secure HTTPS URLs** | Enforces HTTPS CDN delivery URLs (`https://res.cloudinary.com/...`). | `true` |
| **Automatic Image Optimization** | Injects `f_auto,q_auto` defaults for non-SVG raster images when enabled. | `true` |

## Environment Variable Fallback

If credentials are not persisted in Settings JSON, Go CMS reads these environment variables at runtime without storing plaintext secrets in settings JSON:

- `CLOUDINARY_URL=cloudinary://<api_key>:<api_secret>@<cloud_name>`
- Or individual variables:
  - `CLOUDINARY_CLOUD_NAME`
  - `CLOUDINARY_API_KEY`
  - `CLOUDINARY_API_SECRET`

Never commit real Cloudinary credentials into source control or `.env` templates.

## Media Library Integration

- **Images, Videos & Documents**: Uploads route automatically to Cloudinary's Upload API (`image`, `video`, `raw`, or `auto`).
- **Responsive Thumbnails & Transforms**: When responsive thumbnails are enabled, Cloudinary variant URLs (`small`, `medium`, `large`) are stored using validated transformations (`w_245`, `w_750`, `w_1000`, `c_limit`, `f_auto`, `q_auto`) without duplicating image files. Custom validated transforms are exposed at `GET /api/v1/media/{id}/transform`.
- **Replace File (`POST /api/v1/media/{id}/replace`)**: Uploads the replacement asset to the active storage provider, updates `cms_media` while preserving the asset `ID` and Content Manager entry relations, and deletes the replaced file from its original provider.
- **Folders**: Media Library folders map onto `RootFolder/<folder_path>` cleanly. Renaming or moving a folder invokes Cloudinary's `rename` API per affected asset and updates stored keys/metadata.
- **Signed URLs**: Assets with `authenticated` or `private` delivery types generate Cloudinary `s--XXXXXXXX--` signed delivery URLs.

## HLS Video Packages

Multi-file `.zip` HLS streaming packages require filesystem-style multi-file segment extraction and playlist rewriting and are supported on **Local Filesystem**, **Amazon S3**, and **MinIO / S3-Compatible** providers.

When Cloudinary is the active storage driver:
- Uploading a `.zip` HLS package is rejected with HTTP `400 Bad Request`.
- Standard video formats (`.mp4`, `.mov`, `.webm`, `.mkv`) upload normally via Cloudinary `video` delivery and stream in the Media Library preview modal.

## Safe Provider Switching

Switching `Active Storage Driver` between `local`, `s3`, `minio`, and `cloudinary` updates `SwappableProvider` at runtime while keeping all configured providers registered:

- Existing `local` assets remain accessible on `/uploads/*`, movable, replaceable, and deletable even when Cloudinary is active.
- Existing `cloudinary` assets remain accessible via their CDN URLs and cleanly deletable even after switching back to `local` or `s3`.

## Security Controls

- `api_secret` is encrypted with AES-256-GCM before storage, redacted from audit logs and `fmt.Stringer` output, and never returned in HTTP GET responses.
- Upload keys and folder paths are validated against path traversal (`..`, `\`, NUL bytes).
- Cloudinary transformation parameters (`w`, `h`, `crop`, `format`, `quality`, `gravity`, `dpr`) are allowlist-validated before constructing delivery URLs.

## Troubleshooting

1. **Test Connection fails with 401/403**: Re-verify Cloud Name, API Key, and API Secret (or check system clock skew for Upload API timestamps).
2. **Test Connection reports network/TLS error**: Ensure outbound HTTPS access to `api.cloudinary.com` is permitted from the Go CMS host.
3. **Old Local assets return 404 after switching**: Verify `./uploads` directory remains readable and Local Filesystem settings remain valid.

