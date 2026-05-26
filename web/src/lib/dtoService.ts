import type { SupabaseClient } from '@supabase/supabase-js';

export type ProfileUpsert = {
  id: string;
  link?: string | null;
  bio?: string | null;
};

export const profilesDto = {
  getById: (client: SupabaseClient, id: string) => client.from('profiles').select('*').eq('id', id).maybeSingle(),
  upsert: (client: SupabaseClient, payload: ProfileUpsert) => client.from('profiles').upsert(payload),
  updateById: (client: SupabaseClient, id: string, payload: { link?: string | null; bio?: string | null }) =>
    client.from('profiles').update(payload).eq('id', id),
};

export const orgsDto = {
  getById: (client: SupabaseClient, id: string) => client.from('orgs').select('*').eq('id', id).maybeSingle(),
  insert: (client: SupabaseClient, payload: { link?: string | null; bio?: string | null } = {}) =>
    client.from('orgs').insert(payload).select('*').maybeSingle(),
  updateById: (client: SupabaseClient, id: string, payload: { link?: string | null; bio?: string | null }) =>
    client.from('orgs').update(payload).eq('id', id),
  deleteById: (client: SupabaseClient, id: string) => client.from('orgs').delete().eq('id', id),
};

export type OrgProfileInsert = {
  org_id: string;
  profile_id: string;
  admin?: boolean;
};

export const orgsProfilesDto = {
  insert: (client: SupabaseClient, payload: OrgProfileInsert) =>
    client.from('orgs_profiles').insert(payload).select('*').maybeSingle(),
  getByOrgIdAndProfileId: (client: SupabaseClient, orgId: string, profileId: string) =>
    client.from('orgs_profiles').select('*').eq('org_id', orgId).eq('profile_id', profileId).maybeSingle(),
  listByProfileId: (client: SupabaseClient, profileId: string) =>
    client.from('orgs_profiles').select('org_id,admin,created_at').eq('profile_id', profileId).order('created_at', {
      ascending: false,
    }),
};

export type SecretKeyInsert = {
  name: string;
  token_hash: string;
  profile_id: string;
};

export type SecretKeyScopeInsert = {
  secret_key_id: string;
  profile_id?: string | null;
  org_id?: string | null;
};

export const secretKeysDto = {
  insert: (client: SupabaseClient, payload: SecretKeyInsert) =>
    client.from('secret_keys').insert(payload).select('id').maybeSingle(),
  deleteById: (client: SupabaseClient, id: string) => client.from('secret_keys').delete().eq('id', id),
  listByProfileId: (client: SupabaseClient, profileId: string) =>
    client
      .from('secret_keys')
      .select('id,name,created_at,last_used_at')
      .eq('profile_id', profileId)
      .order('created_at', { ascending: false }),
};

export const secretKeyScopesDto = {
  insert: (client: SupabaseClient, payload: SecretKeyScopeInsert[]) => client.from('secret_key_scopes').insert(payload),
  listBySecretKeyIds: (client: SupabaseClient, secretKeyIds: string[]) =>
    client
      .from('secret_key_scopes')
      .select('secret_key_id,profile_id,org_id')
      .in('secret_key_id', secretKeyIds),
};

export type UsernameInsert = {
  name: string;
  user_id?: string | null;
  org_id?: string | null;
};

export const usernamesDto = {
  insert: (client: SupabaseClient, payload: UsernameInsert) => client.from('usernames').insert(payload),
  updateById: (client: SupabaseClient, id: string, payload: { name: string }) =>
    client.from('usernames').update(payload).eq('id', id),
  getByUserId: (client: SupabaseClient, userId: string) => client.from('usernames').select('*').eq('user_id', userId),
  listByUserIds: (client: SupabaseClient, userIds: string[]) =>
    client.from('usernames').select('name,user_id,org_id').in('user_id', userIds),
  listByOrgIds: (client: SupabaseClient, orgIds: string[]) =>
    client.from('usernames').select('name,user_id,org_id').in('org_id', orgIds),
  getByName: (client: SupabaseClient, username: string) =>
    client.from('usernames').select('*').ilike('name', username).maybeSingle(),
};

export type AddonInsert = {
  profile_id?: string | null;
  org_id?: string | null;
  name: string;
  repo: string;
  editor?: boolean;
};

export const addonsDto = {
  insert: (client: SupabaseClient, payload: AddonInsert) =>
    client.from('addons').insert(payload).select('*').maybeSingle(),
  deleteById: (client: SupabaseClient, id: string) => client.from('addons').delete().eq('id', id),
  listAll: async (client: SupabaseClient) => {
    return client
      .from('addons')
      .select('id,name,repo,editor,created_at,profile_id,org_id')
      .order('created_at', { ascending: false });
  },
  listByProfileId: (client: SupabaseClient, profileId: string) =>
    client.from('addons').select('*').eq('profile_id', profileId).order('created_at', { ascending: false }),
  listByOrgId: (client: SupabaseClient, orgId: string) =>
    client.from('addons').select('*').eq('org_id', orgId).order('created_at', { ascending: false }),
  getByProfileIdAndName: (client: SupabaseClient, profileId: string, name: string) =>
    client.from('addons').select('*').eq('profile_id', profileId).eq('name', name).maybeSingle(),
  getByOrgIdAndName: (client: SupabaseClient, orgId: string, name: string) =>
    client.from('addons').select('*').eq('org_id', orgId).eq('name', name).maybeSingle(),
};

export const releasesDto = {
  insert: (client: SupabaseClient, payload: { addon_id: string; major: number; minor: number; patch: number; tag: string; asset: string }) =>
    client.from('releases').insert(payload).select('*').maybeSingle(),
  deleteByVersion: (client: SupabaseClient, addonId: string, major: number, minor: number, patch: number) =>
    client
      .from('releases')
      .delete()
      .eq('addon_id', addonId)
      .eq('major', major)
      .eq('minor', minor)
      .eq('patch', patch),
  listByAddonIds: (client: SupabaseClient, addonIds: string[]) =>
    client
      .from('releases')
      .select('*')
      .in('addon_id', addonIds)
      .order('created_at', { ascending: false }),
  listByAddonId: (client: SupabaseClient, addonId: string) =>
    client.from('releases').select('*').eq('addon_id', addonId).order('created_at', { ascending: false }),
};
