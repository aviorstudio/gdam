const apiBaseUrl = 'https://api.github.com';

const githubHeaders = () => {
  const headers: Record<string, string> = {
    Accept: 'application/vnd.github+json',
    'User-Agent': 'gdam-web',
  };
  const token = String(import.meta.env.GITHUB_TOKEN ?? '').trim();
  if (token) headers.Authorization = token.toLowerCase().startsWith('bearer ') ? token : `Bearer ${token}`;
  return headers;
};

export const releaseTagCandidates = (version: string, explicitTag: string) => {
  const tag = explicitTag.trim();
  if (tag) return [tag];

  const normalizedVersion = version.trim().replace(/^[vV]/, '');
  if (!normalizedVersion) return [];
  return [`v${normalizedVersion}`, normalizedVersion];
};

export const findGitHubReleaseTag = async (owner: string, repo: string, tags: string[]) => {
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
