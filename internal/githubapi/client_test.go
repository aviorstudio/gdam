package githubapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadVerifiedReleaseAsset(t *testing.T) {
	body := []byte("verified archive")
	id, server := testIdentityServer(t, body, func(*ReleaseIdentity) {})
	defer server.Close()
	c := NewClient("")
	c.apiBaseURL = server.URL
	dest := filepath.Join(t.TempDir(), "asset.zip")
	if err := c.DownloadVerifiedReleaseAsset(context.Background(), "dev", "addon", id, dest); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(body) {
		t.Fatalf("got %q", got)
	}
}

func TestDownloadVerifiedReleaseAssetFailsClosedOnDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ReleaseIdentity)
		want   string
	}{
		{"moved tag", func(i *ReleaseIdentity) { i.TagName = "moved" }, "release identity drift"},
		{"changed commit", func(i *ReleaseIdentity) { i.CommitSHA = strings.Repeat("b", 40) }, "commit drift"},
		{"replaced asset", func(i *ReleaseIdentity) { i.AssetName = "replaced.zip" }, "asset drift"},
		{"changed digest", func(i *ReleaseIdentity) { i.Digest = "sha256:" + strings.Repeat("0", 64) }, "asset drift"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, server := testIdentityServer(t, []byte("archive"), func(*ReleaseIdentity) {})
			defer server.Close()
			tt.mutate(&id)
			c := NewClient("")
			c.apiBaseURL = server.URL
			err := c.DownloadVerifiedReleaseAsset(context.Background(), "dev", "addon", id, filepath.Join(t.TempDir(), "x"))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestDownloadVerifiedReleaseAssetDetectsTruncationAndOversize(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length int64
		want   string
	}{
		{"truncated", 99, "truncated"},
		{"oversized", MaxDownloadBytes + 1, "download limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, server := testIdentityServer(t, []byte("short"), func(*ReleaseIdentity) {})
			defer server.Close()
			base := server.Config.Handler
			server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") == "application/octet-stream" {
					w.Header().Set("Content-Length", fmt.Sprint(tc.length))
					_, _ = w.Write([]byte("short"))
					return
				}
				base.ServeHTTP(w, r)
			})
			c := NewClient("")
			c.apiBaseURL = server.URL
			err := c.DownloadVerifiedReleaseAsset(context.Background(), "dev", "addon", id, filepath.Join(t.TempDir(), "x"))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func testIdentityServer(t *testing.T, body []byte, mutateServer func(*ReleaseIdentity)) (ReleaseIdentity, *httptest.Server) {
	t.Helper()
	sum := sha256.Sum256(body)
	id := ReleaseIdentity{ReleaseID: 12, TagName: "Release-1", CommitSHA: strings.Repeat("a", 40), AssetID: 34, AssetName: "addon.zip", Digest: "sha256:" + fmt.Sprintf("%x", sum), PublishedAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)}
	serverID := id
	mutateServer(&serverID)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/dev/addon/releases/12":
			fmt.Fprintf(w, `{"id":12,"tag_name":%q,"published_at":%q,"prerelease":false,"draft":false}`, serverID.TagName, serverID.PublishedAt.Format(time.RFC3339))
		case r.URL.Path == "/repos/dev/addon/commits/Release-1":
			fmt.Fprintf(w, `{"sha":%q}`, serverID.CommitSHA)
		case r.URL.Path == "/repos/dev/addon/releases/assets/34" && r.Header.Get("Accept") == "application/octet-stream":
			_, _ = w.Write(body)
		case r.URL.Path == "/repos/dev/addon/releases/assets/34":
			fmt.Fprintf(w, `{"id":34,"name":%q,"digest":%q,"state":"uploaded"}`, serverID.AssetName, serverID.Digest)
		default:
			http.NotFound(w, r)
		}
	})
	s := httptest.NewServer(h)
	return id, s
}

func TestReadReleaseIdentity(t *testing.T) {
	for _, tc := range []struct {
		name      string
		draft     bool
		assets    int
		digest    string
		wantError bool
	}{
		{"published", false, 1, "sha256:" + strings.Repeat("b", 64), false},
		{"draft", true, 1, "sha256:" + strings.Repeat("b", 64), true},
		{"ambiguous assets", false, 2, "sha256:" + strings.Repeat("b", 64), true},
		{"missing digest", false, 1, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer github-token" {
					t.Error("missing publisher GitHub authorization")
				}
				w.Header().Set("Content-Type", "application/json")
				if strings.Contains(r.URL.Path, "/commits/") {
					fmt.Fprintf(w, `{"sha":"%s"}`, strings.Repeat("a", 40))
					return
				}
				assets := []map[string]any{}
				for i := 0; i < tc.assets; i++ {
					assets = append(assets, map[string]any{"id": 43 + i, "name": "addon.zip", "digest": tc.digest, "size": 100, "state": "uploaded"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": 42, "tag_name": "v1", "published_at": "2026-10-10T00:00:00Z", "draft": tc.draft, "assets": assets})
			}))
			defer server.Close()
			client := NewClient("github-token")
			client.apiBaseURL = server.URL
			identity, size, err := client.ReadReleaseIdentity(context.Background(), "owner", "repo", "v1", "")
			if (err != nil) != tc.wantError {
				t.Fatalf("identity: %+v error %v", identity, err)
			}
			if !tc.wantError && (identity.ReleaseID != 42 || identity.AssetID != 43 || size != 100 || identity.CommitSHA != strings.Repeat("a", 40)) {
				t.Fatalf("facts: %+v size %d", identity, size)
			}
		})
	}
}
