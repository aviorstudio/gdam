package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/gdamdb"
	"github.com/aviorstudio/gdam/cli/internal/semver"
	"github.com/aviorstudio/gdam/cli/internal/spec"
)

type PublishOptions struct {
	Spec       string
	Version    string
	ReleaseTag string
	AssetName  string
}

func Publish(ctx context.Context, opts PublishOptions) error {
	pkg, err := spec.ParsePackageSpec(opts.Spec)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	if strings.TrimSpace(pkg.Version) != "" {
		return fmt.Errorf("%w: publish version must be a separate argument", ErrUserInput)
	}

	version, ok := semver.Parse(opts.Version)
	if !ok || len(version.Pre) > 0 {
		return fmt.Errorf("%w: version must be in MAJOR.MINOR.PATCH format", ErrUserInput)
	}

	releaseTag := strings.TrimSpace(opts.ReleaseTag)
	if releaseTag == "" {
		return fmt.Errorf("%w: release tag is required", ErrUserInput)
	}

	assetName := strings.TrimSpace(opts.AssetName)
	if assetName == "" {
		assetName = defaultCIAssetName()
	}
	if assetName == "" {
		return fmt.Errorf("%w: asset name is required when GITHUB_REPOSITORY is not set", ErrUserInput)
	}

	secretKey := strings.TrimSpace(os.Getenv("GDAM_SECRET_KEY"))
	if secretKey == "" {
		return fmt.Errorf("%w: missing GDAM_SECRET_KEY", ErrUserInput)
	}

	db := gdamdb.NewDefaultClient()
	if err := db.PublishRelease(ctx, gdamdb.PublishReleaseInput{
		SecretKey:  secretKey,
		Owner:      pkg.Owner,
		Addon:      pkg.Repo,
		Major:      version.Major,
		Minor:      version.Minor,
		Patch:      version.Patch,
		ReleaseTag: releaseTag,
		AssetName:  assetName,
	}); err != nil {
		return err
	}

	fmt.Printf("published %s@%d.%d.%d\n", pkg.Name(), version.Major, version.Minor, version.Patch)
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
