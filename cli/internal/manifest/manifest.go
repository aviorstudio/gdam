package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/fsutil"
)

const LinkFilename = "gdam.link.json"

type Manifest struct {
	Addons map[string]Addon `json:"addons"`
}

type Addon struct {
	Repo         string `json:"repo,omitempty"`
	Version      string `json:"version,omitempty"`
	AssetName    string `json:"asset_name,omitempty"`
	EditorPlugin bool   `json:"editor_plugin,omitempty"`
	Link         *Link  `json:"link,omitempty"`
}

type Link struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path,omitempty"`
}

type LinkManifest struct {
	Addons map[string]Link `json:"addons"`
}

func (l *Link) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*l = Link{}
		return nil
	}
	if data[0] != '{' {
		return fmt.Errorf("link must be an object")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k := range raw {
		switch k {
		case "enabled", "path":
		default:
			return fmt.Errorf("unknown link field %q", k)
		}
	}

	enabledRaw, ok := raw["enabled"]
	if !ok {
		return fmt.Errorf("missing link.enabled")
	}
	var enabled bool
	if err := json.Unmarshal(enabledRaw, &enabled); err != nil {
		return err
	}

	var path string
	if pathRaw, ok := raw["path"]; ok {
		if err := json.Unmarshal(pathRaw, &path); err != nil {
			return err
		}
	}
	path = strings.TrimSpace(path)
	if enabled && path == "" {
		return fmt.Errorf("link is enabled but path is empty")
	}

	*l = Link{
		Enabled: enabled,
		Path:    path,
	}
	return nil
}

func (p *Addon) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k := range raw {
		switch k {
		case "repo", "version", "asset_name", "editor_plugin":
		case "link":
			return fmt.Errorf("gdam.json no longer supports link configuration (move it to %s)", LinkFilename)
		default:
			return fmt.Errorf("unknown field %q", k)
		}
	}

	var tmp struct {
		Repo         string `json:"repo,omitempty"`
		Version      string `json:"version,omitempty"`
		AssetName    string `json:"asset_name,omitempty"`
		EditorPlugin bool   `json:"editor_plugin,omitempty"`
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	*p = Addon{
		Repo:         tmp.Repo,
		Version:      tmp.Version,
		AssetName:    tmp.AssetName,
		EditorPlugin: tmp.EditorPlugin,
	}
	return nil
}

func New() Manifest {
	return Manifest{
		Addons: map[string]Addon{},
	}
}

func Load(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}

	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, err
	}
	if m.Addons == nil {
		m.Addons = map[string]Addon{}
	}

	linkPath := filepath.Join(filepath.Dir(path), LinkFilename)
	lm, err := LoadLinkManifest(linkPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return Manifest{}, err
		}
		return m, nil
	}
	for name, link := range lm.Addons {
		addon, ok := m.Addons[name]
		if !ok {
			continue
		}
		l := link
		addon.Link = &l
		m.Addons[name] = addon
	}

	return m, nil
}

func Save(path string, m Manifest) error {
	if m.Addons == nil {
		m.Addons = map[string]Addon{}
	}

	linkPath := filepath.Join(filepath.Dir(path), LinkFilename)
	links := LinkManifest{Addons: map[string]Link{}}
	outManifest := Manifest{Addons: map[string]Addon{}}
	for name, addon := range m.Addons {
		if addon.Link != nil {
			links.Addons[name] = *addon.Link
		}
		addon.Link = nil
		outManifest.Addons[name] = addon
	}

	if len(links.Addons) != 0 {
		if err := SaveLinkManifest(linkPath, links); err != nil {
			return err
		}
	} else {
		if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	out, err := json.MarshalIndent(outManifest, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return fsutil.WriteFileAtomic(path, out, 0o644)
}

func LoadLinkManifest(path string) (LinkManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return LinkManifest{}, err
	}

	var m LinkManifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return LinkManifest{}, err
	}
	if m.Addons == nil {
		m.Addons = map[string]Link{}
	}
	return m, nil
}

func SaveLinkManifest(path string, m LinkManifest) error {
	if m.Addons == nil {
		m.Addons = map[string]Link{}
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return fsutil.WriteFileAtomic(path, out, 0o644)
}

func HasAddon(m Manifest, name string) bool {
	_, ok := m.Addons[name]
	return ok
}

func UpsertAddon(m Manifest, name string, addon Addon) Manifest {
	if m.Addons == nil {
		m.Addons = map[string]Addon{}
	}
	m.Addons[name] = addon
	return m
}

func RemoveAddon(m Manifest, name string) Manifest {
	delete(m.Addons, name)
	return m
}
