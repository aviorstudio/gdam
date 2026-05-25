package fsutil

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractZipAllowRootFilesReturnsDestinationForRootFiles(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "addon.zip")
	writeTestZip(t, zipPath, map[string]string{
		"plugin.cfg": "[plugin]\n",
		"plugin.gd":  "extends EditorPlugin\n",
		"src/mod.gd": "extends RefCounted\n",
	})

	extractDir := filepath.Join(tmpDir, "extract")
	rootDir, err := ExtractZipAllowRootFiles(zipPath, extractDir)
	if err != nil {
		t.Fatal(err)
	}
	if rootDir != extractDir {
		t.Fatalf("expected extract dir root, got %s", rootDir)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "plugin.cfg")); err != nil {
		t.Fatalf("expected plugin.cfg at root: %v", err)
	}
}

func TestExtractZipStillRequiresSingleRootDir(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "addon.zip")
	writeTestZip(t, zipPath, map[string]string{
		"plugin.cfg": "[plugin]\n",
		"plugin.gd":  "extends EditorPlugin\n",
	})

	if _, err := ExtractZip(zipPath, filepath.Join(tmpDir, "extract")); err == nil {
		t.Fatal("expected ExtractZip to reject root-file layout")
	}
}

func writeTestZip(t *testing.T, zipPath string, files map[string]string) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
}
