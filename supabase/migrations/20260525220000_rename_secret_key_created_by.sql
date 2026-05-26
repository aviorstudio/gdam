alter table if exists public.secret_keys rename column created_by to profile_id;

alter index if exists public.secret_keys_created_by_created_at_idx rename to secret_keys_profile_id_created_at_idx;

drop policy if exists secret_keys_select_owner on public.secret_keys;
drop policy if exists secret_keys_insert_owner on public.secret_keys;
drop policy if exists secret_keys_delete_owner on public.secret_keys;
drop policy if exists secret_key_scopes_select_owner on public.secret_key_scopes;
drop policy if exists secret_key_scopes_insert_owner on public.secret_key_scopes;
drop policy if exists secret_key_scopes_delete_owner on public.secret_key_scopes;

create policy secret_keys_select_owner on public.secret_keys
  for select using (profile_id = auth.uid());

create policy secret_keys_insert_owner on public.secret_keys
  for insert with check (profile_id = auth.uid());

create policy secret_keys_delete_owner on public.secret_keys
  for delete using (profile_id = auth.uid());

create policy secret_key_scopes_select_owner on public.secret_key_scopes
  for select using (
    exists (
      select 1 from public.secret_keys sk
      where sk.id = secret_key_id and sk.profile_id = auth.uid()
    )
  );

create policy secret_key_scopes_insert_owner on public.secret_key_scopes
  for insert with check (
    exists (
      select 1 from public.secret_keys sk
      where sk.id = secret_key_id and sk.profile_id = auth.uid()
    )
    and (
      profile_id = auth.uid()
      or public.is_org_admin(org_id)
    )
  );

create policy secret_key_scopes_delete_owner on public.secret_key_scopes
  for delete using (
    exists (
      select 1 from public.secret_keys sk
      where sk.id = secret_key_id and sk.profile_id = auth.uid()
    )
  );

create or replace function public.publish_release_with_secret_key(
  secret_key text,
  owner_name text,
  addon_name text,
  version_major integer,
  version_minor integer,
  version_patch integer,
  release_tag text,
  asset_name text
)
returns void
language plpgsql
security definer
set search_path = public, extensions
as $$
declare
  key_row public.secret_keys%rowtype;
  owner_row public.usernames%rowtype;
  addon_row public.addons%rowtype;
  has_scope boolean;
begin
  if length(btrim(coalesce(secret_key, ''))) = 0 then
    raise exception 'Missing secret key';
  end if;
  if length(btrim(coalesce(owner_name, ''))) = 0 or length(btrim(coalesce(addon_name, ''))) = 0 then
    raise exception 'Owner and addon are required';
  end if;
  if version_major < 0 or version_minor < 0 or version_patch < 0 then
    raise exception 'Version numbers must be non-negative';
  end if;
  if length(btrim(coalesce(release_tag, ''))) = 0 then
    raise exception 'Release tag is required';
  end if;
  if length(btrim(coalesce(asset_name, ''))) = 0 then
    raise exception 'Asset name is required';
  end if;

  select * into key_row
  from public.secret_keys
  where token_hash = encode(digest(secret_key, 'sha256'), 'hex')
  limit 1;

  if key_row.id is null then
    raise exception 'Invalid secret key';
  end if;

  select * into owner_row
  from public.usernames
  where name ilike btrim(owner_name)
  limit 1;

  if owner_row.id is null then
    raise exception 'Owner not found';
  end if;

  select exists (
    select 1
    from public.secret_key_scopes scope
    where scope.secret_key_id = key_row.id
      and (
        (owner_row.user_id is not null and scope.profile_id = owner_row.user_id and key_row.profile_id = owner_row.user_id)
        or (owner_row.org_id is not null and scope.org_id = owner_row.org_id and public.is_org_admin_for_profile(owner_row.org_id, key_row.profile_id))
      )
  ) into has_scope;

  if not has_scope then
    raise exception 'Secret key cannot publish to this owner';
  end if;

  select * into addon_row
  from public.addons
  where name = btrim(addon_name)
    and (
      (owner_row.user_id is not null and profile_id = owner_row.user_id)
      or (owner_row.org_id is not null and org_id = owner_row.org_id)
    )
  limit 1;

  if addon_row.id is null then
    raise exception 'Addon not found';
  end if;

  insert into public.releases (addon_id, major, minor, patch, tag, asset)
  values (addon_row.id, version_major, version_minor, version_patch, btrim(release_tag), btrim(asset_name));

  update public.secret_keys
  set last_used_at = now()
  where id = key_row.id;
end;
$$;

grant execute on function public.publish_release_with_secret_key(text, text, text, integer, integer, integer, text, text) to anon, authenticated;
