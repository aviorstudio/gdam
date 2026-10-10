<!-- Generated from private documentation source. Do not edit directly. Source SHA256: c9e7cf212e113cd39e388c4a50ac35c12883d3f69273820929e24297563e8693 -->

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

Install an exact, case-sensitive GitHub Release tag:

```sh
gdam add @username/addon@Release-1
```

Without a tag, the registry selects the newest stable (non-prerelease) Release.
Exact tags may select prereleases. Tags are opaque: `v1.2.3` and `1.2.3` are
different, and hash-looking text is accepted only when it names a registered
GitHub Release. Tags may contain Git ref characters, including `/`, as long as
the package specification remains unambiguous (tags cannot contain `@`).

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
| `GDAM_API_KEY` | Clerk publishing key used by `gdam publish` |
| `GDAM_SECRET_KEY` | Legacy alias for `GDAM_API_KEY` |
| `GDAM_API_URL`    | Registry API base url, defaults to `https://api.gdam.dev`         |
| `GITHUB_TOKEN`    | Optional GitHub token to avoid rate limits when downloading       |

Set `GDAM_API_URL` to run the CLI against a local registry while developing it.
The CLI ships no credentials of its own: reads are public, and publishing is
authenticated with your Clerk publishing key.

## Project Files

`gdam init` creates a `gdam.json` file in a Godot project. Each registered
dependency stores its exact Release tag in a `tag` field. Old manifests with a
`version` field are intentionally unsupported and must be recreated with
`gdam add @owner/addon@<exact-tag>`; GDAM never rewrites them automatically.
`gdam add`, `gdam remove`, and `gdam install` keep the manifest in sync with
installed addons under `res://addons/`.

Local development links are tracked separately with `gdam.link.json`, so a project can use an unpublished local addon without changing the published dependency manifest.

## Publishing Addons

Registry releases are installed from GitHub Release assets. Publish one exact
GitHub Release tag and, optionally, an asset selector. There is no separate
semantic package version.

The asset name can be anything the publisher chooses. That ZIP should contain the addon files at the archive root, including `plugin.cfg`. GDAM installs the asset into its local convention, such as `res://addons/@username_addon/`, regardless of the asset filename.

For GitHub Actions, use [gdam-actions publish](https://github.com/aviorstudio/gdam-actions)
v0.3.0 or newer with `permissions: id-token: write`; publishing uses GitHub OIDC
and needs no stored publishing key.

For manual publishing, create a publishing key from the app's settings page.
GDAM's Clerk tenant issues the `ak_…` key with the owner scopes you select.
Set it as `GDAM_API_KEY` and publish an existing registered addon's release with:

```sh
gdam publish @username/addon Release-1 @owner_repo.zip
```

Publishing keys are scoped to selected users or orgs and can only publish releases for existing addons under that owner. The CLI reads the registered repository and complete release facts from GitHub
before posting to the index. Existing `gdam_sk_…` keys and `GDAM_SECRET_KEY`
remain supported during the transition. If `ASSET_NAME` is omitted, `gdam publish` uses `@owner_repo.zip` from `GITHUB_REPOSITORY` when available.

## Download integrity and limits

The registry supplies the GitHub Release ID, exact tag, commit SHA, asset ID,
asset name, SHA-256 digest, publication time, and prerelease state. Before each
install, GDAM rechecks that identity with GitHub, downloads through the immutable
asset-ID endpoint, and verifies the digest before extraction. Any release, tag,
commit, asset, digest, truncation, or archive-layout drift fails closed.

Registry requests time out after 30 seconds and response bodies are limited to
4 MiB. Asset downloads time out after two
minutes, follow at most five redirects, and are limited to 128 MiB. Authorization
is removed on cross-origin redirects. Extraction rejects absolute paths, `..`
traversal, backslashes, and symlinks; it permits at most 10,000 entries and 512
MiB total uncompressed content. ZIP assets must contain `plugin.cfg` at the
archive root.


## License

MIT.


## License

MIT.
