package commands

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
	"github.com/aviorstudio/gdam/cli/internal/githubapi"
)

func releaseAssetName(owner, repo string) string {
	return fmt.Sprintf("@%s_%s.zip", strings.TrimSpace(owner), strings.TrimSpace(repo))
}

func prepareGitHubPackageRoot(ctx context.Context, gh *githubapi.Client, owner, repo, ref, repoSubdir, tmpDir string) (string, error) {
	assetZipPath := filepath.Join(tmpDir, "release-asset.zip")
	assetName := releaseAssetName(owner, repo)
	if err := gh.DownloadReleaseAsset(ctx, owner, repo, ref, assetName, assetZipPath); err == nil {
		assetExtractDir := filepath.Join(tmpDir, "release-asset")
		assetRootDir, err := fsutil.ExtractZipAllowRootFiles(assetZipPath, assetExtractDir)
		if err != nil {
			return "", err
		}
		if ok, err := pluginCfgExistsAtDirRoot(assetRootDir); err != nil {
			return "", err
		} else if ok {
			return assetRootDir, nil
		}
	} else if !errors.Is(err, githubapi.ErrReleaseAssetNotFound) {
		return "", err
	}

	zipPath := filepath.Join(tmpDir, "repo.zip")
	if err := gh.DownloadZipball(ctx, owner, repo, ref, zipPath); err != nil {
		return "", err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	rootDir, err := fsutil.ExtractZip(zipPath, extractDir)
	if err != nil {
		return "", err
	}

	return repoSubdirRoot(rootDir, repoSubdir)
}
