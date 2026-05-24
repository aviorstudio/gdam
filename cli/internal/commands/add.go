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

type AddOptions struct {
	Spec string
}

func Add(ctx context.Context, opts AddOptions) error {
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
		if godotDir, ok := project.FindGodotProjectDir(startDir); ok {
			return fmt.Errorf("%w: no gdam.json found (run `gdam init` in %s)", ErrUserInput, godotDir)
		}
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

	existing, hasExisting := m.Addons[pkg.Name()]
	isLinked := hasExisting && pluginLinkEnabled(existing)

	db := gdamdb.NewDefaultClient()

	resolved, err := db.ResolvePlugin(ctx, pkg.Owner, pkg.Repo, pkg.Version)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}

	if isLinked {
		existing.Repo = gdamdb.GitHubTreeURLWithPath(resolved.GitHubOwner, resolved.GitHubRepo, resolved.SHA, resolved.GitHubSubdir)
		existing.Version = resolved.Version
		existing.EditorPlugin = resolved.EditorPlugin
		m = manifest.UpsertPlugin(m, pkg.Name(), existing)
		if err := manifest.Save(manifestPath, m); err != nil {
			return err
		}
		fmt.Printf("updated %s@%s (linked)\n", pkg.Name(), resolved.Version)
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "gdam-add-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "repo.zip")
	gh := githubapi.NewClient(os.Getenv("GITHUB_TOKEN"))
	if err := gh.DownloadZipball(ctx, resolved.GitHubOwner, resolved.GitHubRepo, resolved.SHA, zipPath); err != nil {
		return err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	rootDir, err := fsutil.ExtractZip(zipPath, extractDir)
	if err != nil {
		return err
	}

	pkgRootDir, err := repoSubdirRoot(rootDir, resolved.GitHubSubdir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}

	localAddonsDir := filepath.Join(projectDir, "addons")
	if err := os.MkdirAll(localAddonsDir, 0o755); err != nil {
		return err
	}

	addonDirName, err := addonDirNameForPluginKey(pkg.Name())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}

	if err := validateNoAddonDirCollision(m, pkg.Name(), addonDirName); err != nil {
		return err
	}

	if ok, err := pluginCfgExistsAtDirRoot(pkgRootDir); err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	} else if !ok {
		expected := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
		if strings.TrimSpace(resolved.GitHubSubdir) != "" {
			return fmt.Errorf("%w: package is missing plugin.cfg at %s in repository (expected to install it to %s)", ErrUserInput, resolved.GitHubSubdir, expected)
		}
		return fmt.Errorf("%w: package is missing plugin.cfg at repository root (expected to install it to %s)", ErrUserInput, expected)
	}

	dst := filepath.Join(localAddonsDir, addonDirName)
	if manifest.HasPlugin(m, pkg.Name()) {
		if err := fsutil.RemoveAll(dst); err != nil {
			return err
		}
	} else {
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("%w: destination already exists: %s", ErrUserInput, dst)
		} else if !os.IsNotExist(err) {
			return err
		}
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

	var link *manifest.Link
	if hasExisting {
		link = existing.Link
	}
	m = manifest.UpsertPlugin(m, pkg.Name(), manifest.Plugin{
		Repo:         gdamdb.GitHubTreeURLWithPath(resolved.GitHubOwner, resolved.GitHubRepo, resolved.SHA, resolved.GitHubSubdir),
		Version:      resolved.Version,
		EditorPlugin: resolved.EditorPlugin,
		Link:         link,
	})
	if err := manifest.Save(manifestPath, m); err != nil {
		return err
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	if resolved.EditorPlugin {
		if _, err := os.Stat(projectGodotPath); err == nil {
			pluginCfgResPath := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
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

	fmt.Printf("installed %s@%s (%s)\n", pkg.Name(), resolved.Version, resolved.SHA)
	return nil
}
