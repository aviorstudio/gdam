package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aviorstudio/gdam/cli/internal/manifest"
	"github.com/aviorstudio/gdam/cli/internal/project"
)

type InitOptions struct{}

func Init(ctx context.Context, _ InitOptions) error {
	_ = ctx

	startDir, err := os.Getwd()
	if err != nil {
		return err
	}

	if _, ok := project.FindManifestDir(startDir); ok {
		return nil
	}

	targetDir := startDir
	if dir, ok := project.FindGodotProjectDir(startDir); ok {
		targetDir = dir
	}

	manifestPath := filepath.Join(targetDir, "gdam.json")
	m := manifest.New()
	if err := manifest.Save(manifestPath, m); err != nil {
		return err
	}

	fmt.Printf("created %s\n", manifestPath)
	return nil
}
