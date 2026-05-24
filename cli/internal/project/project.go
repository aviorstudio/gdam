package project

import (
	"os"
	"path/filepath"
)

func FindManifestDir(startDir string) (string, bool) {
	return findUp(startDir, func(dir string) bool {
		_, err := os.Stat(filepath.Join(dir, "gdam.json"))
		return err == nil
	})
}

func FindGodotProjectDir(startDir string) (string, bool) {
	return findUp(startDir, func(dir string) bool {
		_, err := os.Stat(filepath.Join(dir, "project.godot"))
		return err == nil
	})
}

func findUp(startDir string, predicate func(dir string) bool) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false
	}

	for {
		if predicate(dir) {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
