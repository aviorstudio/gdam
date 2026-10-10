package githubapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOfflineCacheRehashesWithoutContactingProvider(t *testing.T) {
	cache := t.TempDir()
	archive := []byte("reviewed archive fixture")
	sum := sha256.Sum256(archive)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	path := filepath.Join(cache, hex.EncodeToString(sum[:])+".zip")
	if err := os.WriteFile(path, archive, 0600); err != nil {
		t.Fatal(err)
	}
	client := NewClient("").WithCache(cache, true)
	client.apiBaseURL = "http://127.0.0.1:1" // any network request would fail
	identity := ReleaseIdentity{ReleaseID: 1, AssetID: 2, TagName: "v1", CommitSHA: "reviewed", AssetName: "addon.zip", Digest: digest, PublishedAt: time.Now()}
	destination := filepath.Join(t.TempDir(), "download.zip")
	if err := client.DownloadVerifiedReleaseAsset(context.Background(), "owner", "repo", identity, destination); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := client.DownloadVerifiedReleaseAsset(context.Background(), "owner", "repo", identity, destination); err == nil {
		t.Fatal("offline cache corruption accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destination, path); err != nil {
		t.Fatal(err)
	}
	if err := client.DownloadVerifiedReleaseAsset(context.Background(), "owner", "repo", identity, destination); err == nil {
		t.Fatal("cache symlink accepted")
	}
}
