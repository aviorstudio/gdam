alter table public.plugin_versions
  add column if not exists release_tag text;

update public.plugin_versions
set release_tag = sha
where release_tag is null or btrim(release_tag) = '';

alter table public.plugin_versions
  alter column release_tag set not null;

alter table public.plugin_versions
  alter column sha drop not null;

alter table public.plugin_versions
  add constraint plugin_versions_release_tag_not_empty check (btrim(release_tag) <> '');

create unique index if not exists plugin_versions_plugin_release_tag_key
  on public.plugin_versions(plugin_id, release_tag);
