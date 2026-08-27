package gdamdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the GDAM API.
//
// It used to query PostgREST directly, which meant the CLI carried a database
// key and reimplemented version selection. Both now live server-side: this is a
// plain HTTP client against two public endpoints, and it ships no credentials
// of any kind. Publishing authenticates with the user's own secret key.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

type PublishReleaseInput struct {
	SecretKey string
	Owner     string
	Addon     string
	TagName   string
	AssetName string
}

func NewDefaultClient() *Client {
	return NewClient(defaultAPIURL())
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ResolvedAddon is everything needed to install an addon. The API decides which
// release this is, so every client resolves identically.
type ResolvedAddon struct {
	Name string `json:"name"`
	Repo string `json:"repo"`

	GitHubOwner string `json:"github_owner"`
	GitHubRepo  string `json:"github_repo"`

	TagName         string    `json:"tag_name"`
	GitHubReleaseID int64     `json:"github_release_id"`
	CommitSHA       string    `json:"commit_sha"`
	AssetID         int64     `json:"asset_id"`
	AssetName       string    `json:"asset_name"`
	AssetDigest     string    `json:"asset_digest"`
	PublishedAt     time.Time `json:"published_at"`
	Prerelease      bool      `json:"prerelease"`

	EditorPlugin bool `json:"editor_plugin"`
}

func (c *Client) ResolveAddon(ctx context.Context, username, addon, requestedTag string) (ResolvedAddon, error) {
	owner := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(username), "@")))
	addonName := strings.TrimSpace(addon)
	if owner == "" || addonName == "" {
		return ResolvedAddon{}, fmt.Errorf("invalid addon spec")
	}

	path := "/api/v1/resolve/" + url.PathEscape(owner) + "/" + url.PathEscape(addonName)
	if tag := strings.TrimSpace(requestedTag); tag != "" {
		path += "?" + url.Values{"tag": {tag}}.Encode()
	}

	var resolved ResolvedAddon
	if err := c.do(ctx, http.MethodGet, path, nil, &resolved); err != nil {
		return ResolvedAddon{}, err
	}
	if err := resolved.validate(); err != nil {
		return ResolvedAddon{}, fmt.Errorf("invalid registry response: %w", err)
	}
	return resolved, nil
}

func (r ResolvedAddon) validate() error {
	if r.Name == "" || r.GitHubOwner == "" || r.GitHubRepo == "" || r.TagName == "" || r.CommitSHA == "" || r.AssetName == "" || r.AssetDigest == "" || r.GitHubReleaseID <= 0 || r.AssetID <= 0 || r.PublishedAt.IsZero() {
		return fmt.Errorf("missing verified release identity fields")
	}
	return nil
}

func (c *Client) PublishRelease(ctx context.Context, input PublishReleaseInput) error {
	payload := map[string]any{
		"secret_key": strings.TrimSpace(input.SecretKey),
		"owner":      strings.TrimSpace(input.Owner),
		"addon":      strings.TrimSpace(input.Addon),
		"tag_name":   strings.TrimSpace(input.TagName),
		"asset_name": strings.TrimSpace(input.AssetName),
	}
	return c.do(ctx, http.MethodPost, "/api/v1/publish", payload, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if c.baseURL == "" {
		return fmt.Errorf("missing GDAM API url (set GDAM_API_URL)")
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s", apiErrorMessage(resp))
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// apiErrorMessage prefers the API's own message, which is written to be shown
// to a user, and falls back to the raw body.
func apiErrorMessage(resp *http.Response) string {
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))

	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(payload, &parsed); err == nil && strings.TrimSpace(parsed.Message) != "" {
		return parsed.Message
	}

	text := strings.TrimSpace(string(payload))
	if text == "" {
		return fmt.Sprintf("gdam api failed (%d)", resp.StatusCode)
	}
	return fmt.Sprintf("gdam api failed (%d): %s", resp.StatusCode, text)
}
