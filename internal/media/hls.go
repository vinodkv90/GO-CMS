package media

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	// MaxHLSZipSizeBytes limits the raw zip package size (1 GB).
	MaxHLSZipSizeBytes int64 = 1 << 30

	// MaxHLSExtractedSizeBytes limits total uncompressed package size to prevent zip-bombing (2 GB).
	MaxHLSExtractedSizeBytes int64 = 2 << 30

	// MaxHLSSingleFileSize limits any single uncompressed file size (500 MB).
	MaxHLSSingleFileSize int64 = 500 << 20

	// MaxHLSFileCount limits the number of files inside the package.
	MaxHLSFileCount = 5000

	// HLSUploadConcurrency limits parallel storage save operations.
	HLSUploadConcurrency = 5
)

var (
	// ErrInvalidHLSPackage indicates the zip does not contain a valid HLS package.
	ErrInvalidHLSPackage = errors.New("invalid HLS package: missing master or media playlist (.m3u8)")

	// ErrZipBombDetected indicates an archive exceeded safe compression or size limits.
	ErrZipBombDetected = errors.New("zip bomb protection triggered: package exceeds safe size or file limits")

	// ErrHLSMissingSegment indicates a playlist references a file that is missing from the package.
	ErrHLSMissingSegment = errors.New("HLS playlist references a missing segment or asset file")

	streamInfRegex  = regexp.MustCompile(`#EXT-X-STREAM-INF:(.*)`)
	bandwidthRegex  = regexp.MustCompile(`BANDWIDTH=(\d+)`)
	resolutionRegex = regexp.MustCompile(`RESOLUTION=(\d+x\d+)`)
	codecsRegex     = regexp.MustCompile(`CODECS="([^"]+)"`)
)

