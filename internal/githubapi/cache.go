package githubapi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (c *Client) cachePath(digest string) (string, error) {
	want, err := parseSHA256(digest)
	if err != nil {
		return "", err
	}
	if c.cacheDir == "" {
		return "", fmt.Errorf("offline install requires an archive cache")
	}
	return filepath.Join(c.cacheDir, hex.EncodeToString(want)+".zip"), nil
}

func (c *Client) copyCached(digest, destination string) error {
	path, err := c.cachePath(digest)
	if err != nil {
		return err
	}
	return copyDigestFile(path, destination, digest)
}

func (c *Client) storeCached(digest, source string) error {
	path, err := c.cachePath(digest)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.cacheDir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(c.cacheDir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("archive cache must be a real directory")
	}
	return copyDigestFile(source, path, digest)
}

// Rehash on every read, including offline. Publish through a private temporary
// file so cancellation, corruption and concurrent writers cannot expose a
// partial archive. Never follow a cache-entry symlink.
func copyDigestFile(source, destination, digest string) error {
	want, err := parseSHA256(digest)
	if err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxDownloadBytes {
		return fmt.Errorf("verified archive unavailable in cache")
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".gdam-archive-*")
	if err != nil {
		return err
	}
	defer os.Remove(output.Name())
	defer output.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, MaxDownloadBytes+1))
	if err != nil {
		return err
	}
	if n > MaxDownloadBytes || !equalBytes(hash.Sum(nil), want) {
		return fmt.Errorf("cached archive digest differs from lock")
	}
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return os.Rename(output.Name(), destination)
}
