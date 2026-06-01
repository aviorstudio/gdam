package gdamdb

import (
	"fmt"
	"net/url"
	"strings"
)

func GitHubTreeURL(owner, repo, ref string) string {
	return GitHubTreeURLWithPath(owner, repo, ref, "")
}

func ParseGitHubTreeURL(treeURL string) (string, string, string, error) {
	owner, repo, ref, _, err := ParseGitHubTreeURLWithPath(treeURL)
	return owner, repo, ref, err
}

func GitHubTreeURLWithPath(owner, repo, ref, repoPath string) string {
	base := "https://github.com/" + owner + "/" + repo + "/tree/" + url.PathEscape(ref)
	repoPath = strings.Trim(strings.TrimSpace(repoPath), "/")
	if repoPath == "" {
		return base
	}

	escapedParts := make([]string, 0, strings.Count(repoPath, "/")+1)
	for _, part := range strings.Split(repoPath, "/") {
		if part == "" {
			continue
		}
		escapedParts = append(escapedParts, url.PathEscape(part))
	}
	if len(escapedParts) == 0 {
		return base
	}
	return base + "/" + strings.Join(escapedParts, "/")
}

func ParseGitHubTreeURLWithPath(treeURL string) (string, string, string, string, error) {
	treeURL = strings.TrimSpace(treeURL)
	if treeURL == "" {
		return "", "", "", "", fmt.Errorf("empty tree url")
	}

	u, err := url.Parse(treeURL)
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid tree url: %w", err)
	}

	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if !strings.HasSuffix(host, "github.com") {
		return "", "", "", "", fmt.Errorf("only github.com tree urls are supported (got %s)", host)
	}

	p := strings.Trim(u.EscapedPath(), "/")
	parts := strings.Split(p, "/")
	if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] != "tree" {
		return "", "", "", "", fmt.Errorf("invalid github tree url (expected github.com/owner/repo/tree/ref): %s", treeURL)
	}

	ref, err := url.PathUnescape(parts[3])
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid tree ref: %w", err)
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", "", "", fmt.Errorf("empty tree ref: %s", treeURL)
	}

	var repoPathParts []string
	for _, part := range parts[4:] {
		if strings.TrimSpace(part) == "" {
			continue
		}
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			return "", "", "", "", fmt.Errorf("invalid tree path: %w", err)
		}
		unescaped = strings.TrimSpace(unescaped)
		if unescaped == "" {
			continue
		}
		repoPathParts = append(repoPathParts, unescaped)
	}
	repoPath := strings.Trim(strings.Join(repoPathParts, "/"), "/")
	return parts[0], parts[1], ref, repoPath, nil
}
