# GDAM

GDAM is the Godot Addon Manager.

Use it to install, link, remove, and publish Godot addons from GitHub release assets through a small CLI and the public registry at [gdam.dev](https://gdam.dev).

## What This Repo Contains

- `cli/`: Go source for the `gdam` command-line tool.
- `web/`: Astro/Qwik registry website and owner/addon management UI.
- `supabase/`: database migrations, local Supabase config, and server-side registry schema.
- `scripts/`: local development, CI, database, and CLI build helpers.
- `.github/workflows/ci.yml`: tests the CLI, builds the web app, runs Playwright checks, validates Supabase RLS, and runs CLI integration checks.
- `.github/workflows/cd.yml`: deploys Supabase migrations.
- `.github/workflows/release.yml`: builds and publishes CLI release binaries.

## Install The CLI

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/aviorstudio/gdam/main/scripts/install_cli.sh | sh
```

Install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/aviorstudio/gdam/main/scripts/install_cli.sh | VERSION=0.0.1 sh
```

Windows builds are available from [GitHub Releases](https://github.com/aviorstudio/gdam/releases).

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

## Web Development

Start Supabase from the repository root:

```sh
./scripts/db_start.sh
```

Seed a local dev user:

```sh
./scripts/db_seed.sh
```

The default seeded login is `test@gdam.dev` / `password123` with username `@dev`.

Create `web/.env.local` with the local API URL and publishable key shown by `supabase status`:

```sh
SUPABASE_URL=http://127.0.0.1:54421
SUPABASE_PUBLISHABLE_KEY=<publishable key from supabase status>
```

Run the web app from `web/`:

```sh
bun install
bun run dev
```

Open Supabase Studio at `http://127.0.0.1:54423`.

To reset the local database and re-run migrations:

```sh
supabase db reset
```

The CLI reads `cli/.env` during local development. It also accepts `SUPABASE_URL` and `SUPABASE_PUBLISHABLE_KEY` from the shell, or `GDAM_SUPABASE_URL` and `GDAM_SUPABASE_PUBLISHABLE_KEY`, to point `gdam` at another Supabase project.

## Versioning And Releases

CLI releases use `cli-v*` tags. The manual release workflow must run from `main`, accepts a `patch`, `minor`, or `major` bump, runs Go tests, injects version/build metadata, and builds `gdam` binaries for Linux, macOS, and Windows with checksums.

Supabase migrations deploy through the CD workflow. Web deployment is managed separately from CLI releases.

## Testing And CI

Run the main local checks with:

```sh
cd cli && go test ./...
./scripts/cli_build.sh
cd web && bun install && bun run build
```

Integration checks use local Supabase and Playwright:

```sh
./scripts/db_start.sh
supabase db reset
./scripts/db_seed.sh
cd web && bun run test:e2e
./scripts/ci_supabase_rls.sh
./scripts/ci_cli_integration.sh
```

CI runs CLI tests/builds, web builds, Playwright tests, Supabase RLS tests, and CLI integration tests.

## License

MIT
