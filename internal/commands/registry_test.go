package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aviorstudio/gdam/internal/gdamdb"
	"github.com/aviorstudio/gdam/internal/githubapi"
)

func withResolvedEditorPlugin(t *testing.T) {
	t.Helper()
	previous := resolveAddonFromRegistry
	resolveAddonFromRegistry = func(ctx context.Context, owner, addon, requestedTag string) (gdamdb.ResolvedAddon, error) {
		return gdamdb.ResolvedAddon{
			Name:         "@" + owner + "/" + addon,
			TagName:      requestedTag,
			EditorPlugin: true,
		}, nil
	}
	t.Cleanup(func() {
		resolveAddonFromRegistry = previous
	})
}

func withFakeRegistryInstall(t *testing.T) {
	t.Helper()
	previousResolve := resolveAddonFromRegistry
	previousPrepare := preparePackageRoot
	resolveAddonFromRegistry = func(ctx context.Context, owner, addon, requestedTag string) (gdamdb.ResolvedAddon, error) {
		tag := requestedTag
		if tag == "" {
			tag = "1.2.3"
		}
		return gdamdb.ResolvedAddon{
			Name:         "@" + owner + "/" + addon,
			GitHubOwner:  owner,
			GitHubRepo:   addon,
			TagName:      tag,
			AssetName:    "@" + owner + "_" + addon + ".zip",
			EditorPlugin: true,
		}, nil
	}
	preparePackageRoot = func(ctx context.Context, gh *githubapi.Client, resolved gdamdb.ResolvedAddon, tmpDir string) (string, error) {
		root := filepath.Join(tmpDir, "pkg")
		if err := os.MkdirAll(root, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(root, "plugin.cfg"), []byte("[plugin]\nname=\"Test\"\n"), 0o644); err != nil {
			return "", err
		}
		return root, nil
	}
	t.Cleanup(func() {
		resolveAddonFromRegistry = previousResolve
		preparePackageRoot = previousPrepare
	})
}
