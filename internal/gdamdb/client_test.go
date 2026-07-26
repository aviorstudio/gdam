package gdamdb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveAddonRequestsTheApi(t *testing.T) {
	var gotPath, gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"name": "@dev/cool",
			"repo": "https://github.com/dev/cool",
			"github_owner": "dev",
			"github_repo": "cool",
			"version": "1.2.3",
			"release_tag": "v1.2.3",
			"asset_name": "cool.zip",
			"editor_plugin": true
		}`)
	}))
	defer server.Close()

	resolved, err := NewClient(server.URL).ResolveAddon(context.Background(), "Dev", "cool", "1.2.3")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if gotPath != "/api/v1/resolve/dev/cool" {
		t.Fatalf("got path %q", gotPath)
	}
	if gotQuery != "version=1.2.3" {
		t.Fatalf("got query %q", gotQuery)
	}
	if resolved.GitHubOwner != "dev" || resolved.GitHubRepo != "cool" {
		t.Fatalf("got %s/%s", resolved.GitHubOwner, resolved.GitHubRepo)
	}
	if resolved.Version != "1.2.3" || resolved.AssetName != "cool.zip" || !resolved.EditorPlugin {
		t.Fatalf("unexpected resolution: %+v", resolved)
	}
}

func TestResolveAddonOmitsEmptyVersion(t *testing.T) {
	var gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = io.WriteString(w, `{"name":"@dev/cool"}`)
	}))
	defer server.Close()

	if _, err := NewClient(server.URL).ResolveAddon(context.Background(), "dev", "cool", "  "); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// An empty version means "latest"; sending version= would ask the API to
	// match a release literally named the empty string.
	if gotQuery != "" {
		t.Fatalf("got query %q, want none", gotQuery)
	}
}

func TestResolveAddonSurfacesApiMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"addon @dev/missing"}`)
	}))
	defer server.Close()

	_, err := NewClient(server.URL).ResolveAddon(context.Background(), "dev", "missing", "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Error() != "addon @dev/missing" {
		t.Fatalf("got %q, want the API's own message", err.Error())
	}
}

func TestResolveAddonRejectsEmptySpec(t *testing.T) {
	// No request should be made at all for an unusable spec.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("client should not have called the API")
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if _, err := client.ResolveAddon(context.Background(), "", "cool", ""); err == nil {
		t.Fatal("expected an error for an empty owner")
	}
	if _, err := client.ResolveAddon(context.Background(), "dev", "  ", ""); err == nil {
		t.Fatal("expected an error for an empty addon")
	}
}

func TestPublishReleasePostsSemver(t *testing.T) {
	var payload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/publish" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	err := NewClient(server.URL).PublishRelease(context.Background(), PublishReleaseInput{
		SecretKey:  "gdam_sk_test",
		Owner:      "dev",
		Addon:      "cool",
		Major:      1,
		Minor:      0,
		Patch:      2,
		ReleaseTag: "v1.0.2",
		AssetName:  "cool.zip",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The API takes a version string, so the triplet has to be joined here.
	if payload["version"] != "1.0.2" {
		t.Fatalf("got version %v, want 1.0.2", payload["version"])
	}
	if payload["secret_key"] != "gdam_sk_test" {
		t.Fatalf("secret key was not sent")
	}
	if payload["release_tag"] != "v1.0.2" || payload["asset_name"] != "cool.zip" {
		t.Fatalf("unexpected payload: %v", payload)
	}
}

func TestPublishReleaseSurfacesApiMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"secret key cannot publish to @other"}`)
	}))
	defer server.Close()

	err := NewClient(server.URL).PublishRelease(context.Background(), PublishReleaseInput{SecretKey: "k"})
	if err == nil || !strings.Contains(err.Error(), "cannot publish to @other") {
		t.Fatalf("got %v", err)
	}
}

func TestClientRequiresBaseURL(t *testing.T) {
	_, err := NewClient("  ").ResolveAddon(context.Background(), "dev", "cool", "")
	if err == nil || !strings.Contains(err.Error(), "GDAM_API_URL") {
		t.Fatalf("got %v, want a message naming GDAM_API_URL", err)
	}
}
