import { createClient } from '@supabase/supabase-js';

export const supabase = createClient(import.meta.env.SUPABASE_URL, import.meta.env.SUPABASE_PUBLISHABLE_KEY);

export const createSupabaseForAccessToken = (accessToken: string) => createClient(
  import.meta.env.SUPABASE_URL,
  import.meta.env.SUPABASE_PUBLISHABLE_KEY,
  {
    global: {
      headers: {
        Authorization: `Bearer ${accessToken}`,
      },
    },
  }
);
