package media

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	disallowedCharsRegex = regexp.MustCompile(`[^a-z0-9\-.]`)
	multiHyphenRegex     = regexp.MustCompile(`-+`)
)

// SanitizeFilename normalizes an uploaded filename into a safe, web-friendly, URL-safe format:
// - Strips path components
// - Converts to lowercase
// - Replaces spaces and underscores with hyphens
// - Strips non-alphanumeric characters (except dots and hyphens)
// - Collapses consecutive hyphens
// - Trims leading/trailing hyphens and dots
func SanitizeFilename(rawName string) string {
	rawName = strings.TrimSpace(rawName)
	if rawName == "" {
		return "file"
	}

	base := filepath.Base(rawName)

	// Remove null bytes
	base = strings.ReplaceAll(base, "\x00", "")

	// Extract extension and name
	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)

	// Lowercase
	cleanName := strings.ToLower(nameWithoutExt)
	cleanExt := strings.ToLower(ext)

	// Replace spaces and underscores with hyphens
	cleanName = strings.ReplaceAll(cleanName, " ", "-")
	cleanName = strings.ReplaceAll(cleanName, "_", "-")

	// Strip disallowed characters
	cleanName = disallowedCharsRegex.ReplaceAllString(cleanName, "")
	cleanExt = disallowedCharsRegex.ReplaceAllString(cleanExt, "")

	// Collapse multiple hyphens
	cleanName = multiHyphenRegex.ReplaceAllString(cleanName, "-")

	// Trim hyphens and dots from ends
	cleanName = strings.Trim(cleanName, "-.")
	if cleanExt == "." {
		cleanExt = ""
	}

	if cleanName == "" {
		cleanName = "file"
	}

	return cleanName + cleanExt
}

// GenerateStorageKey builds a collision-proof date-partitioned storage key:
// e.g. "2026/11/1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d_sanitized-name.png"
func GenerateStorageKey(sanitizedName string) (string, string) {
	now := time.Now().UTC()
	id := uuid.New().String()

	ext := filepath.Ext(sanitizedName)
	nameWithoutExt := strings.TrimSuffix(sanitizedName, ext)

	storageKey := fmt.Sprintf("%04d/%02d/%s_%s%s", now.Year(), now.Month(), id, nameWithoutExt, ext)

	return id, storageKey
}

// SlugifyFolder converts a folder name into a safe URL and directory path slug.
func SlugifyFolder(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	name = disallowedCharsRegex.ReplaceAllString(name, "")
	name = multiHyphenRegex.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-.")
	if name == "" {
		return "folder"
	}
	return name
}

// GenerateFolderStorageKey creates a collision-proof storage key within a folder prefix:
// e.g. "blog/articles/2026/11/1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d_sanitized-name.png"
func GenerateFolderStorageKey(folderStoragePath, sanitizedName string) (string, string) {
	id, relKey := GenerateStorageKey(sanitizedName)
	cleanPrefix := strings.Trim(folderStoragePath, "/")
	if cleanPrefix == "" {
		return id, relKey
	}
	return id, cleanPrefix + "/" + relKey
}
