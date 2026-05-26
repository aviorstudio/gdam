package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/gdamdb"
	"github.com/aviorstudio/gdam/cli/internal/spec"
)

var resolveAddonFromRegistry = func(ctx context.Context, owner, addon, requestedVersion string) (gdamdb.ResolvedAddon, error) {
	return gdamdb.NewDefaultClient().ResolveAddon(ctx, owner, addon, requestedVersion)
}

var preparePackageRoot = prepareGitHubPackageRoot

func resolveManifestAddon(ctx context.Context, addonKey, requestedVersion string) (gdamdb.ResolvedAddon, error) {
	pkg, err := spec.ParsePackageSpec(addonKey)
	if err != nil {
		return gdamdb.ResolvedAddon{}, fmt.Errorf("invalid addon key %s: %v", addonKey, err)
	}
	if strings.TrimSpace(pkg.Version) != "" {
		return gdamdb.ResolvedAddon{}, fmt.Errorf("invalid addon key %s: versions belong in the version field", addonKey)
	}
	return resolveAddonFromRegistry(ctx, pkg.Owner, pkg.Repo, strings.TrimSpace(requestedVersion))
}

func manifestAddonEditorPlugin(ctx context.Context, addonKey, requestedVersion string) bool {
	resolved, err := resolveManifestAddon(ctx, addonKey, requestedVersion)
	if err != nil {
		return false
	}
	return resolved.EditorPlugin
}
