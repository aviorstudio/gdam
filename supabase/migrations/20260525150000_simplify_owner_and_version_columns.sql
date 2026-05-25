alter table public.orgs
  drop column if exists name;

alter table public.profiles
  drop column if exists name;

do $$
begin
  if exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'orgs_profiles' and column_name = 'user_id'
  ) then
    alter table public.orgs_profiles rename column user_id to profile_id;
  end if;
end $$;

alter index if exists orgs_profiles_user_id_idx rename to orgs_profiles_profile_id_idx;

create or replace function public.is_org_admin(target_org_id uuid)
returns boolean
language sql
stable
security definer
set search_path = public
as $$
  select exists (
    select 1 from public.orgs_profiles op
    where op.org_id = target_org_id and op.profile_id = auth.uid() and op.admin
  );
$$;

do $$
begin
  if exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'addon_versions' and column_name = 'release_tag'
  ) then
    alter table public.addon_versions rename column release_tag to tag;
  end if;

  if exists (
    select 1 from information_schema.columns
    where table_schema = 'public' and table_name = 'addon_versions' and column_name = 'asset_name'
  ) then
    alter table public.addon_versions rename column asset_name to asset;
  end if;

  if exists (select 1 from pg_constraint where conrelid = 'public.addon_versions'::regclass and conname = 'addon_versions_release_tag_not_empty') then
    alter table public.addon_versions rename constraint addon_versions_release_tag_not_empty to addon_versions_tag_not_empty;
  end if;

  if exists (select 1 from pg_constraint where conrelid = 'public.addon_versions'::regclass and conname = 'addon_versions_asset_name_not_empty') then
    alter table public.addon_versions rename constraint addon_versions_asset_name_not_empty to addon_versions_asset_not_empty;
  end if;
end $$;

alter index if exists public.addon_versions_addon_release_tag_key rename to addon_versions_addon_tag_key;
