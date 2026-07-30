# GDAM

GDAM is the Godot Addon Manager.

Use it to install, link, remove, and publish Godot addons from GitHub release assets through a small CLI and the public registry at [gdam.dev](https://gdam.dev).

## Install The CLI

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/aviorstudio/gdam/main/scripts/install_cli.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/aviorstudio/gdam/main/scripts/install_cli.sh | VERSION=0.0.5 sh
```

Windows builds are available from [GitHub Releases](https://github.com/aviorstudio/gdam/releases).

If you have the Go toolchain, install it from the module path instead:

```sh
go install github.com/aviorstudio/gdam@latest
```

Or run it without installing anything:

```sh
go run github.com/aviorstudio/gdam@latest --version
```

Replace `@latest` with a release tag to pin a version. This route compiles from source; the installer script and the release archives ship prebuilt binaries.

## CLI Usage

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

## Environment

| Variable          | Purpose                                                          |
| ----------------- | ---------------------------------------------------------------- |
| `GDAM_SECRET_KEY` | Secret key used by `gdam publish` in CI                           |
| `GDAM_API_URL`    | Registry API base url, defaults to `https://api.gdam.dev`         |
| `GITHUB_TOKEN`    | Optional GitHub token to avoid rate limits when downloading       |

Set `GDAM_API_URL` to run the CLI against a local registry while developing it.
The CLI ships no credentials of its own: reads are public, and publishing is
authenticated with your secret key.

## Project Files

`gdam init` creates a `gdam.json` file in a Godot project. `gdam add`, `gdam remove`, and `gdam install` keep that manifest in sync with installed addons under `res://addons/`.

Local development links are tracked separately with `gdam.link.json`, so a project can use an unpublished local addon without changing the published dependency manifest.

## Publishing Addons

Registry releases are installed from GitHub Release assets. Publish an addon version with a semver package version such as `1.2.3`, a GitHub release tag, and an asset name.

The tag can be any valid GitHub release tag. The release tag is required when publishing.

The asset name can be anything the publisher chooses. That ZIP should contain the addon files at the archive root, including `plugin.cfg`. GDAM installs the asset into its local convention, such as `res://addons/@username_addon/`, regardless of the asset filename.

For CI publishing, create a secret key from the owner settings page, store it as `GDAM_SECRET_KEY`, and publish releases with:

```sh
gdam publish @username/addon 1.2.3 v1.2.3 @owner_repo.zip
```

Secret keys are scoped to one user or org and can only publish releases for existing addons under that owner. If `ASSET_NAME` is omitted, `gdam publish` uses `@owner_repo.zip` from `GITHUB_REPOSITORY` when available.

## Development

Build the CLI:

```sh
./scripts/cli_build.sh
```

Run tests:

```sh
go test ./...
```

CLI releases use `v*` tags. The manual release workflow must run from `main`, accepts a `patch`, `minor`, or `major` bump, runs Go tests, injects version/build metadata, and builds `gdam` binaries for Linux, macOS, and Windows with checksums. It needs no repository secrets — the binary contains no keys.

## License

MIT