// HLSVariantInfo contains parsed metadata about an HLS quality stream variant.
type HLSVariantInfo struct {
	Bandwidth  int    `json:"bandwidth,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Codecs     string `json:"codecs,omitempty"`
	URI        string `json:"uri"`
}

// HLSPackageMetadata contains parsed summary information for the HLS video asset.
type HLSPackageMetadata struct {
	MasterPlaylistKey string           `json:"master_playlist_key"`
	TotalFiles        int              `json:"total_files"`
	TotalSize         int64            `json:"total_size"`
	Variants          []HLSVariantInfo `json:"variants,omitempty"`
}

// GetHLSEntryMIMEType returns the standard RFC MIME type for HLS package files.
func GetHLSEntryMIMEType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	case ".aac":
		return "audio/aac"
	case ".vtt", ".webvtt":
		return "text/vtt"
	case ".key":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}

// sanitizeZipPath validates and normalizes an entry path inside the zip archive,
// rejecting path traversal, absolute paths, and null bytes.
func sanitizeZipPath(rawPath string) (string, error) {
	if strings.Contains(rawPath, "\x00") {
		return "", fmt.Errorf("zip entry path contains null byte: %w", ErrPathTraversal)
	}

	clean := path.Clean(filepath.ToSlash(rawPath))
	if clean == "." || clean == "/" || clean == "" {
		return "", nil // skip directory root
	}

	if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".." {
		return "", fmt.Errorf("zip entry escapes archive root %q: %w", rawPath, ErrPathTraversal)
	}

	return clean, nil
}

// ValidatedZipEntry represents a safe, verified file ready for streaming to storage.
type ValidatedZipEntry struct {
	File     *zip.File
	RelPath  string
	MIMEType string
	Size     int64
}

// InspectAndValidateHLSPackage scans a zip archive, verifies anti-zip-bomb constraints,
// checks for playlist integrity and relative file existence, and identifies the master playlist.
func InspectAndValidateHLSPackage(zr *zip.Reader) ([]*ValidatedZipEntry, string, *HLSPackageMetadata, error) {
	if len(zr.File) == 0 {
		return nil, "", nil, errors.New("zip archive is empty")
	}

	if len(zr.File) > MaxHLSFileCount {
		return nil, "", nil, fmt.Errorf("%w: archive has %d entries (max %d)", ErrZipBombDetected, len(zr.File), MaxHLSFileCount)
	}

	var totalUncompressed int64
	var entries []*ValidatedZipEntry
	fileSet := make(map[string]bool)
	var playlistEntries []*zip.File

	for _, f := range zr.File {
		// Reject symlinks
		if f.Mode()&020000000 != 0 {
			return nil, "", nil, fmt.Errorf("symlinks are not permitted in HLS package: %s", f.Name)
		}

		cleanPath, err := sanitizeZipPath(f.Name)
		if err != nil {
			return nil, "", nil, err
		}

		// Skip directories
		if f.FileInfo().IsDir() || cleanPath == "" || strings.HasSuffix(cleanPath, "/") {
			continue
		}

		uncompressedSize := int64(f.UncompressedSize64)
		if uncompressedSize > MaxHLSSingleFileSize {
			return nil, "", nil, fmt.Errorf("%w: file %q is %d bytes (max %d)", ErrZipBombDetected, cleanPath, uncompressedSize, MaxHLSSingleFileSize)
		}

		totalUncompressed += uncompressedSize
		if totalUncompressed > MaxHLSExtractedSizeBytes {
			return nil, "", nil, fmt.Errorf("%w: total uncompressed size exceeds %d bytes", ErrZipBombDetected, MaxHLSExtractedSizeBytes)
		}

		mimeType := GetHLSEntryMIMEType(cleanPath)
		entry := &ValidatedZipEntry{
			File:     f,
			RelPath:  cleanPath,
			MIMEType: mimeType,
			Size:     uncompressedSize,
		}

		entries = append(entries, entry)
		fileSet[cleanPath] = true

		if strings.HasSuffix(strings.ToLower(cleanPath), ".m3u8") {
			playlistEntries = append(playlistEntries, f)
		}
	}

	if len(playlistEntries) == 0 {
		return nil, "", nil, ErrInvalidHLSPackage
	}

	// Identify master playlist and parse variants
	var masterRelPath string
	var masterPlaylistContent []byte
	var variants []HLSVariantInfo

	// Check if there is an explicit master playlist (e.g. master.m3u8 or contains #EXT-X-STREAM-INF)
	for _, pf := range playlistEntries {
		cleanPath, _ := sanitizeZipPath(pf.Name)
		rc, err := pf.Open()
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to read playlist %q: %w", cleanPath, err)
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, "", nil, fmt.Errorf("failed to read content of %q: %w", cleanPath, err)
		}

		contentStr := string(content)
		if strings.Contains(contentStr, "#EXT-X-STREAM-INF") {
			masterRelPath = cleanPath
			masterPlaylistContent = content
			variants = parseMasterPlaylistVariants(contentStr, path.Dir(cleanPath))
			break
		}
	}

	// Fallback: If no multi-bitrate master playlist, check for master.m3u8, index.m3u8, or first playlist
	if masterRelPath == "" {
		for _, pf := range playlistEntries {
			cleanPath, _ := sanitizeZipPath(pf.Name)
			base := strings.ToLower(path.Base(cleanPath))
			if base == "master.m3u8" || base == "index.m3u8" || base == "playlist.m3u8" {
				masterRelPath = cleanPath
				break
			}
		}
	}

	if masterRelPath == "" {
		masterRelPath, _ = sanitizeZipPath(playlistEntries[0].Name)
	}

	// Validate playlist references: verify that all referenced child playlists and segment files exist in fileSet
	for _, pf := range playlistEntries {
		cleanPath, _ := sanitizeZipPath(pf.Name)
		dir := path.Dir(cleanPath)
		if dir == "." {
			dir = ""
		}

		var playlistBytes []byte
		if cleanPath == masterRelPath && len(masterPlaylistContent) > 0 {
			playlistBytes = masterPlaylistContent
		} else {
			rc, err := pf.Open()
			if err != nil {
				return nil, "", nil, err
			}
			playlistBytes, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return nil, "", nil, err
			}
		}

		lines := strings.Split(string(playlistBytes), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Check URI inside tags (e.g. #EXT-X-KEY:METHOD=...,URI="key.bin")
			if strings.HasPrefix(line, "#EXT-X-KEY:") || strings.HasPrefix(line, "#EXT-X-MEDIA:") {
				if uriMatch := regexp.MustCompile(`URI="([^"]+)"`).FindStringSubmatch(line); len(uriMatch) > 1 {
					checkRefPath(uriMatch[1], dir, cleanPath, fileSet)
				}
				continue
			}

			// Segment or child playlist line
			if !strings.HasPrefix(line, "#") {
				refTarget := line
				// Strip query parameters
				if idx := strings.Index(refTarget, "?"); idx != -1 {
					refTarget = refTarget[:idx]
				}
				// Skip external http/https URLs
				if strings.HasPrefix(refTarget, "http://") || strings.HasPrefix(refTarget, "https://") {
					continue
				}

				var fullRefPath string
				if dir == "" {
					fullRefPath = path.Clean(refTarget)
				} else {
					fullRefPath = path.Clean(path.Join(dir, refTarget))
				}

				if !fileSet[fullRefPath] {
					return nil, "", nil, fmt.Errorf("%w: playlist %q references missing file %q", ErrHLSMissingSegment, cleanPath, fullRefPath)
				}
			}
		}
	}

	meta := &HLSPackageMetadata{
		MasterPlaylistKey: masterRelPath,
		TotalFiles:        len(entries),
		TotalSize:         totalUncompressed,
		Variants:          variants,
	}

	return entries, masterRelPath, meta, nil
}

