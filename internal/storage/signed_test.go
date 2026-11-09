package storage_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/go-cms/cms/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLSigner_SignAndVerify(t *testing.T) {
	secretKey := "super-secure-hmac-secret-key-32b"
	signer := storage.NewURLSigner(secretKey, 1*time.Hour)

	rawURL := "/uploads/2026/11/photo.jpg"

	// 1. Sign URL
	signedURLStr, err := signer.Sign(rawURL, 30*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, signedURLStr, "expires=")
	assert.Contains(t, signedURLStr, "signature=")

	parsedURL, err := url.Parse(signedURLStr)
	require.NoError(t, err)

	// 2. Verify valid URL
	err = signer.VerifyURL(parsedURL)
	require.NoError(t, err)

	// 3. Reject tampered path
	tamperedURL, err := url.Parse(signedURLStr)
	require.NoError(t, err)
	tamperedURL.Path = "/uploads/2026/11/different-file.jpg"
	err = signer.VerifyURL(tamperedURL)
	assert.ErrorIs(t, err, storage.ErrInvalidSignature)

	// 4. Reject tampered signature
	tamperedSigURL, err := url.Parse(signedURLStr)
	require.NoError(t, err)
	q := tamperedSigURL.Query()
	q.Set("signature", "deadbeef0123456789abcdef")
	tamperedSigURL.RawQuery = q.Encode()
	err = signer.VerifyURL(tamperedSigURL)
	assert.ErrorIs(t, err, storage.ErrInvalidSignature)

	// 5. Reject missing signature parameters
	plainURL, _ := url.Parse("/uploads/2026/11/photo.jpg")
	err = signer.VerifyURL(plainURL)
	assert.ErrorIs(t, err, storage.ErrMissingSignature)

	// 6. Reject expired signature
	expiredURLStr, err := signer.SignWithExpiration(rawURL, time.Now().Add(-10*time.Minute))
	require.NoError(t, err)
	parsedExpiredURL, _ := url.Parse(expiredURLStr)
	err = signer.VerifyURL(parsedExpiredURL)
	assert.ErrorIs(t, err, storage.ErrSignatureExpired)
}

func TestURLSigner_VerifyPath(t *testing.T) {
	secretKey := "test-secret"
	signer := storage.NewURLSigner(secretKey, 1*time.Hour)

	signedURLStr, err := signer.Sign("/uploads/test.png", 10*time.Minute)
	require.NoError(t, err)

	u, _ := url.Parse(signedURLStr)
	q := u.Query()
	sig := q.Get("signature")
	expiresStr := q.Get("expires")

	expires, err := time.Parse(time.RFC3339, expiresStr)
	var expInt int64
	if err != nil {
		var parseErr error
		expInt, parseErr = time.Now().Add(10*time.Minute).Unix(), nil
		require.NoError(t, parseErr)
	} else {
		expInt = expires.Unix()
	}

	err = signer.VerifyPath("/uploads/test.png", expInt, sig)
	assert.NoError(t, err)
}
