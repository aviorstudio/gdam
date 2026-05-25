import type { SupabaseClient } from '@supabase/supabase-js';

export type ProfileUpsert = {
  id: string;
  name?: string | null;
};

export const profilesDto = {
  getById: (client: SupabaseClient, id: string) => client.from('profiles').select('*').eq('id', id).maybeSingle(),
  upsert: (client: SupabaseClient, payload: ProfileUpsert) => client.from('profiles').upsert(payload),
};

export type OrgInsert = {
  name: string;
};

export const orgsDto = {
  insert: (client: SupabaseClient, payload: OrgInsert) => client.from('orgs').insert(payload).select('*').maybeSingle(),
};

export type OrgProfileInsert = {
  org_id: string;
  user_id: string;
  admin?: boolean;
};

export const orgsProfilesDto = {
  insert: (client: SupabaseClient, payload: OrgProfileInsert) =>
    client.from('orgs_profiles').insert(payload).select('*').maybeSingle(),
  getByOrgIdAndUserId: (client: SupabaseClient, orgId: string, userId: string) =>
    client.from('orgs_profiles').select('*').eq('org_id', orgId).eq('user_id', userId).maybeSingle(),
  listByUserId: (client: SupabaseClient, userId: string) =>
    client.from('orgs_profiles').select('org_id,admin,created_at').eq('user_id', userId).order('created_at', {
      ascending: false,
    }),
};

export type UsernameInsert = {
  username_display: string;
  username_normal: string;
  user_id?: string | null;
  org_id?: string | null;
};

export const usernamesDto = {
  insert: (client: SupabaseClient, payload: UsernameInsert) => client.from('usernames').insert(payload),
  getByUserId: (client: SupabaseClient, userId: string) => client.from('usernames').select('*').eq('user_id', userId),
  listByUserIds: (client: SupabaseClient, userIds: string[]) =>
    client.from('usernames').select('username_display,user_id,org_id').in('user_id', userIds),
  listByOrgIds: (client: SupabaseClient, orgIds: string[]) =>
    client.from('usernames').select('username_display,user_id,org_id').in('org_id', orgIds),
  getByUsernameNormal: (client: SupabaseClient, usernameNormal: string) =>
    client.from('usernames').select('*').eq('username_normal', usernameNormal).maybeSingle(),
};

export type AddonInsert = {
  user_id?: string | null;
  org_id?: string | null;
  name: string;
  repo: string;
  editor_plugin?: boolean;
};

export const addonsDto = {
  insert: (client: SupabaseClient, payload: AddonInsert) =>
    client.from('addons').insert(payload).select('*').maybeSingle(),
  listAll: async (client: SupabaseClient) => {
    return client
      .from('addons')
      .select('id,name,repo,editor_plugin,created_at,user_id,org_id')
      .order('created_at', { ascending: false });
  },
  listByUserId: (client: SupabaseClient, userId: string) =>
    client.from('addons').select('*').eq('user_id', userId).order('created_at', { ascending: false }),
  listByOrgId: (client: SupabaseClient, orgId: string) =>
    client.from('addons').select('*').eq('org_id', orgId).order('created_at', { ascending: false }),
  getByUserIdAndName: (client: SupabaseClient, userId: string, name: string) =>
    client.from('addons').select('*').eq('user_id', userId).eq('name', name).maybeSingle(),
  getByOrgIdAndName: (client: SupabaseClient, orgId: string, name: string) =>
    client.from('addons').select('*').eq('org_id', orgId).eq('name', name).maybeSingle(),
};

export const addonVersionsDto = {
  insert: (client: SupabaseClient, payload: { addon_id: string; major: number; minor: number; patch: number; release_tag: string; asset_name: string }) =>
    client.from('addon_versions').insert(payload).select('*').maybeSingle(),
  listByAddonIds: (client: SupabaseClient, addonIds: string[]) =>
    client
      .from('addon_versions')
      .select('*')
      .in('addon_id', addonIds)
      .order('created_at', { ascending: false }),
  listByAddonId: (client: SupabaseClient, addonId: string) =>
    client.from('addon_versions').select('*').eq('addon_id', addonId).order('created_at', { ascending: false }),
};
