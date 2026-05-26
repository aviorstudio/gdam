package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aviorstudio/gdam/cli/internal/manifest"
)

func TestAdd_ReplacesExistingUnmanagedAddonDir(t *testing.T) {
	withFakeRegistryInstall(t)

	projectDir := t.TempDir()
	m := manifest.New()
	if err := manifest.Save(filepath.Join(projectDir, "gdam.json"), m); err != nil {
		t.Fatalf("write gdam.json: %v", err)
	}

	addonDirName, err := addonDirNameForPluginKey("@user/addon")
	if err != nil {
		t.Fatalf("addonDirNameForPluginKey: %v", err)
	}
	dst := filepath.Join(projectDir, "addons", addonDirName)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir stale addon dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := Add(context.Background(), AddOptions{Spec: "@user/addon@1.2.3"}); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "stale.txt")); err == nil {
		t.Fatalf("expected stale addon content to be removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat stale file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "plugin.cfg")); err != nil {
		t.Fatalf("expected fresh addon content, stat: %v", err)
	}

	loaded, err := manifest.Load(filepath.Join(projectDir, "gdam.json"))
	if err != nil {
		t.Fatalf("load gdam.json: %v", err)
	}
	if got := loaded.Addons["@user/addon"].Version; got != "1.2.3" {
		t.Fatalf("expected version 1.2.3, got %q", got)
	}
}
