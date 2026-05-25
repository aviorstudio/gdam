package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSave_DoesNotWriteSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gdam.json")

	m := New()
	m = UpsertAddon(m, "@user/addon", Addon{
		Repo:    "https://example.com",
		Version: "1.2.3",
		Link: &Link{
			Enabled: true,
			Path:    "~/dev/addon",
		},
	})

	if err := Save(p, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(b), "schemaVersion") {
		t.Fatalf("expected saved file not to include schemaVersion, got:\n%s", string(b))
	}
}

func TestLoadAndSave_PreservesAssetName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gdam.json")

	m := New()
	m = UpsertAddon(m, "@user/addon", Addon{
		Repo:      "https://example.com",
		Version:   "1.2.3",
		AssetName: "addon-release.zip",
	})

	if err := Save(p, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := loaded.Addons["@user/addon"].AssetName; got != "addon-release.zip" {
		t.Fatalf("expected asset name addon-release.zip, got %q", got)
	}
}

func TestLoad_RejectsLinkInGdamJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gdam.json")
	if err := os.WriteFile(p, []byte(`{"addons":{"@user/addon":{"repo":"https://example.com","version":"1.2.3","link":{"enabled":true,"path":"~/dev/addon"}}}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := Load(p); err == nil {
		t.Fatalf("expected error")
	} else if !strings.Contains(err.Error(), LinkFilename) {
		t.Fatalf("expected error to mention %s, got: %v", LinkFilename, err)
	}
}

func TestLoad_RejectsSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gdam.json")
	if err := os.WriteFile(p, []byte(`{"schemaVersion":"0.0.1","addons":{}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := Load(p); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoad_RejectsLegacyPathField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gdam.json")
	if err := os.WriteFile(p, []byte(`{"addons":{"@user/addon":{"repo":"https://example.com","version":"1.2.3","path":"~/dev/addon"}}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := Load(p); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadLinkManifest_RejectsUnknownLinkField(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, LinkFilename)
	if err := os.WriteFile(p, []byte(`{"addons":{"@user/addon":{"enabled":true,"path":"~/dev/addon","extra":true}}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := LoadLinkManifest(p); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadLinkManifest_RejectsLinkEnabledWithoutPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, LinkFilename)
	if err := os.WriteFile(p, []byte(`{"addons":{"@user/addon":{"enabled":true}}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := LoadLinkManifest(p); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadLinkManifest_RejectsLinkMissingEnabled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, LinkFilename)
	if err := os.WriteFile(p, []byte(`{"addons":{"@user/addon":{"path":"~/dev/addon"}}}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := LoadLinkManifest(p); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSave_WritesLinksToLinkManifestAndOmitsFromGdamJSON(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "gdam.json")

	m := New()
	m = UpsertAddon(m, "@user/addon", Addon{
		Repo:    "https://example.com",
		Version: "1.2.3",
		Link: &Link{
			Enabled: true,
			Path:    "~/dev/addon",
		},
	})

	if err := Save(manifestPath, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	gdamBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read gdam.json: %v", err)
	}
	if strings.Contains(string(gdamBytes), `"link"`) {
		t.Fatalf("expected gdam.json to not contain link config, got:\n%s", string(gdamBytes))
	}

	linkPath := filepath.Join(dir, LinkFilename)
	lm, err := LoadLinkManifest(linkPath)
	if err != nil {
		t.Fatalf("LoadLinkManifest: %v", err)
	}
	link, ok := lm.Addons["@user/addon"]
	if !ok {
		t.Fatalf("expected gdam.link.json entry for @user/addon")
	}
	if link.Enabled != true {
		t.Fatalf("expected enabled=true, got %v", link.Enabled)
	}
	if link.Path != "~/dev/addon" {
		t.Fatalf("expected path=~/dev/addon, got %q", link.Path)
	}
}

func TestLoad_MergesLinkManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "gdam.json")
	linkPath := filepath.Join(dir, LinkFilename)

	if err := os.WriteFile(manifestPath, []byte(`{"addons":{"@user/addon":{"repo":"https://example.com","version":"1.2.3"}}}`), 0o644); err != nil {
		t.Fatalf("write gdam.json: %v", err)
	}
	if err := os.WriteFile(linkPath, []byte(`{"addons":{"@user/addon":{"enabled":true,"path":"~/dev/addon"}}}`), 0o644); err != nil {
		t.Fatalf("write gdam.link.json: %v", err)
	}

	m, err := Load(manifestPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Addons["@user/addon"].Link == nil {
		t.Fatalf("expected link to be merged into manifest")
	}
	if got := m.Addons["@user/addon"].Link.Path; got != "~/dev/addon" {
		t.Fatalf("expected link.path=~/dev/addon, got %q", got)
	}
	if got := m.Addons["@user/addon"].Link.Enabled; got != true {
		t.Fatalf("expected link.enabled=true, got %v", got)
	}
}
