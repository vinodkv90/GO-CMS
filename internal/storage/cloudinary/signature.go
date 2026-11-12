package cloudinary

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/go-cms/cms/internal/storage"
)

// SignParams computes the Cloudinary API signature over non-empty sorted parameters.
// Excluded parameters per Cloudinary API spec: file, cloud_name, resource_type, api_key, signature.
func SignParams(params map[string]string, apiSecret string, useSHA256 bool) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v == "" {
			continue
		}
		switch k {
		case "file", "cloud_name", "resource_type", "api_key", "signature":
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	b.WriteString(apiSecret)

	if useSHA256 {
		sum := sha256.Sum256([]byte(b.String()))
		return hex.EncodeToString(sum[:])
	}
	sum := sha1.Sum([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// SignDeliveryPath generates the `s--XXXXXXXX--` signature component for Cloudinary delivery URLs.
// toSign is typically `<transform_segment>/<public_id_with_format>` (excluding version prefix `v1234/`).
func SignDeliveryPath(toSign, apiSecret string) string {
	trimmed := strings.TrimPrefix(toSign, "/")
	sum := sha1.Sum([]byte(trimmed + apiSecret))
	encoded := base64.URLEncoding.EncodeToString(sum[:])
	if len(encoded) > 8 {
		encoded = encoded[:8]
	}
	return "s--" + encoded + "--"
}

// BuildTransformSegment validates and formats safe Cloudinary URL transformation tokens.
func BuildTransformSegment(opts storage.ImageTransformOptions) (string, error) {
	parts := make([]string, 0, 7)

	if opts.Crop != "" {
		crop := strings.ToLower(strings.TrimSpace(opts.Crop))
		if !isValidCrop(crop) {
			return "", fmt.Errorf("invalid crop mode %q", opts.Crop)
		}
		parts = append(parts, "c_"+crop)
	}

	if opts.Gravity != "" {
		gravity := strings.ToLower(strings.TrimSpace(opts.Gravity))
		if !isValidGravity(gravity) {
			return "", fmt.Errorf("invalid gravity %q", opts.Gravity)
		}
		parts = append(parts, "g_"+gravity)
	}

	if opts.Width < 0 || opts.Width > 10000 {
		return "", fmt.Errorf("width must be between 0 and 10000")
	}
	if opts.Width > 0 {
		parts = append(parts, "w_"+strconv.Itoa(opts.Width))
	}

	if opts.Height < 0 || opts.Height > 10000 {
		return "", fmt.Errorf("height must be between 0 and 10000")
	}
	if opts.Height > 0 {
		parts = append(parts, "h_"+strconv.Itoa(opts.Height))
	}

	if opts.DPR != "" {
		dpr := strings.ToLower(strings.TrimSpace(opts.DPR))
		if !isValidDPR(dpr) {
			return "", fmt.Errorf("invalid dpr %q", opts.DPR)
		}
		parts = append(parts, "dpr_"+dpr)
	}

	if opts.Format != "" {
		fmtName := strings.ToLower(strings.TrimSpace(opts.Format))
		if !isValidFormat(fmtName) {
			return "", fmt.Errorf("invalid format %q", opts.Format)
		}
		parts = append(parts, "f_"+fmtName)
	}

	if opts.Quality != "" {
		qual := strings.ToLower(strings.TrimSpace(opts.Quality))
		if !isValidQuality(qual) {
			return "", fmt.Errorf("invalid quality %q", opts.Quality)
		}
		parts = append(parts, "q_"+qual)
	}

	return strings.Join(parts, ","), nil
}

func isValidCrop(v string) bool {
	switch v {
	case "fill", "fit", "limit", "scale", "thumb", "pad", "crop", "lfill", "mfit":
		return true
	default:
		return false
	}
}

func isValidGravity(v string) bool {
	switch v {
	case "auto", "center", "face", "faces", "north", "south", "east", "west":
		return true
	default:
		return false
	}
}

func isValidDPR(v string) bool {
	switch v {
	case "auto", "1", "2", "3", "1.0", "1.5", "2.0", "3.0":
		return true
	default:
		return false
	}
}

func isValidFormat(v string) bool {
	switch v {
	case "auto", "webp", "avif", "jpg", "jpeg", "png", "gif":
		return true
	default:
		return false
	}
}

func isValidQuality(v string) bool {
	switch v {
	case "auto", "auto:best", "auto:good", "auto:eco", "auto:low":
		return true
	}
	n, err := strconv.Atoi(v)
	return err == nil && n >= 1 && n <= 100
}

