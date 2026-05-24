# GDAM Web

## Local Development

Start Supabase from the repository root:

```sh
./scripts/db_start.sh
```

Seed a local dev user:

```sh
./scripts/db_seed.sh
```

The default seeded login is `dev@gdam.local` / `password123` with username `@dev`.

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
