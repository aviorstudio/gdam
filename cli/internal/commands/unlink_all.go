package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/aviorstudio/gdam/cli/internal/manifest"
	"github.com/aviorstudio/gdam/cli/internal/project"
)

type UnlinkAllOptions struct{}

func UnlinkAll(ctx context.Context, opts UnlinkAllOptions) error {
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
	for key, addon := range m.Addons {
		if !pluginLinkEnabled(addon) {
			continue
		}
		pluginKeys = append(pluginKeys, key)
	}
	sort.Strings(pluginKeys)

	for _, pluginKey := range pluginKeys {
		if err := Unlink(ctx, UnlinkOptions{Spec: pluginKey}); err != nil {
			return err
		}
	}

	return nil
}
