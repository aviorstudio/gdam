package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aviorstudio/gdam/cli/internal/manifest"
)

func TestUnlinkAll_RemovesSymlinksWhenNoRepo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (symlink/junction behavior varies by environment)")
	}

	projectDir := t.TempDir()

	pluginDirA := filepath.Join(projectDir, "local_plugin_a")
	if err := os.MkdirAll(pluginDirA, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDirA, "plugin.cfg"), []byte("[plugin]\nname=\"TestA\"\n"), 0o644); err != nil {
		t.Fatalf("write plugin.cfg: %v", err)
	}

	pluginDirB := filepath.Join(projectDir, "local_plugin_b")
	if err := os.MkdirAll(pluginDirB, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDirB, "plugin.cfg"), []byte("[plugin]\nname=\"TestB\"\n"), 0o644); err != nil {
		t.Fatalf("write plugin.cfg: %v", err)
	}

	pluginKeyA := "@user/plugin_a"
	pluginKeyB := "@user/plugin_b"

	m := manifest.New()
	m = manifest.UpsertPlugin(m, pluginKeyA, manifest.Plugin{
		EditorPlugin: true,
		Link: &manifest.Link{
			Enabled: true,
			Path:    pluginDirA,
		},
	})
	m = manifest.UpsertPlugin(m, pluginKeyB, manifest.Plugin{
		EditorPlugin: true,
		Link: &manifest.Link{
			Enabled: true,
			Path:    pluginDirB,
		},
	})
	gdamPath := filepath.Join(projectDir, "gdam.json")
	if err := manifest.Save(gdamPath, m); err != nil {
		t.Fatalf("write gdam.json: %v", err)
	}

	addonDirNameA, err := addonDirNameForPluginKey(pluginKeyA)
	if err != nil {
		t.Fatalf("addonDirNameForPluginKey: %v", err)
	}
	addonDirNameB, err := addonDirNameForPluginKey(pluginKeyB)
	if err != nil {
		t.Fatalf("addonDirNameForPluginKey: %v", err)
	}
	dstA := filepath.Join(projectDir, "addons", addonDirNameA)
	dstB := filepath.Join(projectDir, "addons", addonDirNameB)
	if err := os.MkdirAll(filepath.Dir(dstA), 0o755); err != nil {
		t.Fatalf("mkdir addons: %v", err)
	}
	if err := os.Symlink(pluginDirA, dstA); err != nil {
		t.Fatalf("symlink dstA: %v", err)
	}
	if err := os.Symlink(pluginDirB, dstB); err != nil {
		t.Fatalf("symlink dstB: %v", err)
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	in := "config_version=5\n\n[editor_plugins]\n" +
		"enabled=PackedStringArray(" +
		"\"res://addons/" + addonDirNameA + "/plugin.cfg\", " +
		"\"res://addons/" + addonDirNameB + "/plugin.cfg\"" +
		")\n"
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

	if err := UnlinkAll(context.Background(), UnlinkAllOptions{}); err != nil {
		t.Fatalf("unlink --all: %v", err)
	}

	if _, err := os.Lstat(dstA); err == nil {
		t.Fatalf("expected dstA to be removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("lstat dstA: %v", err)
	}
	if _, err := os.Lstat(dstB); err == nil {
		t.Fatalf("expected dstB to be removed")
	} else if !os.IsNotExist(err) {
		t.Fatalf("lstat dstB: %v", err)
	}

	outBytes, err := os.ReadFile(projectGodotPath)
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	out := string(outBytes)
	if strings.Contains(out, "res://addons/"+addonDirNameA+"/plugin.cfg") {
		t.Fatalf("expected %s to be disabled in project.godot, got:\n%s", pluginKeyA, out)
	}
	if strings.Contains(out, "res://addons/"+addonDirNameB+"/plugin.cfg") {
		t.Fatalf("expected %s to be disabled in project.godot, got:\n%s", pluginKeyB, out)
	}

	m2, err := manifest.Load(gdamPath)
	if err != nil {
		t.Fatalf("read gdam.json: %v", err)
	}
	if _, ok := m2.Addons[pluginKeyA]; !ok {
		t.Fatalf("expected plugin A to remain in gdam.json")
	}
	if _, ok := m2.Addons[pluginKeyB]; !ok {
		t.Fatalf("expected plugin B to remain in gdam.json")
	}

	gdamBytes, err := os.ReadFile(gdamPath)
	if err != nil {
		t.Fatalf("read gdam.json bytes: %v", err)
	}
	if strings.Contains(string(gdamBytes), `"link"`) {
		t.Fatalf("expected gdam.json to not contain link config, got:\n%s", string(gdamBytes))
	}

	linkManifestPath := filepath.Join(projectDir, manifest.LinkFilename)
	lm, err := manifest.LoadLinkManifest(linkManifestPath)
	if err != nil {
		t.Fatalf("read gdam.link.json: %v", err)
	}

	linkA, ok := lm.Addons[pluginKeyA]
	if !ok {
		t.Fatalf("expected gdam.link.json entry for %s", pluginKeyA)
	}
	if linkA.Enabled {
		t.Fatalf("expected gdam.link.json enabled=false for %s", pluginKeyA)
	}
	if got := linkA.Path; got != pluginDirA {
		t.Fatalf("expected gdam.link.json path %q for %s, got %q", pluginDirA, pluginKeyA, got)
	}

	linkB, ok := lm.Addons[pluginKeyB]
	if !ok {
		t.Fatalf("expected gdam.link.json entry for %s", pluginKeyB)
	}
	if linkB.Enabled {
		t.Fatalf("expected gdam.link.json enabled=false for %s", pluginKeyB)
	}
	if got := linkB.Path; got != pluginDirB {
		t.Fatalf("expected gdam.link.json path %q for %s, got %q", pluginDirB, pluginKeyB, got)
	}
}
