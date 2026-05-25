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

The tag can be any valid GitHub release tag. If the tag field is left blank, GDAM tries `v<version>` first and then `<version>`, for example `v1.2.3` then `1.2.3`.

The asset name can be anything the publisher chooses. That ZIP should contain the addon files at the archive root, including `plugin.cfg`. GDAM installs the asset into its local convention, such as `res://addons/@username_addon/`, regardless of the asset filename.
