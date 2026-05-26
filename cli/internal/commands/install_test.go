package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aviorstudio/gdam/cli/internal/manifest"
)

func TestInstall_ReplacesExistingAddonDir(t *testing.T) {
	withFakeRegistryInstall(t)

	projectDir := t.TempDir()

	m := manifest.New()
	m = manifest.UpsertAddon(m, "@user/addon", manifest.Addon{Version: "1.2.3"})
	if err := manifest.Save(filepath.Join(projectDir, "gdam.json"), m); err != nil {
		t.Fatalf("write gdam.json: %v", err)
	}

	addonDirName, err := addonDirNameForPluginKey("@user/addon")
	if err != nil {
		t.Fatalf("addonDirNameForPluginKey: %v", err)
	}
	dst := filepath.Join(projectDir, "addons", addonDirName)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir addons dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
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

	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatalf("install: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dst, "keep.txt")); err == nil {
		t.Fatalf("expected stale addon content to be removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat keep file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "plugin.cfg")); err != nil {
		t.Fatalf("expected fresh addon content, stat: %v", err)
	}
}

func TestInstall_InstallsMissingAddonFromRegistry(t *testing.T) {
	withFakeRegistryInstall(t)

	projectDir := t.TempDir()

	m := manifest.New()
	m = manifest.UpsertAddon(m, "@user/addon", manifest.Addon{Version: "1.2.3"})
	if err := manifest.Save(filepath.Join(projectDir, "gdam.json"), m); err != nil {
		t.Fatalf("write gdam.json: %v", err)
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

	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatalf("install: %v", err)
	}

	addonDirName, err := addonDirNameForPluginKey("@user/addon")
	if err != nil {
		t.Fatalf("addonDirNameForPluginKey: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "addons", addonDirName, "plugin.cfg")); err != nil {
		t.Fatalf("expected installed plugin.cfg, stat: %v", err)
	}
}

func TestInstall_ReplacesManagedAddonButKeepsUnmanagedAddons(t *testing.T) {
	withFakeRegistryInstall(t)

	projectDir := t.TempDir()

	m := manifest.New()
	m = manifest.UpsertAddon(m, "@user/addon", manifest.Addon{Version: "1.2.3"})
	if err := manifest.Save(filepath.Join(projectDir, "gdam.json"), m); err != nil {
		t.Fatalf("write gdam.json: %v", err)
	}

	pluginAddonDirName, err := addonDirNameForPluginKey("@user/addon")
	if err != nil {
		t.Fatalf("addonDirNameForPluginKey: %v", err)
	}

	pluginAddonDir := filepath.Join(projectDir, "addons", pluginAddonDirName)
	if err := os.MkdirAll(pluginAddonDir, 0o755); err != nil {
		t.Fatalf("mkdir addon addon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginAddonDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

	oldAddonDir := filepath.Join(projectDir, "addons", "@old_plugin")
	if err := os.MkdirAll(oldAddonDir, 0o755); err != nil {
		t.Fatalf("mkdir old addon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldAddonDir, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	unmanagedAddonDir := filepath.Join(projectDir, "addons", "manual_plugin")
	if err := os.MkdirAll(unmanagedAddonDir, 0o755); err != nil {
		t.Fatalf("mkdir unmanaged addon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unmanagedAddonDir, "manual.txt"), []byte("manual"), 0o644); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	in := "config_version=5\n\n[editor_plugins]\nenabled=PackedStringArray(\"res://addons/@old_plugin/plugin.cfg\", \"res://addons/" + pluginAddonDirName + "/plugin.cfg\")\n"
	if err := os.WriteFile(projectGodotPath, []byte(in), 0o644); err != nil {
		t.Fatalf("write project.godot: %v", err)
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

	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatalf("install: %v", err)
	}

	if _, err := os.Stat(filepath.Join(pluginAddonDir, "keep.txt")); err == nil {
		t.Fatalf("expected managed addon stale file to be removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat stale file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginAddonDir, "plugin.cfg")); err != nil {
		t.Fatalf("expected managed addon to be freshly installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldAddonDir, "old.txt")); err != nil {
		t.Fatalf("expected managed addon to be kept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unmanagedAddonDir, "manual.txt")); err != nil {
		t.Fatalf("expected unmanaged addon to be kept: %v", err)
	}

	outBytes, err := os.ReadFile(projectGodotPath)
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	if got := string(outBytes); got != in {
		t.Fatalf("expected project.godot unchanged, got:\n%s", got)
	}
}
