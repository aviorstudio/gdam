# GDAM CLI

Installs Godot addons from GitHub repositories (including monorepo subdirectories) into your project's `addons/` folder and tracks them in `gdam.json`.

`gdam` expects the addon directory to contain a `plugin.cfg` at its root (so it can be enabled automatically in `project.godot`).

## Build

From `cli/`:

```sh
go build ./cmd/gdam
```

## Usage

```sh
gdam init
gdam add @username/addon@1.2.3
gdam add @username/addon
gdam install
gdam remove @username/addon
gdam link @username/addon /absolute/path/to/addons/dir
gdam link @username/addon
gdam unlink @username/addon
gdam unlink --all
```

See [`USAGE.md`](USAGE.md) for complete command behavior and state-dependent cases.

`gdam link` will create an addon entry in `gdam.json` if it doesn't exist yet (as a local-only addon, without a `repo`).

`gdam.json` uses:

```json
{
  "addons": {
    "@user/addon": {
      "repo": "https://github.com/owner/repo/tree/<sha>",
      "version": "1.2.3"
    },
    "@user/monorepo_addon": {
      "repo": "https://github.com/owner/monorepo/tree/<sha>/path/to/addon",
      "version": "1.2.3"
    },
    "@user/other": {
    }
  }
}
```

`gdam.link.json` stores per-user link state and paths (add it to your `.gitignore`):

```json
{
  "addons": {
    "@user/addon": {
      "enabled": true,
      "path": "~/dev/addon"
    },
    "@user/other": {
      "enabled": true,
      "path": "~/dev/other"
    }
  }
}
```

`gdam.json` should not contain any `"link"` fields.

If you hit GitHub rate limits, set `GITHUB_TOKEN`.
