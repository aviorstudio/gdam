package gdamdb

import (
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
	anonKey    string
	httpClient *http.Client
}

func NewDefaultClient() *Client {
	return NewClient(defaultSupabaseURL(), defaultSupabaseAnonKey())
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

func defaultSupabaseAnonKey() string {
	if value := strings.TrimSpace(os.Getenv("GDAM_SUPABASE_ANON_KEY")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("SUPABASE_ANON_KEY")); value != "" {
		return value
	}
	return DefaultSupabaseAnonKey
}

func NewClient(baseURL, anonKey string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	anonKey = strings.TrimSpace(anonKey)
	return &Client{
		baseURL: baseURL,
		anonKey: anonKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type ResolvedPlugin struct {
	Name string
	Repo string

	GitHubOwner  string
	GitHubRepo   string
	GitHubSubdir string

	Version string
	SHA     string
}

func (c *Client) ResolvePlugin(ctx context.Context, username, plugin, requestedVersion string) (ResolvedPlugin, error) {
	usernameNormal := strings.ToLower(strings.TrimSpace(username))
	pluginName := strings.TrimSpace(plugin)
	if usernameNormal == "" || pluginName == "" {
		return ResolvedPlugin{}, fmt.Errorf("invalid addon spec")
	}

	userRow, ok, err := c.getUsernameByNormal(ctx, usernameNormal)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	if !ok {
		return ResolvedPlugin{}, fmt.Errorf("owner not found: @%s", usernameNormal)
	}
	if userRow.UserID != nil && userRow.OrgID != nil {
		return ResolvedPlugin{}, fmt.Errorf("username is assigned to multiple owners: @%s", usernameNormal)
	}
	if userRow.UserID == nil && userRow.OrgID == nil {
		return ResolvedPlugin{}, fmt.Errorf("owner not found: @%s", usernameNormal)
	}

	pluginRow, ok, err := c.getPluginByOwnerAndName(ctx, userRow.UserID, userRow.OrgID, pluginName)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	if !ok {
		return ResolvedPlugin{}, fmt.Errorf("addon not found: @%s/%s", usernameNormal, pluginName)
	}
	if strings.TrimSpace(pluginRow.Repo) == "" {
		return ResolvedPlugin{}, fmt.Errorf("addon has no repository set: @%s/%s", usernameNormal, pluginName)
	}

	versionRows, err := c.listPluginVersions(ctx, pluginRow.ID)
	if err != nil {
		return ResolvedPlugin{}, err
	}
	selected, ok := selectVersion(versionRows, requestedVersion)
	if !ok {
		return ResolvedPlugin{}, fmt.Errorf("version not found: %s", requestedVersion)
	}
	sha := strings.TrimSpace(selected.SHA)
	if sha == "" {
		return ResolvedPlugin{}, fmt.Errorf(
			"selected version has no sha: %d.%d.%d",
			selected.Major,
			selected.Minor,
			selected.Patch,
		)
	}

	ghOwner, ghRepo, repoSubdir, err := ParseGitHubRepoURL(pluginRow.Repo)
	if err != nil {
		return ResolvedPlugin{}, err
	}

	ghSubdir := repoSubdir
	if pluginRow.Path != nil {
		repoPath := strings.TrimSpace(*pluginRow.Path)
		if repoPath != "" {
			repoPath = strings.ReplaceAll(repoPath, "\\", "/")
			repoPath = strings.Trim(repoPath, "/")
			ghSubdir = repoPath
		}
	}

	return ResolvedPlugin{
		Name:         "@" + usernameNormal + "/" + pluginName,
		Repo:         pluginRow.Repo,
		GitHubOwner:  ghOwner,
		GitHubRepo:   ghRepo,
		GitHubSubdir: ghSubdir,
		Version:      fmt.Sprintf("%d.%d.%d", selected.Major, selected.Minor, selected.Patch),
		SHA:          sha,
	}, nil
}

type usernameRow struct {
	UsernameDisplay *string `json:"username_display"`
	UserID          *string `json:"user_id"`
	OrgID           *string `json:"org_id"`
}

type pluginRow struct {
	ID        string  `json:"id"`
	Name      *string `json:"name"`
	Repo      string  `json:"repo"`
	Path      *string `json:"path"`
	CreatedAt *string `json:"created_at"`
	UserID    *string `json:"user_id"`
	OrgID     *string `json:"org_id"`
}

type versionRow struct {
	PluginID  *string `json:"plugin_id"`
	Major     int     `json:"major"`
	Minor     int     `json:"minor"`
	Patch     int     `json:"patch"`
	SHA       string  `json:"sha"`
	CreatedAt *string `json:"created_at"`
}

func (c *Client) getUsernameByNormal(ctx context.Context, usernameNormal string) (usernameRow, bool, error) {
	q := url.Values{}
	q.Set("select", "username_display,user_id,org_id")
	q.Set("username_normal", "eq."+usernameNormal)
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

func (c *Client) getPluginByOwnerAndName(ctx context.Context, userID, orgID *string, pluginName string) (pluginRow, bool, error) {
	q := url.Values{}
	selectWithPath := "id,name,repo,path,created_at,user_id,org_id"
	selectLegacy := "id,name,repo,created_at,user_id,org_id"
	q.Set("select", selectWithPath)
	q.Set("name", "eq."+pluginName)
	q.Set("limit", "2")

	if orgID != nil && strings.TrimSpace(*orgID) != "" {
		q.Set("org_id", "eq."+strings.TrimSpace(*orgID))
	} else if userID != nil && strings.TrimSpace(*userID) != "" {
		q.Set("user_id", "eq."+strings.TrimSpace(*userID))
	} else {
		return pluginRow{}, false, fmt.Errorf("owner has no id")
	}

	var rows []pluginRow
	if err := c.get(ctx, "addons", q, &rows); err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "path") &&
			(strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "could not find") || strings.Contains(errMsg, "schema cache")) {
			q.Set("select", selectLegacy)
			rows = nil
			if err2 := c.get(ctx, "addons", q, &rows); err2 != nil {
				return pluginRow{}, false, err2
			}
		} else {
			return pluginRow{}, false, err
		}
	}
	if len(rows) == 0 {
		return pluginRow{}, false, nil
	}
	if len(rows) > 1 {
		return pluginRow{}, false, fmt.Errorf("addon is not unique: %s", pluginName)
	}
	return rows[0], true, nil
}

func (c *Client) listPluginVersions(ctx context.Context, pluginID string) ([]versionRow, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return nil, fmt.Errorf("missing plugin id")
	}

	q := url.Values{}
	q.Set("select", "plugin_id,major,minor,patch,sha,created_at")
	q.Set("plugin_id", "eq."+pluginID)
	q.Set("order", "major.desc,minor.desc,patch.desc,created_at.desc")
	q.Set("limit", "100")

	var rows []versionRow
	if err := c.get(ctx, "plugin_versions", q, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []versionRow{}
	}
	return rows, nil
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
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", c.anonKey)
	req.Header.Set("Authorization", "Bearer "+c.anonKey)

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
