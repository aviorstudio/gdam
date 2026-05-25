alter table public.plugin_versions
  add column if not exists asset_name text;

update public.plugin_versions pv
set asset_name = '@' || match.owner || '_' || regexp_replace(match.repo, '\.git$', '', 'i') || '.zip'
from public.plugins p,
     lateral (
       select (regexp_match(p.repo, 'github\.com[:/]([^/]+)/([^/#?]+)'))[1] as owner,
              (regexp_match(p.repo, 'github\.com[:/]([^/]+)/([^/#?]+)'))[2] as repo
     ) match
where pv.plugin_id = p.id
  and (pv.asset_name is null or btrim(pv.asset_name) = '')
  and match.owner is not null
  and match.repo is not null;

update public.plugin_versions
set asset_name = 'addon.zip'
where asset_name is null or btrim(asset_name) = '';

alter table public.plugin_versions
  alter column asset_name set not null;

alter table public.plugin_versions
  add constraint plugin_versions_asset_name_not_empty check (btrim(asset_name) <> '');

alter table public.plugin_versions
  drop column if exists sha;

alter table public.plugins
  drop column if exists path;

alter table public.profiles
  drop column if exists contact_email;

alter table public.orgs
  drop column if exists contact_email;
