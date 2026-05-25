alter table public.addons
  rename column user_id to profile_id;

alter index if exists public.addons_user_name_key rename to addons_profile_name_key;
alter index if exists public.addons_user_id_idx rename to addons_profile_id_idx;

do $$
begin
  if exists (select 1 from pg_constraint where conrelid = 'public.addons'::regclass and conname = 'plugins_user_id_fkey') then
    alter table public.addons rename constraint plugins_user_id_fkey to addons_profile_id_fkey;
  elsif exists (select 1 from pg_constraint where conrelid = 'public.addons'::regclass and conname = 'addons_user_id_fkey') then
    alter table public.addons rename constraint addons_user_id_fkey to addons_profile_id_fkey;
  end if;
end $$;
