package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func repoSubdirRoot(repoRootDir, repoSubdir string) (string, error) {
	repoSubdir = strings.Trim(strings.TrimSpace(repoSubdir), "/")
	if repoSubdir == "" {
		return repoRootDir, nil
	}

	rel := filepath.FromSlash(repoSubdir)
	rel = filepath.Clean(rel)
	if rel == "." || rel == string(filepath.Separator) || rel == "" {
		return repoRootDir, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid repo subdir: %q", repoSubdir)
	}

	abs := filepath.Join(repoRootDir, rel)
	repoRootClean := filepath.Clean(repoRootDir)
	absClean := filepath.Clean(abs)
	if absClean != repoRootClean && !strings.HasPrefix(absClean, repoRootClean+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid repo subdir: %q", repoSubdir)
	}

	info, err := os.Stat(absClean)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("repo subdir not found: %s", repoSubdir)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo subdir is not a directory: %s", repoSubdir)
	}

	return absClean, nil
}
