alter table public.profiles enable row level security;
alter table public.orgs enable row level security;
alter table public.orgs_profiles enable row level security;
alter table public.usernames enable row level security;
alter table public.plugins enable row level security;
alter table public.plugin_versions enable row level security;

create or replace function public.is_org_admin(target_org_id uuid)
returns boolean
language sql
stable
security definer
set search_path = public
as $$
  select exists (
    select 1 from public.orgs_profiles op
    where op.org_id = target_org_id and op.user_id = auth.uid() and op.admin
  );
$$;

create or replace function public.org_has_no_members(target_org_id uuid)
returns boolean
language sql
stable
security definer
set search_path = public
as $$
  select not exists (
    select 1 from public.orgs_profiles op
    where op.org_id = target_org_id
  );
$$;

create policy profiles_select_public on public.profiles
  for select using (true);

create policy profiles_insert_own on public.profiles
  for insert with check (auth.uid() = id);

create policy profiles_update_own on public.profiles
  for update using (auth.uid() = id) with check (auth.uid() = id);

create policy orgs_select_public on public.orgs
  for select using (true);

create policy orgs_insert_authenticated on public.orgs
  for insert with check (auth.uid() is not null);

create policy orgs_update_admin on public.orgs
  for update using (public.is_org_admin(id)) with check (public.is_org_admin(id));

create policy orgs_profiles_select_public on public.orgs_profiles
  for select using (true);

create policy orgs_profiles_insert_admin on public.orgs_profiles
  for insert with check (
    public.is_org_admin(org_id)
    or (user_id = auth.uid() and admin and public.org_has_no_members(org_id))
  );

create policy orgs_profiles_update_admin on public.orgs_profiles
  for update using (public.is_org_admin(org_id)) with check (public.is_org_admin(org_id));

create policy usernames_select_public on public.usernames
  for select using (true);

create policy usernames_insert_owner on public.usernames
  for insert with check (
    user_id = auth.uid()
    or public.is_org_admin(org_id)
  );

create policy usernames_update_owner on public.usernames
  for update using (
    user_id = auth.uid()
    or public.is_org_admin(org_id)
  ) with check (
    user_id = auth.uid()
    or public.is_org_admin(org_id)
  );

create policy plugins_select_public on public.plugins
  for select using (true);

create policy plugins_insert_owner on public.plugins
  for insert with check (
    user_id = auth.uid()
    or public.is_org_admin(org_id)
  );

create policy plugins_update_owner on public.plugins
  for update using (
    user_id = auth.uid()
    or public.is_org_admin(org_id)
  ) with check (
    user_id = auth.uid()
    or public.is_org_admin(org_id)
  );

create policy plugins_delete_owner on public.plugins
  for delete using (
    user_id = auth.uid()
    or public.is_org_admin(org_id)
  );

create policy plugin_versions_select_public on public.plugin_versions
  for select using (true);

create policy plugin_versions_insert_owner on public.plugin_versions
  for insert with check (
    exists (
      select 1 from public.plugins p
      where p.id = plugin_id
        and (
          p.user_id = auth.uid()
          or public.is_org_admin(p.org_id)
        )
    )
  );

create policy plugin_versions_update_owner on public.plugin_versions
  for update using (
    exists (
      select 1 from public.plugins p
      where p.id = plugin_id
        and (
          p.user_id = auth.uid()
          or public.is_org_admin(p.org_id)
        )
    )
  ) with check (
    exists (
      select 1 from public.plugins p
      where p.id = plugin_id
        and (
          p.user_id = auth.uid()
          or public.is_org_admin(p.org_id)
        )
    )
  );

create policy plugin_versions_delete_owner on public.plugin_versions
  for delete using (
    exists (
      select 1 from public.plugins p
      where p.id = plugin_id
        and (
          p.user_id = auth.uid()
          or public.is_org_admin(p.org_id)
        )
    )
  );
