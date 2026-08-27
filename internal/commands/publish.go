package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aviorstudio/gdam/internal/gdamdb"
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

	secretKey := strings.TrimSpace(os.Getenv("GDAM_SECRET_KEY"))
	if secretKey == "" {
		return fmt.Errorf("%w: missing GDAM_SECRET_KEY", ErrUserInput)
	}

	db := gdamdb.NewDefaultClient()
	if err := db.PublishRelease(ctx, gdamdb.PublishReleaseInput{
		SecretKey: secretKey,
		Owner:     pkg.Owner,
		Addon:     pkg.Repo,
		TagName:   releaseTag,
		AssetName: assetName,
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
