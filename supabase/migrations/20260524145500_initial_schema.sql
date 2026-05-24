create table public.profiles (
  id uuid primary key references auth.users(id) on delete cascade,
  name text,
  contact_email text,
  created_at timestamptz not null default now()
);

create table public.orgs (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  contact_email text,
  created_at timestamptz not null default now()
);

create table public.orgs_profiles (
  org_id uuid not null references public.orgs(id) on delete cascade,
  user_id uuid not null references public.profiles(id) on delete cascade,
  admin boolean not null default false,
  created_at timestamptz not null default now(),
  primary key (org_id, user_id)
);

create table public.usernames (
  id uuid primary key default gen_random_uuid(),
  username_display text not null,
  username_normal text not null unique,
  user_id uuid references public.profiles(id) on delete cascade,
  org_id uuid references public.orgs(id) on delete cascade,
  created_at timestamptz not null default now(),
  constraint usernames_exactly_one_owner check ((user_id is not null) <> (org_id is not null))
);

create unique index usernames_user_id_key on public.usernames(user_id) where user_id is not null;
create unique index usernames_org_id_key on public.usernames(org_id) where org_id is not null;

create table public.plugins (
  id uuid primary key default gen_random_uuid(),
  user_id uuid references public.profiles(id) on delete cascade,
  org_id uuid references public.orgs(id) on delete cascade,
  name text not null,
  repo text not null,
  path text,
  created_at timestamptz not null default now(),
  constraint plugins_exactly_one_owner check ((user_id is not null) <> (org_id is not null))
);

create unique index plugins_user_name_key on public.plugins(user_id, name) where user_id is not null;
create unique index plugins_org_name_key on public.plugins(org_id, name) where org_id is not null;

create table public.plugin_versions (
  id uuid primary key default gen_random_uuid(),
  plugin_id uuid not null references public.plugins(id) on delete cascade,
  major integer not null check (major >= 0),
  minor integer not null check (minor >= 0),
  patch integer not null check (patch >= 0),
  sha text not null,
  created_at timestamptz not null default now(),
  unique (plugin_id, major, minor, patch)
);

create index orgs_profiles_user_id_idx on public.orgs_profiles(user_id);
create index usernames_user_id_idx on public.usernames(user_id);
create index usernames_org_id_idx on public.usernames(org_id);
create index plugins_user_id_idx on public.plugins(user_id);
create index plugins_org_id_idx on public.plugins(org_id);
create index plugin_versions_plugin_id_created_at_idx on public.plugin_versions(plugin_id, created_at desc);
