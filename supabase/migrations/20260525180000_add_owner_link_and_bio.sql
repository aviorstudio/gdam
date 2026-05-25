alter table public.profiles
  add column if not exists link text,
  add column if not exists bio text;

alter table public.orgs
  add column if not exists link text,
  add column if not exists bio text;

alter table public.profiles
  add constraint profiles_link_not_empty check (link is null or btrim(link) <> ''),
  add constraint profiles_bio_not_empty check (bio is null or btrim(bio) <> '');

alter table public.orgs
  add constraint orgs_link_not_empty check (link is null or btrim(link) <> ''),
  add constraint orgs_bio_not_empty check (bio is null or btrim(bio) <> '');
