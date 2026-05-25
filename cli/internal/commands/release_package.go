package commands

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
	"github.com/aviorstudio/gdam/cli/internal/githubapi"
)

var commitSHARefPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func releaseAssetName(owner, repo string) string {
	return fmt.Sprintf("@%s_%s.zip", strings.TrimSpace(owner), strings.TrimSpace(repo))
}

func prepareGitHubPackageRoot(ctx context.Context, gh *githubapi.Client, owner, repo, ref, assetName, tmpDir string) (string, error) {
	assetZipPath := filepath.Join(tmpDir, "release-asset.zip")
	assetName = strings.TrimSpace(assetName)
	if assetName == "" {
		return "", fmt.Errorf("missing release asset name")
	}
	if err := gh.DownloadReleaseAsset(ctx, owner, repo, ref, assetName, assetZipPath); err != nil {
		if errors.Is(err, githubapi.ErrReleaseAssetNotFound) && commitSHARefPattern.MatchString(strings.TrimSpace(ref)) {
			return prepareGitHubTreePackageRoot(ctx, gh, owner, repo, ref, tmpDir)
		}
		return "", err
	}

	assetExtractDir := filepath.Join(tmpDir, "release-asset")
	assetRootDir, err := fsutil.ExtractZipAllowRootFiles(assetZipPath, assetExtractDir)
	if err != nil {
		return "", err
	}
	if ok, err := pluginCfgExistsAtDirRoot(assetRootDir); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("release asset %s is missing plugin.cfg at archive root", assetName)
	}
	return assetRootDir, nil
}

func prepareGitHubTreePackageRoot(ctx context.Context, gh *githubapi.Client, owner, repo, ref, tmpDir string) (string, error) {
	zipPath := filepath.Join(tmpDir, "repo.zip")
	if err := gh.DownloadZipball(ctx, owner, repo, ref, zipPath); err != nil {
		return "", err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	rootDir, err := fsutil.ExtractZip(zipPath, extractDir)
	if err != nil {
		return "", err
	}
	return rootDir, nil
}
