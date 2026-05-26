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

func TestLink_ReplacesLegacyEditorPluginEntryForSameLocalPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (symlink/junction behavior varies by environment)")
	}
	withResolvedEditorPlugin(t)

	projectDir := t.TempDir()

	pluginDir := filepath.Join(projectDir, "local_plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.cfg"), []byte("[plugin]\nname=\"Test\"\n"), 0o644); err != nil {
		t.Fatalf("write plugin.cfg: %v", err)
	}

	m := manifest.New()
	m = manifest.UpsertAddon(m, "@local_plugin", manifest.Addon{
		Link: &manifest.Link{
			Enabled: true,
			Path:    pluginDir,
		},
	})
	m = manifest.UpsertAddon(m, "@user/addon", manifest.Addon{Version: "1.2.3"})
	if err := manifest.Save(filepath.Join(projectDir, "gdam.json"), m); err != nil {
		t.Fatalf("write gdam.json: %v", err)
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	in := "config_version=5\n\n[editor_plugins]\nenabled=PackedStringArray(\"res://addons/@local_plugin/plugin.cfg\", \"res://addons/other/plugin.cfg\")\n"
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

	if err := Link(context.Background(), LinkOptions{
		Spec: "@user/addon",
		Path: pluginDir,
	}); err != nil {
		t.Fatalf("link: %v", err)
	}

	outBytes, err := os.ReadFile(projectGodotPath)
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	out := string(outBytes)

	if strings.Contains(out, "res://addons/@local_plugin/plugin.cfg") {
		t.Fatalf("expected legacy enabled entry removed, got:\n%s", out)
	}
	if !strings.Contains(out, "res://addons/other/plugin.cfg") {
		t.Fatalf("expected unrelated enabled entry retained, got:\n%s", out)
	}
	if !strings.Contains(out, "res://addons/@user_addon/plugin.cfg") {
		t.Fatalf("expected new enabled entry added, got:\n%s", out)
	}
}

func TestLink_OverwritesExistingAddonsDirWhenNotInManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (symlink/junction behavior varies by environment)")
	}

	projectDir := t.TempDir()

	pluginDir := filepath.Join(projectDir, "local_plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.cfg"), []byte("[plugin]\nname=\"Test\"\n"), 0o644); err != nil {
		t.Fatalf("write plugin.cfg: %v", err)
	}

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
		t.Fatalf("mkdir existing addon dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dst, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
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

	if err := Link(context.Background(), LinkOptions{
		Spec: "@user/addon",
		Path: pluginDir,
	}); err != nil {
		t.Fatalf("link: %v", err)
	}

	if info, err := os.Lstat(dst); err != nil {
		t.Fatalf("lstat dst: %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected dst to be symlink, got mode %v", info.Mode())
	}

	gdamPath := filepath.Join(projectDir, "gdam.json")
	m2, err := manifest.Load(gdamPath)
	if err != nil {
		t.Fatalf("read gdam.json: %v", err)
	}
	if _, ok := m2.Addons["@user/addon"]; !ok {
		t.Fatalf("expected addon entry to exist in gdam.json")
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
	link, ok := lm.Addons["@user/addon"]
	if !ok {
		t.Fatalf("expected gdam.link.json entry for @user/addon")
	}
	if got := link.Path; got != pluginDir {
		t.Fatalf("expected gdam.link.json path %q, got %q", pluginDir, got)
	}
	if got := link.Enabled; got != true {
		t.Fatalf("expected gdam.link.json enabled=true, got %v", got)
	}
}

func TestLink_DisablesLegacyEditorPluginEntryDerivedFromPath(t *testing.T) {
	withResolvedEditorPlugin(t)

	projectDir := t.TempDir()

	pluginDir := filepath.Join(projectDir, "gd-playwright-emitter")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.cfg"), []byte("[plugin]\nname=\"Test\"\n"), 0o644); err != nil {
		t.Fatalf("write plugin.cfg: %v", err)
	}

	m := manifest.New()
	m = manifest.UpsertAddon(m, "@aviorstudio/gd-playwright", manifest.Addon{Version: "1.2.3"})
	if err := manifest.Save(filepath.Join(projectDir, "gdam.json"), m); err != nil {
		t.Fatalf("write gdam.json: %v", err)
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	in := "config_version=5\n\n[autoload]\nPlaywrightService=\"*res://addons/@gd-playwright-emitter/autoload.gd\"\n\n[editor_plugins]\nenabled=PackedStringArray(\"res://addons/@gd-playwright-emitter/plugin.cfg\")\n"
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

	if err := Link(context.Background(), LinkOptions{
		Spec: "@aviorstudio/gd-playwright",
		Path: pluginDir,
	}); err != nil {
		t.Fatalf("link: %v", err)
	}

	outBytes, err := os.ReadFile(projectGodotPath)
	if err != nil {
		t.Fatalf("read project.godot: %v", err)
	}
	out := string(outBytes)

	if strings.Contains(out, "res://addons/@gd-playwright-emitter/plugin.cfg") {
		t.Fatalf("expected legacy enabled entry removed, got:\n%s", out)
	}
	if !strings.Contains(out, "res://addons/@aviorstudio_gd-playwright/plugin.cfg") {
		t.Fatalf("expected new enabled entry added, got:\n%s", out)
	}
	if strings.Contains(out, "res://addons/@gd-playwright-emitter/autoload.gd") {
		t.Fatalf("expected legacy autoload path removed, got:\n%s", out)
	}
	if !strings.Contains(out, "res://addons/@aviorstudio_gd-playwright/autoload.gd") {
		t.Fatalf("expected updated autoload path, got:\n%s", out)
	}
}

