package gdamdb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type PublishReleaseInput struct {
	SecretKey  string
	Owner      string
	Addon      string
	Major      int
	Minor      int
	Patch      int
	ReleaseTag string
	AssetName  string
}

func NewDefaultClient() *Client {
	return NewClient(defaultSupabaseURL(), defaultSupabasePublishableKey())
}

func defaultSupabaseURL() string {
	if value := strings.TrimSpace(os.Getenv("GDAM_SUPABASE_URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("SUPABASE_URL")); value != "" {
		return value
	}
	return DefaultSupabaseURL
}

func defaultSupabasePublishableKey() string {
	if value := strings.TrimSpace(os.Getenv("GDAM_SUPABASE_PUBLISHABLE_KEY")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("SUPABASE_PUBLISHABLE_KEY")); value != "" {
		return value
	}
	return strings.TrimSpace(DefaultSupabasePublishableKey)
}

func NewClient(baseURL, apiKey string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type ResolvedAddon struct {
	Name string
	Repo string

	GitHubOwner string
	GitHubRepo  string

	Version    string
	ReleaseTag string
	AssetName  string

	EditorPlugin bool
}

func (c *Client) ResolveAddon(ctx context.Context, username, addon, requestedVersion string) (ResolvedAddon, error) {
	usernameNormal := strings.ToLower(strings.TrimSpace(username))
	addonName := strings.TrimSpace(addon)
	if usernameNormal == "" || addonName == "" {
		return ResolvedAddon{}, fmt.Errorf("invalid addon spec")
	}

	userRow, ok, err := c.getUsernameByNormal(ctx, usernameNormal)
	if err != nil {
		return ResolvedAddon{}, err
	}
	if !ok {
		return ResolvedAddon{}, fmt.Errorf("owner not found: @%s", usernameNormal)
	}
	if userRow.UserID != nil && userRow.OrgID != nil {
		return ResolvedAddon{}, fmt.Errorf("username is assigned to multiple owners: @%s", usernameNormal)
	}
	if userRow.UserID == nil && userRow.OrgID == nil {
		return ResolvedAddon{}, fmt.Errorf("owner not found: @%s", usernameNormal)
	}

	addonRow, ok, err := c.getAddonByOwnerAndName(ctx, userRow.UserID, userRow.OrgID, addonName)
	if err != nil {
		return ResolvedAddon{}, err
	}
	if !ok {
		return ResolvedAddon{}, fmt.Errorf("addon not found: @%s/%s", usernameNormal, addonName)
	}
	if strings.TrimSpace(addonRow.Repo) == "" {
		return ResolvedAddon{}, fmt.Errorf("addon has no repository set: @%s/%s", usernameNormal, addonName)
	}

	versionRows, err := c.listReleases(ctx, addonRow.ID)
	if err != nil {
		return ResolvedAddon{}, err
	}
	selected, ok := selectVersion(versionRows, requestedVersion)
	if !ok {
		return ResolvedAddon{}, fmt.Errorf("version not found: %s", requestedVersion)
	}
	releaseTag := strings.TrimSpace(selected.ReleaseTag)
	if releaseTag == "" {
		return ResolvedAddon{}, fmt.Errorf(
			"selected version has no release tag: %d.%d.%d",
			selected.Major,
			selected.Minor,
			selected.Patch,
		)
	}
	assetName := strings.TrimSpace(selected.AssetName)
	if assetName == "" {
		return ResolvedAddon{}, fmt.Errorf(
			"selected version has no release asset name: %d.%d.%d",
			selected.Major,
			selected.Minor,
			selected.Patch,
		)
	}

	ghOwner, ghRepo, _, err := ParseGitHubRepoURL(addonRow.Repo)
	if err != nil {
		return ResolvedAddon{}, err
	}

	return ResolvedAddon{
		Name:         "@" + usernameNormal + "/" + addonName,
		Repo:         addonRow.Repo,
		GitHubOwner:  ghOwner,
		GitHubRepo:   ghRepo,
		Version:      fmt.Sprintf("%d.%d.%d", selected.Major, selected.Minor, selected.Patch),
		ReleaseTag:   releaseTag,
		AssetName:    assetName,
		EditorPlugin: addonRow.EditorPlugin != nil && *addonRow.EditorPlugin,
	}, nil
}

type usernameRow struct {
	UsernameDisplay *string `json:"name"`
	UserID          *string `json:"user_id"`
	OrgID           *string `json:"org_id"`
}

type addonRow struct {
	ID           string  `json:"id"`
	Name         *string `json:"name"`
	Repo         string  `json:"repo"`
	EditorPlugin *bool   `json:"editor"`
	CreatedAt    *string `json:"created_at"`
	ProfileID    *string `json:"profile_id"`
	OrgID        *string `json:"org_id"`
}

type releaseRow struct {
	AddonID    *string `json:"addon_id"`
	Major      int     `json:"major"`
	Minor      int     `json:"minor"`
	Patch      int     `json:"patch"`
	ReleaseTag string  `json:"tag"`
	AssetName  string  `json:"asset"`
	CreatedAt  *string `json:"created_at"`
}

func (c *Client) getUsernameByNormal(ctx context.Context, usernameNormal string) (usernameRow, bool, error) {
	q := url.Values{}
	q.Set("select", "name,user_id,org_id")
	q.Set("name", "ilike."+usernameNormal)
	q.Set("limit", "2")

	var rows []usernameRow
	if err := c.get(ctx, "usernames", q, &rows); err != nil {
		return usernameRow{}, false, err
	}
	if len(rows) == 0 {
		return usernameRow{}, false, nil
	}
	if len(rows) > 1 {
		return usernameRow{}, false, fmt.Errorf("username is not unique: %s", usernameNormal)
	}
	return rows[0], true, nil
}

func (c *Client) getAddonByOwnerAndName(ctx context.Context, profileID, orgID *string, addonName string) (addonRow, bool, error) {
	q := url.Values{}
	q.Set("select", "id,name,repo,editor,created_at,profile_id,org_id")
	q.Set("name", "eq."+addonName)
	q.Set("limit", "2")

	if orgID != nil && strings.TrimSpace(*orgID) != "" {
		q.Set("org_id", "eq."+strings.TrimSpace(*orgID))
	} else if profileID != nil && strings.TrimSpace(*profileID) != "" {
		q.Set("profile_id", "eq."+strings.TrimSpace(*profileID))
	} else {
		return addonRow{}, false, fmt.Errorf("owner has no id")
	}

	var rows []addonRow
	if err := c.get(ctx, "addons", q, &rows); err != nil {
		return addonRow{}, false, err
	}
	if len(rows) == 0 {
		return addonRow{}, false, nil
	}
	if len(rows) > 1 {
		return addonRow{}, false, fmt.Errorf("addon is not unique: %s", addonName)
	}
	return rows[0], true, nil
}

func (c *Client) listReleases(ctx context.Context, addonID string) ([]releaseRow, error) {
	addonID = strings.TrimSpace(addonID)
	if addonID == "" {
		return nil, fmt.Errorf("missing addon id")
	}

	q := url.Values{}
	q.Set("select", "addon_id,major,minor,patch,tag,asset,created_at")
	q.Set("addon_id", "eq."+addonID)
	q.Set("order", "major.desc,minor.desc,patch.desc,created_at.desc")
	q.Set("limit", "100")

	var rows []releaseRow
	if err := c.get(ctx, "releases", q, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []releaseRow{}
	}
	return rows, nil
}

func (c *Client) PublishRelease(ctx context.Context, input PublishReleaseInput) error {
	payload := map[string]any{
		"secret_key":    strings.TrimSpace(input.SecretKey),
		"owner_name":    strings.TrimSpace(input.Owner),
		"addon_name":    strings.TrimSpace(input.Addon),
		"version_major": input.Major,
		"version_minor": input.Minor,
		"version_patch": input.Patch,
		"release_tag":   strings.TrimSpace(input.ReleaseTag),
		"asset_name":    strings.TrimSpace(input.AssetName),
	}
	return c.postRPC(ctx, "publish_release_with_secret_key", payload)
}

func (c *Client) postRPC(ctx context.Context, fn string, payload any) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, "rest/v1/rpc", fn)
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	if c.apiKey == "" {
		return fmt.Errorf("missing Supabase publishable key (set GDAM_SUPABASE_PUBLISHABLE_KEY or SUPABASE_PUBLISHABLE_KEY)")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		return fmt.Errorf("gdam db failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

func (c *Client) get(ctx context.Context, table string, query url.Values, dst any) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return err
	}
	u.Path = path.Join(u.Path, "rest/v1", table)
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	if c.apiKey == "" {
		return fmt.Errorf("missing Supabase publishable key (set GDAM_SUPABASE_PUBLISHABLE_KEY or SUPABASE_PUBLISHABLE_KEY)")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
		return fmt.Errorf("gdam db failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}
