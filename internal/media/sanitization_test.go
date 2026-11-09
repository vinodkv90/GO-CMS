package media_test

import (
	"strings"
	"testing"

	"github.com/go-cms/cms/internal/media"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My Vacation Photo.jpg", "my-vacation-photo.jpg"},
		{"Screenshot 2026-11-08 at 12.30.45.png", "screenshot-2026-11-08-at-12.30.45.png"},
		{"../../../etc/passwd", "passwd"},
		{"..\\..\\boot.ini", "boot.ini"},
		{"Hello___World---2026.JPEG", "hello-world-2026.jpeg"},
		{"Special @#$%^&*()Chars!.pdf", "special-chars.pdf"},
		{"  spaces   everywhere  .gif  ", "spaces-everywhere.gif"},
		{"...leading.dots.txt", "leading.dots.txt"},
		{"", "file"},
	}

	for _, tc := range tests {
		actual := media.SanitizeFilename(tc.input)
		assert.Equal(t, tc.expected, actual, "failed for input: %q", tc.input)
	}
}

func TestGenerateStorageKey(t *testing.T) {
	id, key := media.GenerateStorageKey("my-photo.jpg")

	assert.NotEmpty(t, id)
	assert.True(t, strings.HasSuffix(key, "my-photo.jpg"))
	assert.Contains(t, key, "/")
	assert.Contains(t, key, id)
}

func TestSlugifyFolder(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Blog Articles", "blog-articles"},
		{"Products & Electronics!", "products-electronics"},
		{"  Nested / Child Folder  ", "nested-child-folder"},
		{"Tutorials_2026", "tutorials-2026"},
		{"", "folder"},
	}

	for _, tc := range tests {
		actual := media.SlugifyFolder(tc.input)
		assert.Equal(t, tc.expected, actual)
	}
}

func TestGenerateFolderStorageKey(t *testing.T) {
	id, key := media.GenerateFolderStorageKey("blog/articles/", "go-cms.jpg")
	assert.NotEmpty(t, id)
	assert.True(t, strings.HasPrefix(key, "blog/articles/"))
	assert.True(t, strings.HasSuffix(key, "go-cms.jpg"))
}
