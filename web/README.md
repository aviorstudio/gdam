# GDAM Web

## Local Development

Start Supabase from the repository root:

```sh
supabase start
```

Create `web/.env.local` with the local API URL and publishable key shown by `supabase status`:

```sh
SUPABASE_URL=http://127.0.0.1:54421
SUPABASE_ANON_KEY=<publishable key from supabase status>
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

The CLI also reads `SUPABASE_URL` and `SUPABASE_ANON_KEY`. Set those in your shell, or use `GDAM_SUPABASE_URL` and `GDAM_SUPABASE_ANON_KEY`, to point `gdam` at the local database.
