const apiBaseUrl = 'https://api.github.com';

const allowLocalReleaseFixtures = () => {
  const flag = String(import.meta.env.GDAM_ALLOW_LOCAL_RELEASE_FIXTURES ?? '').trim().toLowerCase();
  const supabaseUrl = String(import.meta.env.SUPABASE_URL ?? '').trim();
  return flag === 'true' && /^https?:\/\/(127\.0\.0\.1|localhost)(:|\/|$)/i.test(supabaseUrl);
};

const githubHeaders = () => {
  const headers: Record<string, string> = {
    Accept: 'application/vnd.github+json',
    'User-Agent': 'gdam-web',
  };
  const token = String(import.meta.env.GITHUB_TOKEN ?? '').trim();
  if (token) headers.Authorization = token.toLowerCase().startsWith('bearer ') ? token : `Bearer ${token}`;
  return headers;
};

export const findGitHubReleaseTag = async (owner: string, repo: string, tags: string[]) => {
  if (allowLocalReleaseFixtures()) {
    const tag = tags.map((candidate) => candidate.trim()).find(Boolean) ?? '';
    if (tag) return { tag };
  }

  for (const tag of tags) {
    const trimmedTag = tag.trim();
    if (!trimmedTag) continue;

    const url = `${apiBaseUrl}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/releases/tags/${encodeURIComponent(trimmedTag)}`;
    const response = await fetch(url, { headers: githubHeaders() });
    if (response.status === 404) continue;
    if (!response.ok) {
      const body = await response.text();
      return { tag: '', error: `GitHub release lookup failed (${response.status}): ${body.trim()}` };
    }

    const payload = await response.json();
    const releaseTag = String(payload?.tag_name ?? '').trim();
    if (releaseTag) return { tag: releaseTag };
  }

  return { tag: '', error: `No GitHub Release found for ${tags.join(' or ')}` };
};

export const releaseHasAsset = async (owner: string, repo: string, tag: string, assetName: string) => {
  const releaseTag = tag.trim();
  const expectedAsset = assetName.trim();
  if (!releaseTag || !expectedAsset) return { ok: false, error: 'Release tag and asset name are required.' };
  if (allowLocalReleaseFixtures()) return { ok: true };

  const url = `${apiBaseUrl}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/releases/tags/${encodeURIComponent(releaseTag)}`;
  const response = await fetch(url, { headers: githubHeaders() });
  if (!response.ok) {
    const body = await response.text();
    return { ok: false, error: `GitHub release lookup failed (${response.status}): ${body.trim()}` };
  }

  const payload = await response.json();
  const assets = Array.isArray(payload?.assets) ? payload.assets : [];
  const found = assets.some((asset) => String(asset?.name ?? '').trim() === expectedAsset);
  if (!found) return { ok: false, error: `GitHub Release ${releaseTag} is missing asset ${expectedAsset}.` };
  return { ok: true };
};
