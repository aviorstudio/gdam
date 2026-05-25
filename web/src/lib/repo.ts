export const toRepoUrl = (value: string) => {
  const repo = value.trim();
  if (!repo) return '';

  if (/^https?:\/\//i.test(repo)) {
    try {
      const url = new URL(repo);
      const normalizedPath = url.pathname.replace(/\/+$/, '').replace(/\.git$/i, '');
      return `${url.origin}${normalizedPath}`;
    } catch {
      return repo;
    }
  }

  const gitSshMatch = repo.match(/^git@([^:]+):(.+)$/i);
  if (gitSshMatch) {
    const host = gitSshMatch[1];
    const path = gitSshMatch[2].replace(/\.git$/i, '').replace(/^\/+/, '').replace(/\/+$/, '');
    return path ? `https://${host}/${path}` : `https://${host}`;
  }

  const sanitized = repo.replace(/^\/+/, '').replace(/\/+$/, '').replace(/\.git$/i, '');
  return sanitized ? `https://${sanitized}` : '';
};

export const toGitHubRepoRootUrl = (repoUrl: string) => {
  const normalized = toRepoUrl(repoUrl);
  if (!normalized) return '';

  let url: URL;
  try {
    url = new URL(normalized);
  } catch {
    return '';
  }

  const hostname = url.hostname.toLowerCase();
  if (!hostname.endsWith('github.com')) return '';

  const parts = url.pathname.replace(/\/+$/, '').replace(/\.git$/i, '').split('/').filter(Boolean);
  if (parts.length < 2) return '';

  return `${url.origin}/${parts[0]}/${parts[1]}`;
};

export const parseGitHubOwnerRepo = (repoUrl: string) => {
  const root = toGitHubRepoRootUrl(repoUrl);
  if (!root) return null;

  let url: URL;
  try {
    url = new URL(root);
  } catch {
    return null;
  }

  const parts = url.pathname.replace(/\/+$/, '').replace(/\.git$/i, '').split('/').filter(Boolean);
  if (parts.length < 2) return null;
  return { owner: parts[0], repo: parts[1] };
};

export const toGitHubTreeUrl = (repoUrl: string, ref: string, repoSubdir?: string) => {
  const repo = toGitHubRepoRootUrl(repoUrl);
  const treeRef = ref.trim();
  if (!repo || !treeRef) return '';
  const base = `${repo}/tree/${encodeURIComponent(treeRef)}`;

  const subdirRaw = String(repoSubdir ?? '').trim();
  if (!subdirRaw) return base;

  const normalized = subdirRaw.replace(/\\/g, '/').replace(/^\/+/, '').replace(/\/+$/, '').replace(/\/{2,}/g, '/');
  if (!normalized || normalized === '.') return base;

  const parts = normalized.split('/').filter(Boolean);
  if (parts.some((part) => part === '.' || part === '..')) return base;

  return `${base}/${parts.map(encodeURIComponent).join('/')}`;
};
