package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func pluginCfgExistsAtDirRoot(dir string) (bool, error) {
	pluginCfgPath := filepath.Join(dir, "plugin.cfg")
	info, err := os.Stat(pluginCfgPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("plugin.cfg is a directory: %s", pluginCfgPath)
	}
	return true, nil
}
