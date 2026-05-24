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

type RemoveOptions struct {
	Spec string
}

func Remove(ctx context.Context, opts RemoveOptions) error {
	_ = ctx

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
		return fmt.Errorf("%w: remove does not take a version (use @username/addon)", ErrUserInput)
	}

	if !manifest.HasPlugin(m, pkg.Name()) {
		return fmt.Errorf("%w: addon not found in gdam.json: %s", ErrUserInput, pkg.Name())
	}

	addonDirName := strings.ReplaceAll(pkg.Name(), "/", "_")
	if err := validateAddonDirName(addonDirName); err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}

	dst := filepath.Join(projectDir, "addons", addonDirName)

	projectGodotPath := filepath.Join(projectDir, "project.godot")
	if _, err := os.Stat(projectGodotPath); err == nil {
		pluginCfgResPath := "res://" + path.Join("addons", addonDirName, "plugin.cfg")
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

	m = manifest.RemovePlugin(m, pkg.Name())
	if err := manifest.Save(manifestPath, m); err != nil {
		return err
	}

	fmt.Printf("removed %s\n", pkg.Name())
	return nil
}
