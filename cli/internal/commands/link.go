package commands

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
	"github.com/aviorstudio/gdam/cli/internal/manifest"
	"github.com/aviorstudio/gdam/cli/internal/project"
	"github.com/aviorstudio/gdam/cli/internal/spec"
)

type LinkOptions struct {
	Spec string
	Path string
}

func Link(ctx context.Context, opts LinkOptions) error {
	_ = ctx

	specInput := strings.TrimSpace(opts.Spec)
	if specInput == "" {
		return fmt.Errorf("%w: missing addon spec", ErrUserInput)
	}
	if !strings.HasPrefix(specInput, "@") {
		specInput = "@" + specInput
	}
	pkg, err := spec.ParsePackageSpec(specInput)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	if pkg.Version != "" {
		return fmt.Errorf("%w: link does not take a version (use @username/addon)", ErrUserInput)
	}
	pluginKey := pkg.Name()

	startDir, err := os.Getwd()
	if err != nil {
		return err
	}

	projectDir, ok := project.FindManifestDir(startDir)
	if !ok {
		return fmt.Errorf("%w: no gdam.json found (run `gdam init`)", ErrUserInput)
	}

	manifestPath := filepath.Join(projectDir, "gdam.json")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}

	addon, pluginExists := m.Addons[pluginKey]

	pathInput := strings.TrimSpace(opts.Path)
	usingStoredPath := false
	if pathInput == "" {
		if !pluginExists || addon.Link == nil || strings.TrimSpace(addon.Link.Path) == "" {
			return fmt.Errorf("%w: missing local path (run `gdam link %s <local_path>`)", ErrUserInput, pluginKey)
		}
		pathInput = addon.Link.Path
		usingStoredPath = true
	}

	var abs string
	if usingStoredPath {
		abs, err = pluginAbsPath(projectDir, pathInput)
		if err != nil {
			return err
		}
	} else {
		expanded, err := fsutil.ExpandHome(pathInput)
		if err != nil {
			return err
		}
		abs, err = filepath.Abs(expanded)
		if err != nil {
			return err
		}
	}

	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: local path is not a directory: %s", ErrUserInput, abs)
	}

	if ok, err := pluginCfgExistsAtDirRoot(abs); err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	} else if !ok {
		return fmt.Errorf("%w: plugin.cfg not found at %s (pass the addon directory that contains plugin.cfg)", ErrUserInput, filepath.Join(abs, "plugin.cfg"))
	}

	addonDirName, err := addonDirNameForPluginKey(pluginKey)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	if err := validateNoAddonDirCollision(m, pluginKey, addonDirName); err != nil {
		return err
	}

	localAddonsDir := filepath.Join(projectDir, "addons")
	if err := os.MkdirAll(localAddonsDir, 0o755); err != nil {
		return err
	}

	dst := filepath.Join(localAddonsDir, addonDirName)
	if err := fsutil.RemoveAll(dst); err != nil {
		return err
	}

	if err := fsutil.SymlinkDir(abs, dst); err != nil {
		return err
	}

	if ok, err := pluginCfgExistsAtDirRoot(dst); err != nil {
		_ = fsutil.RemoveAll(dst)
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	} else if !ok {
		_ = fsutil.RemoveAll(dst)
		return fmt.Errorf("%w: linked addon is missing plugin.cfg at %s", ErrUserInput, filepath.Join(dst, "plugin.cfg"))
	}

	storedPath := pathInput
	if !usingStoredPath {
		storedPath, err = fsutil.AbbrevHome(abs)
		if err != nil {
			return err
		}
	}

	if addon.Link == nil {
		addon.Link = &manifest.Link{}
	}
	addon.Link.Enabled = true
	if !usingStoredPath {
		addon.Link.Path = storedPath
	}
	m = manifest.UpsertAddon(m, pluginKey, addon)
	if err := manifest.Save(manifestPath, m); err != nil {
		return err
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	if addon.EditorPlugin {
		if _, err := os.Stat(projectGodotPath); err == nil {
			pluginCfgResPath := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
			if err := disableEditorPluginAliases(projectGodotPath, projectDir, m, pluginKey, addonDirName, abs); err != nil {
				return err
			}
			updated, err := project.SetEditorPluginEnabled(projectGodotPath, pluginCfgResPath, true)
			if err != nil {
				return err
			}
			if updated {
				fmt.Printf("enabled %s\n", pluginCfgResPath)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	fmt.Printf("linked %s -> %s\n", pluginKey, storedPath)
	return nil
}

func disableEditorPluginAliases(projectGodotPath, projectDir string, m manifest.Manifest, pluginKey, addonDirName, abs string) error {
	absResolved := filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(absResolved); err == nil {
		absResolved = filepath.Clean(resolved)
	}

	if legacyKey := legacyPluginKeyFromLocalPath(absResolved); legacyKey != "" && legacyKey != pluginKey {
		legacyAddonDirName, err := addonDirNameForPluginKey(legacyKey)
		if err == nil && legacyAddonDirName != "" && legacyAddonDirName != addonDirName {
			legacyPluginCfgResPath := "res://" + path.Join("addons", legacyAddonDirName, "plugin.cfg")
			if _, err := project.SetEditorPluginEnabled(projectGodotPath, legacyPluginCfgResPath, false); err != nil {
				return err
			}
			if _, err := project.ReplaceAutoloadAddonDir(projectGodotPath, legacyAddonDirName, addonDirName); err != nil {
				return err
			}
		}
	}

	for otherKey, otherPlugin := range m.Addons {
		if otherKey == pluginKey {
			continue
		}
		if !pluginLinkEnabled(otherPlugin) {
			continue
		}

		otherAbs, err := pluginAbsPath(projectDir, pluginLinkPath(otherPlugin))
		if err != nil {
			continue
		}
		otherResolved := filepath.Clean(otherAbs)
		if resolved, err := filepath.EvalSymlinks(otherResolved); err == nil {
			otherResolved = filepath.Clean(resolved)
		}
		if otherResolved != absResolved {
			continue
		}

		otherAddonDirName, err := addonDirNameForPluginKey(otherKey)
		if err != nil {
			continue
		}
		if otherAddonDirName == addonDirName {
			continue
		}

		otherPluginCfgResPath := "res://" + path.Join("addons", otherAddonDirName, "plugin.cfg")
		if _, err := project.SetEditorPluginEnabled(projectGodotPath, otherPluginCfgResPath, false); err != nil {
			return err
		}
		if _, err := project.ReplaceAutoloadAddonDir(projectGodotPath, otherAddonDirName, addonDirName); err != nil {
			return err
		}
	}

	addonsDir := filepath.Join(projectDir, "addons")
	entries, err := os.ReadDir(addonsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == addonDirName {
			continue
		}
		entryPath := filepath.Join(addonsDir, name)

		resolved, err := filepath.EvalSymlinks(entryPath)
		if err != nil {
			continue
		}
		if filepath.Clean(resolved) != absResolved {
			continue
		}

		legacyPluginCfgResPath := "res://" + path.Join("addons", name, "plugin.cfg")
		if _, err := project.SetEditorPluginEnabled(projectGodotPath, legacyPluginCfgResPath, false); err != nil {
			return err
		}
		if _, err := project.ReplaceAutoloadAddonDir(projectGodotPath, name, addonDirName); err != nil {
			return err
		}
	}

	return nil
}

func legacyPluginKeyFromLocalPath(absDir string) string {
	baseName := strings.TrimSpace(filepath.Base(absDir))
	baseName = strings.TrimPrefix(baseName, "@")
	baseName = strings.ReplaceAll(baseName, " ", "_")
	if baseName == "" {
		return ""
	}
	return "@" + baseName
}

func pluginAbsPath(projectDir, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}

	expanded, err := fsutil.ExpandHome(p)
	if err != nil {
		return "", err
	}
	if expanded == "" {
		return "", nil
	}
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(projectDir, expanded)
	}
	return filepath.Abs(expanded)
}
