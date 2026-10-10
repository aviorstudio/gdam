<!-- Generated from private documentation source. Do not edit directly. Source SHA256: c169bb942e22f93202dbbfedd7cc6774eec3af13b997fe9ce71696cb1a4c93b6 -->

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

## Dependencies

An addon can depend on other addons. Its release asset ships a `gdam.json` at
its root, in the same shape as a project's, naming the exact tag of each addon
it needs:

```json
{ "addons": { "@aviorstudio/gd-session": { "tag": "v0.0.1" } } }
```

The publisher records that declaration with the release, and the registry
refuses a release whose dependency is not itself published. `gdam install`
reads the declarations from the registry, works out the whole set before
downloading anything, and installs every release in it.

A Godot project has one `addons/` directory and one global class namespace,
so one copy of an addon normally serves everyone: it is installed at
`addons/<owner_addon>` (hoisted). When exact pins disagree, the project's own
pin takes that address and a consumer that pinned a different tag gets its own
copy nested inside its directory, at `addons/<consumer>/.gdam/<owner_addon>`,
with fresh script UIDs. Nothing errors on a disagreement; `gdam install`
prints where each nested copy went and which addon asked for it.

An addon reaches its dependencies only through the file gdam generates for it,
`.gdam/deps.gd`, never by a hard-coded `res://addons/...` path, because the
address differs between the hoisted and nested cases:

```gdscript
const Deps = preload("../.gdam/deps.gd")

func make_adapter():
	return Deps.GdSession_credential_adapter.new()
```

`deps.gd` holds a `PATHS` dictionary (addon name to its `res://` directory) and
one `preload` constant per dependency script, named `<Addon>_<path>` with the
owner and a leading `src/` dropped: `GdSession_credential_adapter` for
`@aviorstudio/gd-session`'s `src/credential_adapter.gd`. It is rewritten on
every install and must not be committed by the consuming project or shipped in
a release asset.

Two rules make a nested second copy possible. An addon that others depend on
declares no `class_name`: Godot registers class names project-wide, so a second
copy would fail to load, and the registry refuses such a release as a
dependency. And an object that crosses an addon boundary (a credential adapter
a game hands to gd-clerk, say) is matched by its methods, not by `is` against
the dependency's class, because the game and the addon may hold different
copies of that class.

In an addon's own repository, `"package": "addon"` in the project's `gdam.json`
names the addon's source directory. `gdam install` then checks the directory's
own `gdam.json` against the project's pins (they must name the same tags) and
writes `addon/.gdam/deps.gd` there, so the addon under development reaches its
dependencies exactly as an installed copy does.

## The lock file

`gdam.lock` records what the project's pins resolved to: every release in the
set, its GitHub release id, asset id and SHA-256, and every path it was
installed to with the declaration that asked for it. `gdam add`, `gdam update`
and `gdam remove` resolve and rewrite it. `gdam install` installs from the lock
alone when it still answers `gdam.json`, without consulting the registry, and
verifies every download against the locked identity before extracting a byte.
When `gdam.json` has changed it re-resolves and rewrites the lock;
`gdam install --frozen-lockfile` fails instead, which is what CI should run.
Commit the lock.

`gdam update @owner/addon@<tag>` moves one pin and re-resolves; it is the same
command as `gdam add`.

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
before posting to the index. If `ASSET_NAME` is omitted, `gdam publish` uses `@owner_repo.zip` from `GITHUB_REPOSITORY` when available.

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