func TestLink_UsesStoredPathWhenNoPathProvided(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows (symlink/junction behavior varies by environment)")
	}

	projectDir := t.TempDir()

	pluginDir := filepath.Join(projectDir, "local_plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.cfg"), []byte("[plugin]\nname=\"Test\"\n"), 0o644); err != nil {
		t.Fatalf("write plugin.cfg: %v", err)
	}

	m := manifest.New()
	m = manifest.UpsertAddon(m, "@user/addon", manifest.Addon{
		Link: &manifest.Link{
			Enabled: false,
			Path:    pluginDir,
		},
	})
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

	if err := Link(context.Background(), LinkOptions{
		Spec: "@user/addon",
	}); err != nil {
		t.Fatalf("link: %v", err)
	}

	addonDirName, err := addonDirNameForPluginKey("@user/addon")
	if err != nil {
		t.Fatalf("addonDirNameForPluginKey: %v", err)
	}
	dst := filepath.Join(projectDir, "addons", addonDirName)
	if info, err := os.Lstat(dst); err != nil {
		t.Fatalf("lstat dst: %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected dst to be symlink, got mode %v", info.Mode())
	}
	if resolved, err := filepath.EvalSymlinks(dst); err != nil {
		t.Fatalf("EvalSymlinks dst: %v", err)
	} else {
		expected := pluginDir
		if expectedResolved, err := filepath.EvalSymlinks(pluginDir); err == nil {
			expected = expectedResolved
		}
		if filepath.Clean(resolved) != filepath.Clean(expected) {
			t.Fatalf("expected dst to resolve to %q, got %q", expected, resolved)
		}
	}

	gdamPath := filepath.Join(projectDir, "gdam.json")
	m2, err := manifest.Load(gdamPath)
	if err != nil {
		t.Fatalf("read gdam.json: %v", err)
	}
	if _, ok := m2.Addons["@user/addon"]; !ok {
		t.Fatalf("expected addon entry to exist in gdam.json")
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
	link, ok := lm.Addons["@user/addon"]
	if !ok {
		t.Fatalf("expected gdam.link.json entry for @user/addon")
	}
	if got := link.Path; got != pluginDir {
		t.Fatalf("expected gdam.link.json path %q, got %q", pluginDir, got)
	}
	if got := link.Enabled; got != true {
		t.Fatalf("expected gdam.link.json enabled=true, got %v", got)
	}
}

func TestLink_RequiresPathWhenNoStoredPath(t *testing.T) {
	projectDir := t.TempDir()

	m := manifest.New()
	m = manifest.UpsertAddon(m, "@user/addon", manifest.Addon{
		Link: &manifest.Link{
			Enabled: false,
		},
	})
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

	if err := Link(context.Background(), LinkOptions{
		Spec: "@user/addon",
	}); err == nil {
		t.Fatalf("expected error")
	}
}
