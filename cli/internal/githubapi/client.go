package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aviorstudio/gdam/cli/internal/semver"
)

const apiBaseURL = "https://api.github.com"

type Client struct {
	httpClient *http.Client
	token      string
	userAgent  string
}

func NewClient(token string) *Client {
	token = strings.TrimSpace(token)
	if token != "" && !strings.HasPrefix(strings.ToLower(token), "bearer ") && !strings.HasPrefix(strings.ToLower(token), "token ") {
		token = "Bearer " + token
	}

	return &Client{
		httpClient: &http.Client{Timeout: 60 * time.Second},
		token:      token,
		userAgent:  "gdam-cli",
	}
}

func (c *Client) ResolveRefAndSHA(ctx context.Context, owner, repo, version string) (string, string, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		ref, err := c.latestVersionRef(ctx, owner, repo)
		if err == nil {
			sha, err := c.resolveCommitSHA(ctx, owner, repo, ref)
			if err != nil {
				return "", "", err
			}
			return ref, sha, nil
		}

		branch, err2 := c.defaultBranch(ctx, owner, repo)
		if err2 != nil {
			return "", "", err
		}
		sha, err2 := c.resolveCommitSHA(ctx, owner, repo, branch)
		if err2 != nil {
			return "", "", err2
		}
		return branch, sha, nil
	}

	sha, err := c.resolveCommitSHA(ctx, owner, repo, version)
	if err == nil {
		return version, sha, nil
	}

	if !strings.HasPrefix(version, "v") {
		sha2, err2 := c.resolveCommitSHA(ctx, owner, repo, "v"+version)
		if err2 == nil {
			return "v" + version, sha2, nil
		}
	}
	return "", "", err
}

func (c *Client) DownloadZipball(ctx context.Context, owner, repo, sha, destPath string) error {
	u := apiBaseURL + "/repos/" + path.Join(owner, repo) + "/zipball/" + url.PathEscape(sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	c.addHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return fmt.Errorf("github zipball failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func (c *Client) latestVersionRef(ctx context.Context, owner, repo string) (string, error) {
	tag, err := c.latestReleaseTag(ctx, owner, repo)
	if err == nil && tag != "" {
		return tag, nil
	}

	tags, err := c.listTags(ctx, owner, repo)
	if err != nil {
		return "", err
	}

	best, ok := semver.BestTag(tags)
	if !ok {
		return "", errors.New("no semver tags found")
	}
	return best, nil
}

func (c *Client) latestReleaseTag(ctx context.Context, owner, repo string) (string, error) {
	u := apiBaseURL + "/repos/" + path.Join(owner, repo) + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	c.addHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", errors.New("no releases")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return "", fmt.Errorf("github releases/latest failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var out struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	out.TagName = strings.TrimSpace(out.TagName)
	if out.TagName == "" {
		return "", errors.New("empty latest release tag")
	}
	return out.TagName, nil
}

func (c *Client) listTags(ctx context.Context, owner, repo string) ([]string, error) {
	u := apiBaseURL + "/repos/" + path.Join(owner, repo) + "/tags?per_page=100"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.addHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("github tags failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var out []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	var tags []string
	for _, t := range out {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		tags = append(tags, t.Name)
	}
	return tags, nil
}

func (c *Client) defaultBranch(ctx context.Context, owner, repo string) (string, error) {
	u := apiBaseURL + "/repos/" + path.Join(owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	c.addHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return "", fmt.Errorf("github repo failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var out struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	out.DefaultBranch = strings.TrimSpace(out.DefaultBranch)
	if out.DefaultBranch == "" {
		return "", errors.New("empty default_branch")
	}
	return out.DefaultBranch, nil
}

func (c *Client) resolveCommitSHA(ctx context.Context, owner, repo, ref string) (string, error) {
	u := apiBaseURL + "/repos/" + path.Join(owner, repo) + "/commits/" + ref
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	c.addHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return "", fmt.Errorf("github commit lookup failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var out struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	out.SHA = strings.TrimSpace(out.SHA)
	if out.SHA == "" {
		return "", errors.New("empty sha")
	}
	return out.SHA, nil
}

func (c *Client) addHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
}
