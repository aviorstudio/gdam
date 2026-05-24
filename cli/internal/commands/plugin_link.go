package commands

import (
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/manifest"
)

func pluginLinkEnabled(plugin manifest.Plugin) bool {
	if plugin.Link == nil || !plugin.Link.Enabled {
		return false
	}
	return strings.TrimSpace(plugin.Link.Path) != ""
}

func pluginLinkPath(plugin manifest.Plugin) string {
	if plugin.Link == nil {
		return ""
	}
	return strings.TrimSpace(plugin.Link.Path)
}
