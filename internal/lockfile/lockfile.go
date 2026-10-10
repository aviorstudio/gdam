// Package lockfile is gdam.lock: the exact set of addon releases a project
// installs, where each copy lands, and which declaration asked for it.
//
// gdam.json holds what the project asked for; the lock holds what that
// resolves to, transitively, with the release identity (GitHub release id,
// asset id, digest) that `gdam install` verifies before extracting a byte.
// A project with a lock installs without consulting the registry; a project
// whose gdam.json no longer matches the lock's recorded requirements is
// re-resolved and the lock rewritten, unless --frozen-lockfile forbids it.
package lockfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/aviorstudio/gdam/internal/fsutil"
)

const Filename = "gdam.lock"

const Version = 1

// Lock is the file. Requirements is a copy of gdam.json's pins at the time
// of resolution, so install can tell whether the lock still answers the
// manifest. Addons is keyed by "name@tag": one release may be installed
// more than once (hoisted for the project, nested under an addon that pins
// a different tag than the project does), so the key is the release, not
// the addon.
type Lock struct {
	Version      int                 `json:"version"`
	Requirements map[string]string   `json:"requirements"`
	Addons       map[string]*Release `json:"addons"`
}

// Release is one resolved release and every place it is installed.
type Release struct {
	Name            string            `json:"name"`
	Tag             string            `json:"tag"`
	GitHubOwner     string            `json:"github_owner"`
	GitHubRepo      string            `json:"github_repo"`
	GitHubReleaseID int64             `json:"github_release_id"`
	CommitSHA       string            `json:"commit_sha"`
	AssetID         int64             `json:"asset_id"`
	AssetName       string            `json:"asset_name"`
	AssetDigest     string            `json:"asset_digest"`
	PublishedAt     time.Time         `json:"published_at"`
	Prerelease      bool              `json:"prerelease"`
	EditorPlugin    bool              `json:"editor_plugin"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	Installs        []Install         `json:"installs"`
}

// Install is one copy on disk. Path is relative to the project directory
// with forward slashes ("addons/@owner_addon", or for a nested copy
// "addons/@owner_consumer/.gdam/@owner_addon"). RequiredBy names the
// declarations that asked for this copy: "gdam.json" for the project's own
// pin, otherwise the consuming release as "name@tag".
type Install struct {
	Path       string   `json:"path"`
	RequiredBy []string `json:"required_by"`
}

func Key(name, tag string) string {
	return name + "@" + tag
}

func New() Lock {
	return Lock{Version: Version, Requirements: map[string]string{}, Addons: map[string]*Release{}}
}

func Load(path string) (Lock, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Lock{}, err
	}
	var lock Lock
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&lock); err != nil {
		return Lock{}, fmt.Errorf("%s: %w", Filename, err)
	}
	if lock.Version != Version {
		return Lock{}, fmt.Errorf("%s: unsupported version %d (this gdam writes %d)", Filename, lock.Version, Version)
	}
	if lock.Requirements == nil {
		lock.Requirements = map[string]string{}
	}
	if lock.Addons == nil {
		lock.Addons = map[string]*Release{}
	}
	for key, release := range lock.Addons {
		if release == nil || release.Name == "" || release.Tag == "" || Key(release.Name, release.Tag) != key {
			return Lock{}, fmt.Errorf("%s: entry %q does not describe %s", Filename, key, key)
		}
		if release.GitHubReleaseID <= 0 || release.AssetID <= 0 || release.AssetDigest == "" || release.AssetName == "" || release.CommitSHA == "" {
			return Lock{}, fmt.Errorf("%s: entry %q lacks a verified release identity", Filename, key)
		}
		if len(release.Installs) == 0 {
			return Lock{}, fmt.Errorf("%s: entry %q is installed nowhere", Filename, key)
		}
	}
	return lock, nil
}

// Save writes the lock with stable ordering so a rewrite that changes
// nothing produces identical bytes.
func Save(path string, lock Lock) error {
	lock.Version = Version
	if lock.Requirements == nil {
		lock.Requirements = map[string]string{}
	}
	if lock.Addons == nil {
		lock.Addons = map[string]*Release{}
	}
	for _, release := range lock.Addons {
		sort.Slice(release.Installs, func(i, j int) bool { return release.Installs[i].Path < release.Installs[j].Path })
		for i := range release.Installs {
			sort.Strings(release.Installs[i].RequiredBy)
		}
	}
	out, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return fsutil.WriteFileAtomic(path, out, 0o644)
}

// Matches reports whether the lock was resolved from exactly these
// requirements, which is the condition for installing from it without the
// registry.
func (l Lock) Matches(requirements map[string]string) bool {
	if len(l.Requirements) != len(requirements) {
		return false
	}
	for name, tag := range requirements {
		if l.Requirements[name] != tag {
			return false
		}
	}
	return true
}

// Paths lists every install path in the lock, sorted.
func (l Lock) Paths() []string {
	var out []string
	for _, release := range l.Addons {
		for _, install := range release.Installs {
			out = append(out, install.Path)
		}
	}
	sort.Strings(out)
	return out
}
