package githubapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
)

const (
	apiBaseURL       = "https://api.github.com"
	DownloadTimeout  = 2 * time.Minute
	MaxRedirects     = 5
	MaxDownloadBytes = int64(128 << 20)
	maxMetadataBytes = int64(4 << 20)
)

var ErrReleaseAssetNotFound = errors.New("release asset not found")

type ReleaseIdentity struct {
	ReleaseID   int64
	TagName     string
	CommitSHA   string
	AssetID     int64
	AssetName   string
	Digest      string
	PublishedAt time.Time
	Prerelease  bool
}

type Client struct {
	httpClient *http.Client
	token      string
	userAgent  string
	apiBaseURL string
}

func NewClient(token string) *Client {
	token = strings.TrimSpace(token)
	if token != "" && !strings.HasPrefix(strings.ToLower(token), "bearer ") && !strings.HasPrefix(strings.ToLower(token), "token ") {
		token = "Bearer " + token
	}
	c := &Client{token: token, userAgent: "gdam", apiBaseURL: apiBaseURL}
	c.httpClient = &http.Client{
		Timeout: DownloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirects {
				return fmt.Errorf("github download exceeded %d redirects", MaxRedirects)
			}
			// Release downloads redirect to GitHub's object store. Never forward
			// registry/GitHub authorization to another origin.
			if len(via) > 0 && !sameOrigin(req.URL, via[0].URL) {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
	return c
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func (c *Client) DownloadVerifiedReleaseAsset(ctx context.Context, owner, repo string, identity ReleaseIdentity, destPath string) error {
	if identity.ReleaseID <= 0 || identity.AssetID <= 0 || strings.TrimSpace(identity.TagName) == "" || strings.TrimSpace(identity.CommitSHA) == "" || strings.TrimSpace(identity.AssetName) == "" || strings.TrimSpace(identity.Digest) == "" || identity.PublishedAt.IsZero() {
		return fmt.Errorf("incomplete verified release identity")
	}
	if err := c.verifyRelease(ctx, owner, repo, identity); err != nil {
		return err
	}

	u := c.apiBaseURL + "/repos/" + path.Join(owner, repo) + "/releases/assets/" + fmt.Sprint(identity.AssetID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	c.addHeaders(req)
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrReleaseAssetNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return responseError("github release asset download", resp)
	}
	if resp.ContentLength > MaxDownloadBytes {
		return fmt.Errorf("release asset exceeds %d-byte download limit", MaxDownloadBytes)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(destPath)
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, MaxDownloadBytes+1))
	if err != nil && !(resp.ContentLength >= 0 && n != resp.ContentLength) {
		return err
	}
	if n > MaxDownloadBytes {
		return fmt.Errorf("release asset exceeds %d-byte download limit", MaxDownloadBytes)
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return fmt.Errorf("release asset truncated: received %d of %d bytes", n, resp.ContentLength)
	}
	if err := f.Close(); err != nil {
		return err
	}
	want, err := parseSHA256(identity.Digest)
	if err != nil {
		return err
	}
	got := h.Sum(nil)
	if !equalBytes(got, want) {
		return fmt.Errorf("release asset digest mismatch: expected %s, got sha256:%s", identity.Digest, hex.EncodeToString(got))
	}
	ok = true
	return nil
}

func (c *Client) verifyRelease(ctx context.Context, owner, repo string, want ReleaseIdentity) error {
	var release struct {
		ID          int64     `json:"id"`
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at"`
		Prerelease  bool      `json:"prerelease"`
		Draft       bool      `json:"draft"`
	}
	if err := c.getJSON(ctx, c.apiBaseURL+"/repos/"+path.Join(owner, repo)+"/releases/"+fmt.Sprint(want.ReleaseID), &release); err != nil {
		return err
	}
	if release.Draft || release.ID != want.ReleaseID || release.TagName != want.TagName || !release.PublishedAt.Equal(want.PublishedAt) || release.Prerelease != want.Prerelease {
		return fmt.Errorf("registered release identity drift detected")
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := c.getJSON(ctx, c.apiBaseURL+"/repos/"+path.Join(owner, repo)+"/commits/"+url.PathEscape(want.TagName), &commit); err != nil {
		return err
	}
	if !strings.EqualFold(commit.SHA, want.CommitSHA) {
		return fmt.Errorf("registered release commit drift detected")
	}
	var asset struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Digest string `json:"digest"`
		State  string `json:"state"`
	}
	if err := c.getJSON(ctx, c.apiBaseURL+"/repos/"+path.Join(owner, repo)+"/releases/assets/"+fmt.Sprint(want.AssetID), &asset); err != nil {
		return err
	}
	if asset.ID != want.AssetID || asset.Name != want.AssetName || asset.Digest != want.Digest || asset.State != "uploaded" {
		return fmt.Errorf("registered release asset drift detected")
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, u string, out any) error {
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
	if resp.StatusCode == http.StatusNotFound {
		return ErrReleaseAssetNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return responseError("github metadata request", resp)
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxMetadataBytes+1))
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func responseError(prefix string, resp *http.Response) error {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	return fmt.Errorf("%s failed (%d): %s", prefix, resp.StatusCode, strings.TrimSpace(string(msg)))
}

func parseSHA256(digest string) ([]byte, error) {
	algorithm, value, ok := strings.Cut(strings.TrimSpace(digest), ":")
	if !ok || !strings.EqualFold(algorithm, "sha256") {
		return nil, fmt.Errorf("unsupported release asset digest %q (expected sha256)", digest)
	}
	b, err := hex.DecodeString(value)
	if err != nil || len(b) != sha256.Size {
		return nil, fmt.Errorf("invalid sha256 release asset digest")
	}
	return b, nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func (c *Client) addHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", c.token)
	}
}
