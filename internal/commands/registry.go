package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/aviorstudio/gdam/internal/gdamdb"
	"github.com/aviorstudio/gdam/internal/spec"
)

var resolveAddonFromRegistry = func(ctx context.Context, owner, addon, requestedTag string) (gdamdb.ResolvedAddon, error) {
	return gdamdb.NewDefaultClient().ResolveAddon(ctx, owner, addon, requestedTag)
}

var preparePackageRoot = prepareGitHubPackageRoot

func resolveManifestAddon(ctx context.Context, addonKey, requestedTag string) (gdamdb.ResolvedAddon, error) {
	pkg, err := spec.ParsePackageSpec(addonKey)
	if err != nil {
		return gdamdb.ResolvedAddon{}, fmt.Errorf("invalid addon key %s: %v", addonKey, err)
	}
	if strings.TrimSpace(pkg.Tag) != "" {
		return gdamdb.ResolvedAddon{}, fmt.Errorf("invalid addon key %s: tags belong in the tag field", addonKey)
	}
	return resolveAddonFromRegistry(ctx, pkg.Owner, pkg.Repo, strings.TrimSpace(requestedTag))
}

func manifestAddonEditorPlugin(ctx context.Context, addonKey, requestedTag string) bool {
	resolved, err := resolveManifestAddon(ctx, addonKey, requestedTag)
	if err != nil {
		return false
	}
	return resolved.EditorPlugin
}
