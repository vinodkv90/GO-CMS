package media_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/go-cms/cms/internal/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func createTestJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 50, G: 150, B: 250, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	return buf.Bytes()
}

func TestMIMESniffing(t *testing.T) {
	// PNG
	pngData := createTestPNG(10, 10)
	mime, _, err := media.SniffMIME(bytes.NewReader(pngData))
	require.NoError(t, err)
	assert.Equal(t, "image/png", mime)

	// JPEG
	jpegData := createTestJPEG(10, 10)
	mime, _, err = media.SniffMIME(bytes.NewReader(jpegData))
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", mime)

	// SVG
	svgData := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"><circle cx="50" cy="50" r="40"/></svg>`)
	mime, _, err = media.SniffMIME(bytes.NewReader(svgData))
	require.NoError(t, err)
	assert.Equal(t, "image/svg+xml", mime)

	// Plain text
	textData := []byte("Hello, this is just plain text.")
	mime, _, err = media.SniffMIME(bytes.NewReader(textData))
	require.NoError(t, err)
	assert.Contains(t, mime, "text/plain")
}

func TestInspectImage(t *testing.T) {
	// 1. Inspect PNG
	pngData := createTestPNG(800, 600)
	w, h, format, err := media.InspectImage(pngData)
	require.NoError(t, err)
	assert.Equal(t, 800, w)
	assert.Equal(t, 600, h)
	assert.Equal(t, "png", format)

	// 2. Inspect JPEG
	jpegData := createTestJPEG(1024, 768)
	w, h, format, err = media.InspectImage(jpegData)
	require.NoError(t, err)
	assert.Equal(t, 1024, w)
	assert.Equal(t, 768, h)
	assert.Equal(t, "jpeg", format)

	// 3. Inspect non-image data returns error
	_, _, _, err = media.InspectImage([]byte("not an image"))
	assert.Error(t, err)
}

func TestGenerateThumbnail(t *testing.T) {
	pngData := createTestPNG(1200, 800)
	thumbBytes, tw, th, err := media.GenerateThumbnail(bytes.NewReader(pngData), 250)
	require.NoError(t, err)
	assert.NotEmpty(t, thumbBytes)
	assert.Equal(t, 250, tw)
	assert.Equal(t, (800*250)/1200, th)

	// Verify thumbnail is valid decodable image
	cfg, format, err := image.DecodeConfig(bytes.NewReader(thumbBytes))
	require.NoError(t, err)
	assert.Equal(t, 250, cfg.Width)
	assert.Equal(t, th, cfg.Height)
	assert.Equal(t, "png", format)
}

