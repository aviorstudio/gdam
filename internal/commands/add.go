package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aviorstudio/gdam/internal/manifest"
	"github.com/aviorstudio/gdam/internal/project"
	"github.com/aviorstudio/gdam/internal/spec"
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

	resolved, err := resolveAddonFromRegistry(ctx, pkg.Owner, pkg.Repo, pkg.Tag)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}

	if isLinked {
		existing.Tag = resolved.TagName
		m = manifest.UpsertAddon(m, pkg.Name(), existing)
		if err := manifest.Save(manifestPath, m); err != nil {
			return err
		}
		fmt.Printf("updated %s@%s (linked)\n", pkg.Name(), resolved.TagName)
		return nil
	}

	addonDirName, err := addonDirNameForPluginKey(pkg.Name())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	if err := validateNoAddonDirCollision(m, pkg.Name(), addonDirName); err != nil {
		return err
	}

	var link *manifest.Link
	if hasExisting {
		link = existing.Link
	}
	m = manifest.UpsertAddon(m, pkg.Name(), manifest.Addon{
		Tag:  resolved.TagName,
		Link: link,
	})
	if err := manifest.Save(manifestPath, m); err != nil {
		return err
	}

	// The pin is recorded; the install resolves the whole set, rewrites the
	// lock and places every copy, this addon's dependencies included.
	return installProject(ctx, projectDir, InstallOptions{})
}
