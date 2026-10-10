// Package resolve turns a project's pins into the exact set of release
// copies to install, and where each one goes.
//
// A Godot project has one addons directory and one global class namespace,
// so one release of an addon normally serves everyone: that copy is
// "hoisted" to addons/<dir>. Exact pins can disagree, though, and the
// package manager must not turn that into an install error, so a consumer
// whose pin differs from the hoisted tag gets its own copy nested inside its
// own directory, at <consumer>/.gdam/<dir>. That is npm's layout. The
// consumer reaches its dependencies only through a generated deps file, so
// it never has to know which of the two addresses it got.
//
// The registry refuses a dependency release that declares global classes,
// which is what makes a nested second copy loadable at all.
package resolve

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// Release is what the registry says about one exact release.
type Release struct {
	Name            string
	Tag             string
	GitHubOwner     string
	GitHubRepo      string
	GitHubReleaseID int64
	CommitSHA       string
	AssetID         int64
	AssetName       string
	AssetDigest     string
	PublishedAt     time.Time
	Prerelease      bool
	EditorPlugin    bool
	Dependencies    map[string]string
	GlobalClasses   []string
	// Local is set for a linked addon: it is not downloaded and never
	// nested; its dependencies come from its own gdam.json.
	Local bool
}

// Fetcher answers one exact release. Tag is never empty here: a project pin
// without a tag is resolved to one by the caller before resolution starts.
type Fetcher func(ctx context.Context, name, tag string) (Release, error)

// Copy is one installed directory.
type Copy struct {
	Release Release
	// Path is project-relative with forward slashes.
	Path string
	// Nested is true for a copy that lives inside its consumer.
	Nested bool
	// RequiredBy lists the declarations that asked for this copy:
	// "gdam.json" or "name@tag" of the consuming release.
	RequiredBy []string
	// Dependencies maps each dependency name to the path of the copy that
	// satisfies it for this consumer. It is what the deps file encodes.
	Dependencies map[string]string
}

// Plan is the full placement.
type Plan struct {
	// Copies in install order: every hoisted copy, then nested copies, each
	// parent before its children.
	Copies []*Copy
	// Hoisted maps addon name to the tag on the shelf.
	Hoisted map[string]string
}

// DirName is the addons/ directory name for an addon: "@owner/addon"
// becomes "@owner_addon".
func DirName(name string) string {
	return strings.ReplaceAll(name, "/", "_")
}

const nestedDir = ".gdam"

// Resolve builds the plan for the given project pins.
func Resolve(ctx context.Context, pins map[string]string, fetch Fetcher) (Plan, error) {
	if len(pins) == 0 {
		return Plan{Hoisted: map[string]string{}}, nil
	}
	// 1. The closure: every release reachable from the pins, each fetched once.
	releases := map[string]Release{}
	requesters := map[string]map[string][]string{} // name -> tag -> who asked
	type edge struct{ name, tag, by string }
	queue := []edge{}
	names := sortedKeys(pins)
	for _, name := range names {
		queue = append(queue, edge{name, pins[name], "gdam.json"})
	}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if _, ok := requesters[next.name]; !ok {
			requesters[next.name] = map[string][]string{}
		}
		requesters[next.name][next.tag] = append(requesters[next.name][next.tag], next.by)
		key := next.name + "@" + next.tag
		if _, seen := releases[key]; seen {
			continue
		}
		release, err := fetch(ctx, next.name, next.tag)
		if err != nil {
			return Plan{}, fmt.Errorf("resolving %s@%s (required by %s): %w", next.name, next.tag, next.by, err)
		}
		if release.Name == "" {
			release.Name = next.name
		}
		if release.Tag == "" {
			release.Tag = next.tag
		}
		releases[key] = release
		for _, depName := range sortedKeys(release.Dependencies) {
			depTag := release.Dependencies[depName]
			if depName == next.name {
				return Plan{}, fmt.Errorf("%s@%s depends on itself", next.name, next.tag)
			}
			queue = append(queue, edge{depName, depTag, key})
		}
	}

	// 2. The shelf: one tag per addon. The project's own pin wins; otherwise
	// the tag with the most requesters, then the most recently published.
	hoisted := map[string]string{}
	for name, byTag := range requesters {
		if tag, ok := pins[name]; ok {
			hoisted[name] = tag
			continue
		}
		var best string
		for _, tag := range sortedKeys(byTag) {
			if best == "" {
				best = tag
				continue
			}
			if len(byTag[tag]) > len(byTag[best]) || (len(byTag[tag]) == len(byTag[best]) && releases[name+"@"+tag].PublishedAt.After(releases[name+"@"+best].PublishedAt)) {
				best = tag
			}
		}
		hoisted[name] = best
	}

	// 3. Placement: hoisted copies first, then nested copies wherever a
	// consumer's pin is not the hoisted tag.
	plan := Plan{Hoisted: hoisted}
	var nested []*Copy
	place := func(copyPath string, consumer *Copy) {
		// Resolve each dependency of this copy to a path.
		for _, depName := range sortedKeys(consumer.Release.Dependencies) {
			depTag := consumer.Release.Dependencies[depName]
			if hoisted[depName] == depTag {
				consumer.Dependencies[depName] = path.Join("addons", DirName(depName))
				continue
			}
			if consumer.Release.Local {
				// A linked addon is the developer's own source tree; nothing
				// is written inside it. Its pins must match the shelf.
				consumer.Dependencies[depName] = ""
				continue
			}
			child := &Copy{
				Release:      releases[depName+"@"+depTag],
				Path:         path.Join(copyPath, nestedDir, DirName(depName)),
				Nested:       true,
				RequiredBy:   []string{consumer.Release.Name + "@" + consumer.Release.Tag},
				Dependencies: map[string]string{},
			}
			consumer.Dependencies[depName] = child.Path
			nested = append(nested, child)
		}
	}
	for _, name := range sortedKeys(hoisted) {
		tag := hoisted[name]
		release := releases[name+"@"+tag]
		c := &Copy{
			Release:      release,
			Path:         path.Join("addons", DirName(name)),
			RequiredBy:   append([]string(nil), requesters[name][tag]...),
			Dependencies: map[string]string{},
		}
		sort.Strings(c.RequiredBy)
		plan.Copies = append(plan.Copies, c)
	}
	for _, c := range plan.Copies {
		place(c.Path, c)
	}
	// Nested copies may need their own nested copies; walk until settled.
	for i := 0; i < len(nested); i++ {
		c := nested[i]
		place(c.Path, c)
	}
	plan.Copies = append(plan.Copies, nested...)

	for _, c := range plan.Copies {
		if c.Release.Local {
			for depName, depPath := range c.Dependencies {
				if depPath == "" {
					return Plan{}, fmt.Errorf("linked addon %s pins %s@%s but the project installs %s@%s; pin the same tag in gdam.json", c.Release.Name, depName, c.Release.Dependencies[depName], depName, hoisted[depName])
				}
			}
		}
	}
	return plan, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
