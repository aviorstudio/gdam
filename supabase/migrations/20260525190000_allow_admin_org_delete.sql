create policy orgs_delete_admin on public.orgs
  for delete using (public.is_org_admin(id));
