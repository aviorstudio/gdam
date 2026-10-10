package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aviorstudio/gdam/internal/gdamdb"
	"github.com/aviorstudio/gdam/internal/githubapi"
	"github.com/aviorstudio/gdam/internal/spec"
)

type PublishOptions struct {
	Spec      string
	TagName   string
	AssetName string
}

func Publish(ctx context.Context, opts PublishOptions) error {
	pkg, err := spec.ParsePackageSpec(opts.Spec)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	if strings.TrimSpace(pkg.Tag) != "" {
		return fmt.Errorf("%w: publish tag must be a separate argument", ErrUserInput)
	}

	releaseTag := strings.TrimSpace(opts.TagName)
	if releaseTag == "" {
		return fmt.Errorf("%w: release tag is required", ErrUserInput)
	}

	assetName := strings.TrimSpace(opts.AssetName)
	if assetName == "" {
		assetName = defaultCIAssetName()
	}

	secretKey := strings.TrimSpace(os.Getenv("GDAM_API_KEY"))
	if secretKey == "" {
		return fmt.Errorf("%w: missing GDAM_API_KEY", ErrUserInput)
	}

	if !strings.HasPrefix(secretKey, "ak_") {
		return fmt.Errorf("%w: GDAM_API_KEY must be a Clerk publishing key", ErrUserInput)
	}

	db := gdamdb.NewDefaultClient()
	repository, err := db.AddonRepository(ctx, pkg.Owner, pkg.Repo)
	if err != nil {
		return err
	}
	owner, repo, _ := strings.Cut(repository, "/")
	gh := githubapi.NewClient(os.Getenv("GITHUB_TOKEN"))
	identity, size, err := gh.ReadReleaseIdentity(ctx, owner, repo, releaseTag, assetName)
	if err != nil {
		return err
	}
	if err := db.PublishRelease(ctx, gdamdb.PublishReleaseInput{
		APIKey:    secretKey,
		Owner:     pkg.Owner,
		Addon:     pkg.Repo,
		TagName:   releaseTag,
		AssetName: identity.AssetName,
		ReleaseID: identity.ReleaseID, CommitSHA: identity.CommitSHA, AssetID: identity.AssetID, AssetDigest: identity.Digest, AssetSize: size, PublishedAt: identity.PublishedAt, Prerelease: identity.Prerelease,
	}); err != nil {
		return err
	}

	fmt.Printf("published %s@%s\n", pkg.Name(), releaseTag)
	return nil
}

func defaultCIAssetName() string {
	repo := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY"))
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || strings.TrimSpace(owner) == "" || strings.TrimSpace(name) == "" {
		return ""
	}
	return releaseAssetName(strings.TrimSpace(owner), strings.TrimSpace(name))
}
