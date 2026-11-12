package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-cms/cms/internal/storage"
)

// Provider implements StorageProvider for the local filesystem.
type Provider struct {
	baseDir   string
	urlPrefix string
}

// New creates a new local filesystem storage provider.
func New(baseDir, urlPrefix string) (*Provider, error) {
	if baseDir == "" {
		baseDir = "./uploads"
	}
	if urlPrefix == "" {
		urlPrefix = "/uploads"
	}

	absDir, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path for uploads directory %q: %w", baseDir, err)
	}

	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to initialize storage directory %q: %w", absDir, err)
	}

	return &Provider{
		baseDir:   absDir,
		urlPrefix: strings.TrimRight(urlPrefix, "/"),
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "local"
}

// BaseDir returns the root storage directory on the host filesystem.
func (p *Provider) BaseDir() string {
	return p.baseDir
}

// resolvePath safely resolves a storage key to an absolute host filesystem path.
func (p *Provider) resolvePath(key string) (string, error) {
	if strings.Contains(key, "\x00") {
		return "", storage.ErrPathTraversal
	}

	// Clean key and handle slash conversions
	cleanKey := filepath.Clean(filepath.FromSlash(key))
	if strings.HasPrefix(cleanKey, "..") || filepath.IsAbs(cleanKey) || strings.HasPrefix(cleanKey, string(filepath.Separator)) {
		return "", storage.ErrPathTraversal
	}

	fullPath := filepath.Join(p.baseDir, cleanKey)

	// Guard against path traversal escaping root
	if !strings.HasPrefix(fullPath, p.baseDir+string(filepath.Separator)) && fullPath != p.baseDir {
		return "", storage.ErrPathTraversal
	}

	return fullPath, nil
}

// Save writes data from reader into the local filesystem at key.
func (p *Provider) Save(ctx context.Context, key string, r io.Reader, size int64, mimeType string) error {
	fullPath, err := p.resolvePath(key)
	if err != nil {
		return err
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dir, err)
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", fullPath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		_ = os.Remove(fullPath)
		return fmt.Errorf("failed to write content to %q: %w", fullPath, err)
	}

	return nil
}

// Open opens the file identified by key for reading.
func (p *Provider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath, err := p.resolvePath(key)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrFileNotFound
		}
		return nil, fmt.Errorf("failed to open file %q: %w", fullPath, err)
	}

	return file, nil
}

// Delete removes the file identified by key from the filesystem.
func (p *Provider) Delete(ctx context.Context, key string) error {
	fullPath, err := p.resolvePath(key)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return storage.ErrFileNotFound
		}
		return fmt.Errorf("failed to delete file %q: %w", fullPath, err)
	}

	return nil
}

// Exists checks if the file exists on the filesystem.
func (p *Provider) Exists(ctx context.Context, key string) (bool, error) {
	fullPath, err := p.resolvePath(key)
	if err != nil {
		return false, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return !info.IsDir(), nil
}

// URL generates the public relative URL to access this asset.
func (p *Provider) URL(ctx context.Context, key string) (string, error) {
	cleanKey := strings.TrimPrefix(filepath.ToSlash(key), "/")
	return fmt.Sprintf("%s/%s", p.urlPrefix, cleanKey), nil
}

// Move renames or moves a file or directory from srcKey to dstKey on the filesystem.
func (p *Provider) Move(ctx context.Context, srcKey, dstKey string) error {
	srcPath, err := p.resolvePath(srcKey)
	if err != nil {
		return err
	}
	dstPath, err := p.resolvePath(dstKey)
	if err != nil {
		return err
	}

	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return storage.ErrFileNotFound
		}
		return err
	}

	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory %q: %w", dstDir, err)
	}

	if err := os.Rename(srcPath, dstPath); err != nil {
		// If rename fails (e.g. across mount points), fall back to copy and remove
		if copyErr := p.copyRecursively(srcPath, dstPath); copyErr != nil {
			return fmt.Errorf("failed to move %q to %q: %w", srcPath, dstPath, copyErr)
		}
		_ = os.RemoveAll(srcPath)
	}

	return nil
}

// copyRecursively copies files or directories from src to dst.
func (p *Provider) copyRecursively(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if err := os.MkdirAll(dst, 0755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			s := filepath.Join(src, entry.Name())
			d := filepath.Join(dst, entry.Name())
			if err := p.copyRecursively(s, d); err != nil {
				return err
			}
		}
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// DeletePrefix deletes all files and directories matching prefix.
func (p *Provider) DeletePrefix(ctx context.Context, prefix string) error {
	cleanPrefix := strings.Trim(filepath.ToSlash(prefix), "/")
	if cleanPrefix == "" || cleanPrefix == "." {
		return fmt.Errorf("refusing to delete empty prefix / root storage")
	}

	fullPath, err := p.resolvePath(prefix)
	if err != nil {
		return err
	}

	// If the path exists as directory or file, remove it directly
	if _, err := os.Stat(fullPath); err == nil {
		return os.RemoveAll(fullPath)
	}

	// Also check if prefix is a file prefix (e.g. thumb pattern) inside parent
	dir := filepath.Dir(fullPath)
	base := filepath.Base(fullPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), base) {
			_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
		}
	}

	return nil
}

// CheckHealth verifies the configured local uploads directory exists and is writable.
func (p *Provider) CheckHealth(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	info, err := os.Stat(p.baseDir)
	if err != nil {
		return fmt.Errorf("local storage directory unavailable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local storage path is not a directory: %s", p.baseDir)
	}
	f, err := os.CreateTemp(p.baseDir, ".cms_healthcheck_*")
	if err != nil {
		return fmt.Errorf("local storage directory not writable: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}
