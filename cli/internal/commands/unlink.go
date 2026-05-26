package commands

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
	"github.com/aviorstudio/gdam/cli/internal/githubapi"
	"github.com/aviorstudio/gdam/cli/internal/manifest"
	"github.com/aviorstudio/gdam/cli/internal/project"
	"github.com/aviorstudio/gdam/cli/internal/spec"
)

type UnlinkOptions struct {
	Spec string
}

func Unlink(ctx context.Context, opts UnlinkOptions) error {
	specInput := strings.TrimSpace(opts.Spec)
	if specInput == "" {
		return fmt.Errorf("%w: missing addon spec", ErrUserInput)
	}
	if !strings.HasPrefix(specInput, "@") {
		specInput = "@" + specInput
	}

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

	pkg, err := spec.ParsePackageSpec(specInput)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	if pkg.Version != "" {
		return fmt.Errorf("%w: unlink does not take a version (use @username/addon)", ErrUserInput)
	}
	pluginKey := pkg.Name()

	addon, ok := m.Addons[pluginKey]
	if !ok {
		return fmt.Errorf("%w: addon not found in gdam.json: %s", ErrUserInput, pluginKey)
	}
	if !pluginLinkEnabled(addon) {
		return fmt.Errorf("%w: addon is not linked: %s", ErrUserInput, pluginKey)
	}

	addonDirName, err := addonDirNameForPluginKey(pluginKey)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	dst := filepath.Join(projectDir, "addons", addonDirName)

	linkedAbs, err := pluginAbsPath(projectDir, pluginLinkPath(addon))
	if err != nil {
		return err
	}

	if addon.Link != nil {
		addon.Link.Enabled = false
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	hasProjectGodot := false
	pluginCfgResPath := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
	if _, err := os.Stat(projectGodotPath); err == nil {
		hasProjectGodot = true
		if linkedAbs != "" {
			if err := disableEditorPluginAliases(projectGodotPath, projectDir, m, pluginKey, addonDirName, linkedAbs); err != nil {
				return err
			}
		}
		updated, err := project.SetEditorPluginEnabled(projectGodotPath, pluginCfgResPath, false)
		if err != nil {
			return err
		}
		if updated {
			fmt.Printf("disabled %s\n", pluginCfgResPath)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if strings.TrimSpace(addon.Version) == "" {
		if err := fsutil.RemoveAll(dst); err != nil {
			return err
		}

		m = manifest.UpsertAddon(m, pluginKey, addon)
		if err := manifest.Save(manifestPath, m); err != nil {
			return err
		}
		fmt.Printf("unlinked %s\n", pluginKey)
		return nil
	}

	resolved, err := resolveManifestAddon(ctx, pluginKey, addon.Version)
	if err != nil {
		return fmt.Errorf("%w: unable to resolve %s: %v", ErrUserInput, pluginKey, err)
	}

	tmpDir, err := os.MkdirTemp("", "gdam-unlink-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	gh := githubapi.NewClient(os.Getenv("GITHUB_TOKEN"))
	pkgRootDir, err := prepareGitHubPackageRoot(ctx, gh, resolved.GitHubOwner, resolved.GitHubRepo, resolved.ReleaseTag, resolved.AssetName, tmpDir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}

	if ok, err := pluginCfgExistsAtDirRoot(pkgRootDir); err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	} else if !ok {
		expected := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
		return fmt.Errorf("%w: package is missing plugin.cfg in release asset %s (expected to install it to %s)", ErrUserInput, resolved.AssetName, expected)
	}

	localAddonsDir := filepath.Join(projectDir, "addons")
	if err := os.MkdirAll(localAddonsDir, 0o755); err != nil {
		return err
	}

	if err := fsutil.RemoveAll(dst); err != nil {
		return err
	}

	if err := fsutil.CopyPath(pkgRootDir, dst); err != nil {
		return err
	}

	if ok, err := pluginCfgExistsAtDirRoot(dst); err != nil {
		_ = fsutil.RemoveAll(dst)
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	} else if !ok {
		_ = fsutil.RemoveAll(dst)
		return fmt.Errorf("%w: installed addon is missing plugin.cfg at %s", ErrUserInput, filepath.Join(dst, "plugin.cfg"))
	}

	m = manifest.UpsertAddon(m, pluginKey, addon)
	if err := manifest.Save(manifestPath, m); err != nil {
		return err
	}

	if hasProjectGodot && resolved.EditorPlugin {
		updated, err := project.SetEditorPluginEnabled(projectGodotPath, pluginCfgResPath, true)
		if err != nil {
			return err
		}
		if updated {
			fmt.Printf("enabled %s\n", pluginCfgResPath)
		}
	}

	fmt.Printf("unlinked %s\n", pluginKey)
	return nil
}
