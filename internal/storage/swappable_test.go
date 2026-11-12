package storage

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubProvider struct {
	name string
	caps *ProviderCapabilities
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Save(context.Context, string, io.Reader, int64, string) error {
	return nil
}
func (s *stubProvider) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("ok")), nil
}
func (s *stubProvider) Delete(context.Context, string) error       { return nil }
func (s *stubProvider) Exists(context.Context, string) (bool, error) { return true, nil }
func (s *stubProvider) URL(_ context.Context, key string) (string, error) {
	return "/" + s.name + "/" + key, nil
}
func (s *stubProvider) Move(context.Context, string, string) error   { return nil }
func (s *stubProvider) DeletePrefix(context.Context, string) error   { return nil }
func (s *stubProvider) Capabilities() ProviderCapabilities {
	if s.caps != nil {
		return *s.caps
	}
	return ProviderCapabilities{Provider: s.name, SupportsHLS: true}
}

func TestSwappableProviderRegistryAndCapabilities(t *testing.T) {
	local := &stubProvider{
		name: "local",
		caps: &ProviderCapabilities{Provider: "local", SupportsHLS: true, SupportsSignedURLs: true},
	}
	cloud := &stubProvider{
		name: "cloudinary",
		caps: &ProviderCapabilities{Provider: "cloudinary", SupportsHLS: false, SupportsSignedURLs: true, SupportsTransforms: true},
	}

	sw := NewSwappable(local)
	assert.Equal(t, "local", sw.Name())
	assert.True(t, sw.Capabilities().SupportsHLS)

	// Switch active provider to cloudinary while local remains registered.
	sw.Swap(cloud)
	assert.Equal(t, "cloudinary", sw.Name())
	assert.False(t, sw.Capabilities().SupportsHLS)
	assert.True(t, sw.Capabilities().SupportsTransforms)

	resolvedLocal, ok := sw.ResolveProvider("local")
	require.True(t, ok)
	assert.Equal(t, "local", resolvedLocal.Name())

	resolvedCloud, ok := sw.ResolveProvider("cloudinary")
	require.True(t, ok)
	assert.Equal(t, "cloudinary", resolvedCloud.Name())

	_, missing := sw.ResolveProvider("unknown")
	assert.False(t, missing)

	// Register another provider without swapping active driver.
	s3p := &stubProvider{name: "s3"}
	sw.RegisterProvider(s3p)
	assert.Equal(t, "cloudinary", sw.Name())
	resolvedS3, ok := sw.ResolveProvider("s3")
	require.True(t, ok)
	assert.Equal(t, "s3", resolvedS3.Name())
}

