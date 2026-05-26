# GDAM

GDAM is the Godot Addon Manager.

Use it to install and track Godot addons from GitHub repositories with a small CLI and a public addon registry.

Website: [gdam.dev](https://gdam.dev)

## Install

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/aviorstudio/gdam/main/scripts/install_cli.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/aviorstudio/gdam/main/scripts/install_cli.sh | VERSION=0.0.1 sh
```

Windows builds are available from [GitHub Releases](https://github.com/aviorstudio/gdam/releases).

## Usage

From a Godot project:

```sh
gdam init
gdam add @username/addon
gdam install
```

Install a specific addon version:

```sh
gdam add @username/addon@1.2.3
```

Remove an addon:

```sh
gdam remove @username/addon
```

Link a local addon while developing it:

```sh
gdam link @username/addon /path/to/addon
gdam unlink @username/addon
```

Check your installed CLI version:

```sh
gdam --version
```

If you hit GitHub rate limits while installing addons, set `GITHUB_TOKEN`.

## Publishing

Registry releases are installed from GitHub Release assets. Publish an addon version with a semver package version such as `1.2.3`, a GitHub release tag, and an asset name.

The tag can be any valid GitHub release tag. The release tag is required when publishing.

The asset name can be anything the publisher chooses. That ZIP should contain the addon files at the archive root, including `plugin.cfg`. GDAM installs the asset into its local convention, such as `res://addons/@username_addon/`, regardless of the asset filename.

For CI publishing, create a secret key from the owner settings page, store it as `GDAM_SECRET_KEY`, and publish releases with:

```sh
gdam publish @username/addon 1.2.3 v1.2.3 @owner_repo.zip
```

Secret keys are scoped to one user or org and can only publish releases for existing addons under that owner. If `ASSET_NAME` is omitted, `gdam publish` uses `@owner_repo.zip` from `GITHUB_REPOSITORY` when available.
