package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aviorstudio/gdam/internal/fsutil"
	"github.com/aviorstudio/gdam/internal/gdamdb"
	"github.com/aviorstudio/gdam/internal/githubapi"
)

func releaseAssetName(owner, repo string) string {
	return fmt.Sprintf("@%s_%s.zip", strings.TrimSpace(owner), strings.TrimSpace(repo))
}

func prepareGitHubPackageRoot(ctx context.Context, gh *githubapi.Client, resolved gdamdb.ResolvedAddon, tmpDir string) (string, error) {
	assetZipPath := filepath.Join(tmpDir, "release-asset.zip")
	assetName := strings.TrimSpace(resolved.AssetName)
	if assetName == "" {
		return "", fmt.Errorf("missing release asset name")
	}
	identity := githubapi.ReleaseIdentity{ReleaseID: resolved.GitHubReleaseID, TagName: resolved.TagName, CommitSHA: resolved.CommitSHA, AssetID: resolved.AssetID, AssetName: resolved.AssetName, Digest: resolved.AssetDigest, PublishedAt: resolved.PublishedAt, Prerelease: resolved.Prerelease}
	if err := gh.DownloadVerifiedReleaseAsset(ctx, resolved.GitHubOwner, resolved.GitHubRepo, identity, assetZipPath); err != nil {
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
