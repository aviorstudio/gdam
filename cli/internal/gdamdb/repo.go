package gdamdb

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var gitSSHRe = regexp.MustCompile(`^git@([^:]+):(.+)$`)

func NormalizeRepoURL(value string) string {
	repo := strings.TrimSpace(value)
	if repo == "" {
		return ""
	}

	if strings.HasPrefix(strings.ToLower(repo), "http://") || strings.HasPrefix(strings.ToLower(repo), "https://") {
		u, err := url.Parse(repo)
		if err != nil {
			return repo
		}
		normalizedPath := strings.TrimRight(u.Path, "/")
		normalizedPath = strings.TrimSuffix(normalizedPath, ".git")
		u.Path = normalizedPath
		u.RawQuery = ""
		u.Fragment = ""
		return strings.TrimRight(u.String(), "/")
	}

	if m := gitSSHRe.FindStringSubmatch(repo); m != nil {
		host := strings.TrimSpace(m[1])
		pathPart := strings.TrimSpace(m[2])
		pathPart = strings.TrimSuffix(pathPart, ".git")
		pathPart = strings.Trim(pathPart, "/")
		if pathPart == "" {
			return "https://" + host
		}
		return "https://" + host + "/" + pathPart
	}

	sanitized := strings.Trim(repo, "/")
	sanitized = strings.TrimSuffix(sanitized, ".git")
	if sanitized == "" {
		return ""
	}
	return "https://" + sanitized
}

func ParseGitHubOwnerRepo(repo string) (string, string, error) {
	owner, repoName, subdir, err := ParseGitHubRepoURL(repo)
	if err != nil {
		return "", "", err
	}
	if subdir != "" {
		return "", "", fmt.Errorf("invalid github repo path (expected github.com/owner/repo): %s", NormalizeRepoURL(repo))
	}
	return owner, repoName, nil
}

func ParseGitHubRepoURL(repo string) (string, string, string, error) {
	repoURL := NormalizeRepoURL(repo)
	if repoURL == "" {
		return "", "", "", fmt.Errorf("empty repo url")
	}

	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid repo url: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if !strings.HasSuffix(host, "github.com") {
		return "", "", "", fmt.Errorf("only github.com repos are supported (got %s)", host)
	}

	p := strings.Trim(u.Path, "/")
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("invalid github repo path (expected github.com/owner/repo): %s", repoURL)
	}
	owner := parts[0]
	repoName := parts[1]

	rest := parts[2:]
	if len(rest) >= 2 && (rest[0] == "tree" || rest[0] == "blob") {
		rest = rest[2:]
	}

	subdir := strings.Join(rest, "/")
	subdir = strings.Trim(subdir, "/")
	if subdir == "" {
		return owner, repoName, "", nil
	}
	if strings.Contains(subdir, "\\") {
		return "", "", "", fmt.Errorf("invalid github repo path (unexpected \\\\): %s", repoURL)
	}

	for _, part := range strings.Split(subdir, "/") {
		switch part {
		case "", ".", "..":
			return "", "", "", fmt.Errorf("invalid github repo path: %s", repoURL)
		}
	}

	return owner, repoName, subdir, nil
}
