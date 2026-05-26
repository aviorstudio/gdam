package commands

import (
	"context"
	"testing"

	"github.com/aviorstudio/gdam/cli/internal/gdamdb"
)

func withResolvedEditorPlugin(t *testing.T) {
	t.Helper()
	previous := resolveAddonFromRegistry
	resolveAddonFromRegistry = func(ctx context.Context, owner, addon, requestedVersion string) (gdamdb.ResolvedAddon, error) {
		return gdamdb.ResolvedAddon{
			Name:         "@" + owner + "/" + addon,
			Version:      requestedVersion,
			EditorPlugin: true,
		}, nil
	}
	t.Cleanup(func() {
		resolveAddonFromRegistry = previous
	})
}
