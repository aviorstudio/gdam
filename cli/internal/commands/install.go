package commands

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
	"github.com/aviorstudio/gdam/cli/internal/githubapi"
	"github.com/aviorstudio/gdam/cli/internal/manifest"
	"github.com/aviorstudio/gdam/cli/internal/project"
)

type InstallOptions struct{}

type installCandidate struct {
	pluginKey    string
	addonDir     string
	dst          string
	version      string
	editorPlugin bool
	ghOwner      string
	ghRepo       string
	ref          string
	assetName    string
	prepRootDir  string
}

func Install(ctx context.Context, opts InstallOptions) error {
	_ = opts

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

	pluginKeys := make([]string, 0, len(m.Addons))
	for key := range m.Addons {
		pluginKeys = append(pluginKeys, key)
	}
	sort.Strings(pluginKeys)

	addonsDir := filepath.Join(projectDir, "addons")
	candidates := make([]installCandidate, 0, len(pluginKeys))

	for _, pluginKey := range pluginKeys {
		addonDirName, err := addonDirNameForPluginKey(pluginKey)
		if err != nil {
			return fmt.Errorf("%w: invalid addon key in gdam.json: %s (%v)", ErrUserInput, pluginKey, err)
		}
		if err := validateNoAddonDirCollision(m, pluginKey, addonDirName); err != nil {
			return err
		}

		addon := m.Addons[pluginKey]
		if pluginLinkEnabled(addon) {
			continue
		}

		dst := filepath.Join(addonsDir, addonDirName)
		if info, err := os.Lstat(dst); err == nil {
			if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			return fmt.Errorf("%w: addon path exists and is not a directory: %s", ErrUserInput, dst)
		} else if !os.IsNotExist(err) {
			return err
		}

		resolved, err := resolveManifestAddon(ctx, pluginKey, addon.Version)
		if err != nil {
			return fmt.Errorf("%w: unable to resolve %s: %v", ErrUserInput, pluginKey, err)
		}

		candidates = append(candidates, installCandidate{
			pluginKey:    pluginKey,
			addonDir:     addonDirName,
			dst:          dst,
			version:      resolved.Version,
			editorPlugin: resolved.EditorPlugin,
			ghOwner:      resolved.GitHubOwner,
			ghRepo:       resolved.GitHubRepo,
			ref:          resolved.ReleaseTag,
			assetName:    resolved.AssetName,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return err
	}

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	hasProjectGodot := false
	if _, err := os.Stat(projectGodotPath); err == nil {
		hasProjectGodot = true
	} else if !os.IsNotExist(err) {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "gdam-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	gh := githubapi.NewClient(os.Getenv("GITHUB_TOKEN"))

	for i := range candidates {
		if _, err := os.Lstat(candidates[i].dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		pkgTmpDir := filepath.Join(tmpDir, fmt.Sprintf("pkg-%d", i))
		if err := os.MkdirAll(pkgTmpDir, 0o755); err != nil {
			return err
		}

		pkgRootDir, err := prepareGitHubPackageRoot(ctx, gh, candidates[i].ghOwner, candidates[i].ghRepo, candidates[i].ref, candidates[i].assetName, pkgTmpDir)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		}

		if ok, err := pluginCfgExistsAtDirRoot(pkgRootDir); err != nil {
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		} else if !ok {
			expected := "res://" + path.Join("addons", candidates[i].addonDir, "plugin.cfg")
			return fmt.Errorf("%w: package is missing plugin.cfg in release asset %s (expected to install it to %s)", ErrUserInput, candidates[i].assetName, expected)
		}

		if _, err := os.Lstat(candidates[i].dst); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := fsutil.CopyPath(pkgRootDir, candidates[i].dst); err != nil {
			return err
		}

		if ok, err := pluginCfgExistsAtDirRoot(candidates[i].dst); err != nil {
			_ = fsutil.RemoveAll(candidates[i].dst)
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		} else if !ok {
			_ = fsutil.RemoveAll(candidates[i].dst)
			return fmt.Errorf("%w: installed addon is missing plugin.cfg at %s", ErrUserInput, filepath.Join(candidates[i].dst, "plugin.cfg"))
		}

		if hasProjectGodot && candidates[i].editorPlugin {
			pluginCfgResPath := "res://" + path.Join("addons", candidates[i].addonDir, "plugin.cfg")
			updated, err := project.SetEditorPluginEnabled(projectGodotPath, pluginCfgResPath, true)
			if err != nil {
				return err
			}
			if updated {
				fmt.Printf("enabled %s\n", pluginCfgResPath)
			}
		}

		if candidates[i].version != "" {
			fmt.Printf("installed %s@%s\n", candidates[i].pluginKey, candidates[i].version)
		} else {
			fmt.Printf("installed %s\n", candidates[i].pluginKey)
		}
	}

	return nil
}
