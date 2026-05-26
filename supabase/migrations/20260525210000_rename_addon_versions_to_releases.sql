alter table if exists public.addon_versions rename to releases;

do $$
begin
  if to_regclass('public.releases') is not null then
    if exists (select 1 from pg_constraint where conrelid = 'public.releases'::regclass and conname = 'addon_versions_pkey') then
      alter table public.releases rename constraint addon_versions_pkey to releases_pkey;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.releases'::regclass and conname = 'addon_versions_addon_id_fkey') then
      alter table public.releases rename constraint addon_versions_addon_id_fkey to releases_addon_id_fkey;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.releases'::regclass and conname = 'addon_versions_addon_id_major_minor_patch_key') then
      alter table public.releases rename constraint addon_versions_addon_id_major_minor_patch_key to releases_addon_id_major_minor_patch_key;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.releases'::regclass and conname = 'addon_versions_tag_not_empty') then
      alter table public.releases rename constraint addon_versions_tag_not_empty to releases_tag_not_empty;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.releases'::regclass and conname = 'addon_versions_asset_not_empty') then
      alter table public.releases rename constraint addon_versions_asset_not_empty to releases_asset_not_empty;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'releases' and policyname = 'addon_versions_select_public') then
      alter policy addon_versions_select_public on public.releases rename to releases_select_public;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'releases' and policyname = 'addon_versions_insert_owner') then
      alter policy addon_versions_insert_owner on public.releases rename to releases_insert_owner;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'releases' and policyname = 'addon_versions_update_owner') then
      alter policy addon_versions_update_owner on public.releases rename to releases_update_owner;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'releases' and policyname = 'addon_versions_delete_owner') then
      alter policy addon_versions_delete_owner on public.releases rename to releases_delete_owner;
    end if;
  end if;
end $$;

alter index if exists public.addon_versions_addon_id_created_at_idx rename to releases_addon_id_created_at_idx;
alter index if exists public.addon_versions_addon_tag_key rename to releases_addon_tag_key;

drop function if exists public.publish_addon_version_with_secret_key(text, text, text, integer, integer, integer, text, text);

create or replace function public.publish_release_with_secret_key(
  secret_key text,
  owner_name text,
  addon_name text,
  version_major integer,
  version_minor integer,
  version_patch integer,
  release_tag text,
  asset_name text
)
returns void
language plpgsql
security definer
set search_path = public, extensions
as $$
declare
  key_row public.secret_keys%rowtype;
  owner_row public.usernames%rowtype;
  addon_row public.addons%rowtype;
  has_scope boolean;
begin
  if length(btrim(coalesce(secret_key, ''))) = 0 then
    raise exception 'Missing secret key';
  end if;
  if length(btrim(coalesce(owner_name, ''))) = 0 or length(btrim(coalesce(addon_name, ''))) = 0 then
    raise exception 'Owner and addon are required';
  end if;
  if version_major < 0 or version_minor < 0 or version_patch < 0 then
    raise exception 'Version numbers must be non-negative';
  end if;
  if length(btrim(coalesce(release_tag, ''))) = 0 then
    raise exception 'Release tag is required';
  end if;
  if length(btrim(coalesce(asset_name, ''))) = 0 then
    raise exception 'Asset name is required';
  end if;

  select * into key_row
  from public.secret_keys
  where token_hash = encode(digest(secret_key, 'sha256'), 'hex')
  limit 1;

  if key_row.id is null then
    raise exception 'Invalid secret key';
  end if;

  select * into owner_row
  from public.usernames
  where name ilike btrim(owner_name)
  limit 1;

  if owner_row.id is null then
    raise exception 'Owner not found';
  end if;

  select exists (
    select 1
    from public.secret_key_scopes scope
    where scope.secret_key_id = key_row.id
      and (
        (owner_row.user_id is not null and scope.profile_id = owner_row.user_id and key_row.created_by = owner_row.user_id)
        or (owner_row.org_id is not null and scope.org_id = owner_row.org_id and public.is_org_admin_for_profile(owner_row.org_id, key_row.created_by))
      )
  ) into has_scope;

  if not has_scope then
    raise exception 'Secret key cannot publish to this owner';
  end if;

  select * into addon_row
  from public.addons
  where name = btrim(addon_name)
    and (
      (owner_row.user_id is not null and profile_id = owner_row.user_id)
      or (owner_row.org_id is not null and org_id = owner_row.org_id)
    )
  limit 1;

  if addon_row.id is null then
    raise exception 'Addon not found';
  end if;

  insert into public.releases (addon_id, major, minor, patch, tag, asset)
  values (addon_row.id, version_major, version_minor, version_patch, btrim(release_tag), btrim(asset_name));

  update public.secret_keys
  set last_used_at = now()
  where id = key_row.id;
end;
$$;

grant execute on function public.publish_release_with_secret_key(text, text, text, integer, integer, integer, text, text) to anon, authenticated;
