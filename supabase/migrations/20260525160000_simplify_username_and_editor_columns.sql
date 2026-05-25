alter table public.addons
  rename column editor_plugin to editor;

alter table public.usernames
  rename column username_display to name;

alter table public.usernames
  drop column if exists username_normal;

alter table public.usernames
  add constraint usernames_name_not_empty check (btrim(name) <> '');

create unique index usernames_name_lower_key on public.usernames(lower(name));
