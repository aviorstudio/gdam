package fsutil

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func SymlinkDir(target, link string) error {
	if err := os.Symlink(target, link); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create junction: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
