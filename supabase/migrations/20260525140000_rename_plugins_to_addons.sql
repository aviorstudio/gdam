do $$
begin
  if to_regclass('public.plugin_versions') is not null
     and exists (
       select 1 from information_schema.columns
       where table_schema = 'public' and table_name = 'plugin_versions' and column_name = 'plugin_id'
     ) then
    alter table public.plugin_versions rename column plugin_id to addon_id;
  end if;
end $$;

do $$
begin
  if to_regclass('public.plugins') is not null then
    if exists (select 1 from pg_constraint where conrelid = 'public.plugins'::regclass and conname = 'plugins_pkey') then
      alter table public.plugins rename constraint plugins_pkey to addons_pkey;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.plugins'::regclass and conname = 'plugins_exactly_one_owner') then
      alter table public.plugins rename constraint plugins_exactly_one_owner to addons_exactly_one_owner;
    end if;
  end if;
end $$;

alter index if exists public.plugins_user_name_key rename to addons_user_name_key;
alter index if exists public.plugins_org_name_key rename to addons_org_name_key;
alter index if exists public.plugins_user_id_idx rename to addons_user_id_idx;
alter index if exists public.plugins_org_id_idx rename to addons_org_id_idx;

do $$
begin
  if to_regclass('public.plugin_versions') is not null then
    if exists (select 1 from pg_constraint where conrelid = 'public.plugin_versions'::regclass and conname = 'plugin_versions_pkey') then
      alter table public.plugin_versions rename constraint plugin_versions_pkey to addon_versions_pkey;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.plugin_versions'::regclass and conname = 'plugin_versions_plugin_id_fkey') then
      alter table public.plugin_versions rename constraint plugin_versions_plugin_id_fkey to addon_versions_addon_id_fkey;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.plugin_versions'::regclass and conname = 'plugin_versions_plugin_id_major_minor_patch_key') then
      alter table public.plugin_versions rename constraint plugin_versions_plugin_id_major_minor_patch_key to addon_versions_addon_id_major_minor_patch_key;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.plugin_versions'::regclass and conname = 'plugin_versions_asset_name_not_empty') then
      alter table public.plugin_versions rename constraint plugin_versions_asset_name_not_empty to addon_versions_asset_name_not_empty;
    end if;

    if exists (select 1 from pg_constraint where conrelid = 'public.plugin_versions'::regclass and conname = 'plugin_versions_release_tag_not_empty') then
      alter table public.plugin_versions rename constraint plugin_versions_release_tag_not_empty to addon_versions_release_tag_not_empty;
    end if;
  end if;
end $$;

alter index if exists public.plugin_versions_plugin_id_created_at_idx rename to addon_versions_addon_id_created_at_idx;
alter index if exists public.plugin_versions_plugin_release_tag_key rename to addon_versions_addon_release_tag_key;

alter table if exists public.plugins rename to addons;
alter table if exists public.plugin_versions rename to addon_versions;

do $$
begin
  if to_regclass('public.addons') is not null then
    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addons' and policyname = 'plugins_select_public') then
      alter policy plugins_select_public on public.addons rename to addons_select_public;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addons' and policyname = 'plugins_insert_owner') then
      alter policy plugins_insert_owner on public.addons rename to addons_insert_owner;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addons' and policyname = 'plugins_update_owner') then
      alter policy plugins_update_owner on public.addons rename to addons_update_owner;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addons' and policyname = 'plugins_delete_owner') then
      alter policy plugins_delete_owner on public.addons rename to addons_delete_owner;
    end if;
  end if;

  if to_regclass('public.addon_versions') is not null then
    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addon_versions' and policyname = 'plugin_versions_select_public') then
      alter policy plugin_versions_select_public on public.addon_versions rename to addon_versions_select_public;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addon_versions' and policyname = 'plugin_versions_insert_owner') then
      alter policy plugin_versions_insert_owner on public.addon_versions rename to addon_versions_insert_owner;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addon_versions' and policyname = 'plugin_versions_update_owner') then
      alter policy plugin_versions_update_owner on public.addon_versions rename to addon_versions_update_owner;
    end if;

    if exists (select 1 from pg_policies where schemaname = 'public' and tablename = 'addon_versions' and policyname = 'plugin_versions_delete_owner') then
      alter policy plugin_versions_delete_owner on public.addon_versions rename to addon_versions_delete_owner;
    end if;
  end if;
end $$;
