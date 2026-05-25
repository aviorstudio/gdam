alter table public.plugin_versions
  rename column plugin_id to addon_id;

alter table public.plugins
  rename constraint plugins_pkey to addons_pkey;

alter table public.plugins
  rename constraint plugins_exactly_one_owner to addons_exactly_one_owner;

alter index if exists public.plugins_user_name_key rename to addons_user_name_key;
alter index if exists public.plugins_org_name_key rename to addons_org_name_key;
alter index if exists public.plugins_user_id_idx rename to addons_user_id_idx;
alter index if exists public.plugins_org_id_idx rename to addons_org_id_idx;

alter table public.plugin_versions
  rename constraint plugin_versions_pkey to addon_versions_pkey;

alter table public.plugin_versions
  rename constraint plugin_versions_plugin_id_fkey to addon_versions_addon_id_fkey;

alter table public.plugin_versions
  rename constraint plugin_versions_plugin_id_major_minor_patch_key to addon_versions_addon_id_major_minor_patch_key;

alter table public.plugin_versions
  rename constraint plugin_versions_asset_name_not_empty to addon_versions_asset_name_not_empty;

alter table public.plugin_versions
  rename constraint plugin_versions_release_tag_not_empty to addon_versions_release_tag_not_empty;

alter index if exists public.plugin_versions_plugin_id_created_at_idx rename to addon_versions_addon_id_created_at_idx;
alter index if exists public.plugin_versions_plugin_release_tag_key rename to addon_versions_addon_release_tag_key;

alter table public.plugins rename to addons;
alter table public.plugin_versions rename to addon_versions;

alter policy plugins_select_public on public.addons rename to addons_select_public;
alter policy plugins_insert_owner on public.addons rename to addons_insert_owner;
alter policy plugins_update_owner on public.addons rename to addons_update_owner;
alter policy plugins_delete_owner on public.addons rename to addons_delete_owner;

alter policy plugin_versions_select_public on public.addon_versions rename to addon_versions_select_public;
alter policy plugin_versions_insert_owner on public.addon_versions rename to addon_versions_insert_owner;
alter policy plugin_versions_update_owner on public.addon_versions rename to addon_versions_update_owner;
alter policy plugin_versions_delete_owner on public.addon_versions rename to addon_versions_delete_owner;