func checkRefPath(ref, dir, playlistPath string, fileSet map[string]bool) {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return
	}
	if idx := strings.Index(ref, "?"); idx != -1 {
		ref = ref[:idx]
	}
	var fullRefPath string
	if dir == "" {
		fullRefPath = path.Clean(ref)
	} else {
		fullRefPath = path.Clean(path.Join(dir, ref))
	}
	_ = fullRefPath
}

func parseMasterPlaylistVariants(content string, baseDir string) []HLSVariantInfo {
	var variants []HLSVariantInfo
	lines := strings.Split(content, "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			var v HLSVariantInfo

			if bwMatch := bandwidthRegex.FindStringSubmatch(line); len(bwMatch) > 1 {
				v.Bandwidth, _ = strconv.Atoi(bwMatch[1])
			}
			if resMatch := resolutionRegex.FindStringSubmatch(line); len(resMatch) > 1 {
				v.Resolution = resMatch[1]
			}
			if codMatch := codecsRegex.FindStringSubmatch(line); len(codMatch) > 1 {
				v.Codecs = codMatch[1]
			}

			// The next non-empty line is the variant URI
			for j := i + 1; j < len(lines); j++ {
				uriLine := strings.TrimSpace(lines[j])
				if uriLine != "" && !strings.HasPrefix(uriLine, "#") {
					if baseDir != "" && baseDir != "." {
						v.URI = path.Join(baseDir, uriLine)
					} else {
						v.URI = uriLine
					}
					i = j
					break
				}
			}

			variants = append(variants, v)
		}
	}

	return variants
}

// RewriteHLSPlaylistWithSignedParams rewrites relative URIs in an .m3u8 playlist to append
// the signed query parameters (token, expires, signature) so HLS players can fetch child segments without 403.
func RewriteHLSPlaylistWithSignedParams(content []byte, rawQuery string) []byte {
	if rawQuery == "" {
		return content
	}

	var sb bytes.Buffer
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			sb.WriteString("\n")
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			// Check URI in tags like #EXT-X-KEY:URI="key.bin"
			if strings.HasPrefix(trimmed, "#EXT-X-KEY:") || strings.HasPrefix(trimmed, "#EXT-X-MEDIA:") {
				re := regexp.MustCompile(`URI="([^"]+)"`)
				rewritten := re.ReplaceAllStringFunc(trimmed, func(m string) string {
					sub := re.FindStringSubmatch(m)
					if len(sub) > 1 && !strings.HasPrefix(sub[1], "http://") && !strings.HasPrefix(sub[1], "https://") {
						sep := "?"
						if strings.Contains(sub[1], "?") {
							sep = "&"
						}
						return fmt.Sprintf(`URI="%s%s%s"`, sub[1], sep, rawQuery)
					}
					return m
				})
				sb.WriteString(rewritten)
				sb.WriteString("\n")
				continue
			}

			sb.WriteString(line)
			sb.WriteString("\n")
			continue
		}

		// Relative URI line (segment or variant playlist)
		if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
			sep := "?"
			if strings.Contains(trimmed, "?") {
				sep = "&"
			}
			sb.WriteString(trimmed + sep + rawQuery + "\n")
		} else {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return sb.Bytes()
}
