package gdamdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const resolvedJSON = `{"name":"@dev/cool","repo":"https://github.com/dev/cool","github_owner":"dev","github_repo":"cool","tag_name":"Release-1","github_release_id":123,"commit_sha":"0123456789012345678901234567890123456789","asset_id":456,"asset_name":"cool.zip","asset_digest":"sha256:0123456789012345678901234567890123456789012345678901234567890123","published_at":"2026-08-26T10:00:00Z","prerelease":false,"editor_plugin":true}`
const releasesJSON = `[{"github_release_id":123,"tag_name":"Release-1","commit_sha":"0123456789012345678901234567890123456789","asset_id":456,"asset_name":"cool.zip","asset_digest":"sha256:0123456789012345678901234567890123456789012345678901234567890123","published_at":"2026-08-26T10:00:00Z","prerelease":false}]`

func serveResolved(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/releases") {
		_, _ = io.WriteString(w, releasesJSON)
		return
	}
	_, _ = io.WriteString(w, resolvedJSON)
}

func TestResolveAddonUsesExactTagContract(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases") {
			query = r.URL.RawQuery
		}
		serveResolved(w, r)
	}))
	defer server.Close()
	got, err := NewClient(server.URL).ResolveAddon(context.Background(), "Dev", "cool", "Release-1")
	if err != nil {
		t.Fatal(err)
	}
	if query != "tag=Release-1" || got.TagName != "Release-1" || got.AssetID != 456 {
		t.Fatalf("query=%q response=%+v", query, got)
	}
}

func TestResolveAddonOmitsEmptyTag(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases") {
			query = r.URL.RawQuery
		}
		serveResolved(w, r)
	}))
	defer server.Close()
	if _, err := NewClient(server.URL).ResolveAddon(context.Background(), "dev", "cool", " "); err != nil {
		t.Fatal(err)
	}
	if query != "" {
		t.Fatalf("query %q", query)
	}
}

func TestResolveAddonRejectsIncompleteIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{"name":"@dev/cool"}`) }))
	defer server.Close()
	_, err := NewClient(server.URL).ResolveAddon(context.Background(), "dev", "cool", "")
	if err == nil || !strings.Contains(err.Error(), "missing verified release identity") {
		t.Fatalf("got %v", err)
	}
}

func TestResolveAddonSurfacesAPIMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"message":"addon missing"}`)
	}))
	defer server.Close()
	_, err := NewClient(server.URL).ResolveAddon(context.Background(), "dev", "cool", "")
	if err == nil || err.Error() != "addon missing" {
		t.Fatalf("got %v", err)
	}
}

func TestPublishReleasePostsTagOnly(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.WriteHeader(201)
	}))
	defer server.Close()
	err := NewClient(server.URL).PublishRelease(context.Background(), PublishReleaseInput{SecretKey: "secret", Owner: "dev", Addon: "cool", TagName: "Release-1", AssetName: "cool.zip"})
	if err != nil {
		t.Fatal(err)
	}
	if payload["tag_name"] != "Release-1" || payload["asset_name"] != "cool.zip" {
		t.Fatalf("%v", payload)
	}
	if _, ok := payload["version"]; ok {
		t.Fatalf("legacy version sent: %v", payload)
	}
	if _, ok := payload["release_tag"]; ok {
		t.Fatalf("legacy release_tag sent: %v", payload)
	}
}

func TestClerkPublishSendsCompleteFactsAndBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/publish" || r.Header.Get("Authorization") != "Bearer ak_secret" {
			t.Error("wrong publish authentication")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["secret_key"]; ok {
			t.Error("Clerk secret duplicated in request body")
		}
		if body["github_release_id"] != float64(42) || body["asset_id"] != float64(43) || body["asset_size"] != float64(100) || body["commit_sha"] != strings.Repeat("a", 40) {
			t.Errorf("missing release facts: %+v", body)
		}
		w.WriteHeader(201)
	}))
	defer server.Close()
	err := NewClient(server.URL).PublishRelease(context.Background(), PublishReleaseInput{SecretKey: "ak_secret", Owner: "owner", Addon: "addon", TagName: "v1", AssetName: "addon.zip", ReleaseID: 42, CommitSHA: strings.Repeat("a", 40), AssetID: 43, AssetDigest: "sha256:" + strings.Repeat("b", 64), AssetSize: 100, PublishedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
}
