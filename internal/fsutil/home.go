package fsutil

import (
	"os"
	"path/filepath"
	"strings"
)

func ExpandHome(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rest := strings.TrimPrefix(path, "~")
		rest = strings.TrimPrefix(rest, "/")
		rest = strings.TrimPrefix(rest, "\\")
		rest = filepath.FromSlash(rest)
		if rest == "" {
			return homeDir, nil
		}
		return filepath.Join(homeDir, rest), nil
	}
	return path, nil
}

func AbbrevHome(absPath string) (string, error) {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return "", nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return absPath, nil
	}
	homeDir = filepath.Clean(homeDir)
	absPath = filepath.Clean(absPath)

	rel, err := filepath.Rel(homeDir, absPath)
	if err != nil {
		return absPath, nil
	}
	if rel == "." {
		return "~", nil
	}
	sep := string(os.PathSeparator)
	if rel == ".." || strings.HasPrefix(rel, ".."+sep) {
		return absPath, nil
	}
	return "~" + sep + rel, nil
}
