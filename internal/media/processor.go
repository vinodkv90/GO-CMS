package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
)

// SniffMIME inspects the first 512 bytes of a stream to accurately detect its MIME content-type.
func SniffMIME(r io.Reader) (string, []byte, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", nil, fmt.Errorf("failed to read bytes for MIME detection: %w", err)
	}

	header := buf[:n]
	mime := http.DetectContentType(header)

	// Special check for WebP: begins with "RIFF" and contains "WEBP" at offset 8
	if len(header) >= 12 && string(header[0:4]) == "RIFF" && string(header[8:12]) == "WEBP" {
		mime = "image/webp"
	}

	// Special check for SVG
	if strings.Contains(strings.ToLower(string(header)), "<svg") {
		mime = "image/svg+xml"
	}

	return mime, header, nil
}

// InspectImage attempts to decode image dimensions and format from a byte buffer.
func InspectImage(data []byte) (width int, height int, format string, err error) {
	// 1. Try standard Go image decoders (PNG, JPEG, GIF)
	cfg, detectedFormat, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		return cfg.Width, cfg.Height, detectedFormat, nil
	}

	// 2. Fallback: Parse WebP header directly
	if len(data) >= 30 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		chunkType := string(data[12:16])
		switch chunkType {
		case "VP8 ": // Lossy
			if len(data) >= 30 {
				w := int(binary.LittleEndian.Uint16(data[26:28])) & 0x3fff
				h := int(binary.LittleEndian.Uint16(data[28:30])) & 0x3fff
				return w, h, "webp", nil
			}
		case "VP8L": // Lossless
			if len(data) >= 25 {
				b1, b2, b3, b4 := uint32(data[21]), uint32(data[22]), uint32(data[23]), uint32(data[24])
				val := b1 | (b2 << 8) | (b3 << 16) | (b4 << 24)
				w := int(val&0x3fff) + 1
				h := int((val>>14)&0x3fff) + 1
				return w, h, "webp", nil
			}
		case "VP8X": // Extended
			if len(data) >= 30 {
				w := int(binary.LittleEndian.Uint32(append(data[24:27], 0))) + 1
				h := int(binary.LittleEndian.Uint32(append(data[27:30], 0))) + 1
				return w, h, "webp", nil
			}
		}
	}

	return 0, 0, "", fmt.Errorf("unsupported or non-image format")
}

// GenerateThumbnail creates a scaled-down thumbnail (max dimension bounded by maxDim)
// using high-quality bilinear interpolation and encodes it as PNG.
func GenerateThumbnail(srcReader io.Reader, maxDim int) ([]byte, int, int, error) {
	if maxDim <= 0 {
		maxDim = 250
	}

	srcImg, _, err := image.Decode(srcReader)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to decode source image: %w", err)
	}

	bounds := srcImg.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	if origW <= 0 || origH <= 0 {
		return nil, 0, 0, fmt.Errorf("invalid image dimensions")
	}

	// Calculate proportional thumbnail dimensions
	var thumbW, thumbH int
	if origW > origH {
		thumbW = maxDim
		thumbH = (origH * maxDim) / origW
	} else {
		thumbH = maxDim
		thumbW = (origW * maxDim) / origH
	}
	if thumbW < 1 {
		thumbW = 1
	}
	if thumbH < 1 {
		thumbH = 1
	}

	// Bilinear downscaling
	dst := image.NewRGBA(image.Rect(0, 0, thumbW, thumbH))
	scaleX := float64(origW) / float64(thumbW)
	scaleY := float64(origH) / float64(thumbH)

	for y := 0; y < thumbH; y++ {
		srcY := float64(y) * scaleY
		yFloor := int(srcY)
		yCeil := yFloor + 1
		if yCeil >= origH {
			yCeil = origH - 1
		}
		yWeight := srcY - float64(yFloor)

		for x := 0; x < thumbW; x++ {
			srcX := float64(x) * scaleX
			xFloor := int(srcX)
			xCeil := xFloor + 1
			if xCeil >= origW {
				xCeil = origW - 1
			}
			xWeight := srcX - float64(xFloor)

			c00 := srcImg.At(bounds.Min.X+xFloor, bounds.Min.Y+yFloor)
			c10 := srcImg.At(bounds.Min.X+xCeil, bounds.Min.Y+yFloor)
			c01 := srcImg.At(bounds.Min.X+xFloor, bounds.Min.Y+yCeil)
			c11 := srcImg.At(bounds.Min.X+xCeil, bounds.Min.Y+yCeil)

			r00, g00, b00, a00 := c00.RGBA()
			r10, g10, b10, a10 := c10.RGBA()
			r01, g01, b01, a01 := c01.RGBA()
			r11, g11, b11, a11 := c11.RGBA()

			// Bilinear blend
			topR := float64(r00)*(1-xWeight) + float64(r10)*xWeight
			topG := float64(g00)*(1-xWeight) + float64(g10)*xWeight
			topB := float64(b00)*(1-xWeight) + float64(b10)*xWeight
			topA := float64(a00)*(1-xWeight) + float64(a10)*xWeight

			botR := float64(r01)*(1-xWeight) + float64(r11)*xWeight
			botG := float64(g01)*(1-xWeight) + float64(g11)*xWeight
			botB := float64(b01)*(1-xWeight) + float64(b11)*xWeight
			botA := float64(a01)*(1-xWeight) + float64(a11)*xWeight

			finalR := uint8((topR*(1-yWeight) + botR*yWeight) / 257)
			finalG := uint8((topG*(1-yWeight) + botG*yWeight) / 257)
			finalB := uint8((topB*(1-yWeight) + botB*yWeight) / 257)
			finalA := uint8((topA*(1-yWeight) + botA*yWeight) / 257)

			dst.Set(x, y, color.RGBA{R: finalR, G: finalG, B: finalB, A: finalA})
		}
	}

	var buf bytes.Buffer
	// Encode as PNG for lossless and transparent thumbnail
	if err := png.Encode(&buf, dst); err != nil {
		// Fallback to JPEG
		buf.Reset()
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to encode thumbnail: %w", err)
		}
	}

	return buf.Bytes(), thumbW, thumbH, nil
}

