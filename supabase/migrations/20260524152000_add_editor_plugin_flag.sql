alter table public.plugins
  add column if not exists editor_plugin boolean not null default false;
