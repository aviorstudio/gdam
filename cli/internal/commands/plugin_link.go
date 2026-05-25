package commands

import (
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/manifest"
)

func pluginLinkEnabled(addon manifest.Addon) bool {
	if addon.Link == nil || !addon.Link.Enabled {
		return false
	}
	return strings.TrimSpace(addon.Link.Path) != ""
}

func pluginLinkPath(addon manifest.Addon) string {
	if addon.Link == nil {
		return ""
	}
	return strings.TrimSpace(addon.Link.Path)
}
