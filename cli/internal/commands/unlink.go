package commands

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
	"github.com/aviorstudio/gdam/cli/internal/gdamdb"
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

	plugin, ok := m.Addons[pluginKey]
	if !ok {
		return fmt.Errorf("%w: addon not found in gdam.json: %s", ErrUserInput, pluginKey)
	}
	if !pluginLinkEnabled(plugin) {
		return fmt.Errorf("%w: addon is not linked: %s", ErrUserInput, pluginKey)
	}

	addonDirName, err := addonDirNameForPluginKey(pluginKey)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	dst := filepath.Join(projectDir, "addons", addonDirName)

	linkedAbs, err := pluginAbsPath(projectDir, pluginLinkPath(plugin))
	if err != nil {
		return err
	}

	if plugin.Link != nil {
		plugin.Link.Enabled = false
	}

	if strings.TrimSpace(plugin.Repo) == "" {
		projectGodotPath := filepath.Join(projectDir, "project.godot")
		if _, err := os.Stat(projectGodotPath); err == nil {
			pluginCfgResPath := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
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

		if err := fsutil.RemoveAll(dst); err != nil {
			return err
		}

		m = manifest.UpsertPlugin(m, pluginKey, plugin)
		if err := manifest.Save(manifestPath, m); err != nil {
			return err
		}
		fmt.Printf("unlinked %s\n", pluginKey)
		return nil
	}

	ghOwner, ghRepo, ref, repoSubdir, err := gdamdb.ParseGitHubTreeURLWithPath(plugin.Repo)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "gdam-unlink-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "repo.zip")
	gh := githubapi.NewClient(os.Getenv("GITHUB_TOKEN"))
	if err := gh.DownloadZipball(ctx, ghOwner, ghRepo, ref, zipPath); err != nil {
		return err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	rootDir, err := fsutil.ExtractZip(zipPath, extractDir)
	if err != nil {
		return err
	}

	pkgRootDir, err := repoSubdirRoot(rootDir, repoSubdir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}

	if ok, err := pluginCfgExistsAtDirRoot(pkgRootDir); err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	} else if !ok {
		expected := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
		if strings.TrimSpace(repoSubdir) != "" {
			return fmt.Errorf("%w: package is missing plugin.cfg at %s in repository (expected to install it to %s)", ErrUserInput, repoSubdir, expected)
		}
		return fmt.Errorf("%w: package is missing plugin.cfg at repository root (expected to install it to %s)", ErrUserInput, expected)
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

	m = manifest.UpsertPlugin(m, pluginKey, plugin)
	if err := manifest.Save(manifestPath, m); err != nil {
		return err
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	if _, err := os.Stat(projectGodotPath); err == nil {
		pluginCfgResPath := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
		if linkedAbs != "" {
			if err := disableEditorPluginAliases(projectGodotPath, projectDir, m, pluginKey, addonDirName, linkedAbs); err != nil {
				return err
			}
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

	fmt.Printf("unlinked %s\n", pluginKey)
	return nil
}
