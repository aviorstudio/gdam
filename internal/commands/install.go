package commands

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aviorstudio/gdam/internal/fsutil"
	"github.com/aviorstudio/gdam/internal/gdamdb"
	"github.com/aviorstudio/gdam/internal/githubapi"
	"github.com/aviorstudio/gdam/internal/godotuid"
	"github.com/aviorstudio/gdam/internal/lockfile"
	"github.com/aviorstudio/gdam/internal/manifest"
	"github.com/aviorstudio/gdam/internal/project"
	"github.com/aviorstudio/gdam/internal/resolve"
)

type InstallOptions struct {
	// FrozenLockfile refuses to resolve: gdam.lock must already answer
	// gdam.json exactly. CI passes it so a stale lock fails loudly instead
	// of being rewritten on the runner.
	FrozenLockfile bool
}

// Install brings addons/ to what gdam.json asks for, transitively.
//
// The project's pins are resolved against the registry once and recorded
// in gdam.lock with each release's verified identity; a later install whose
// gdam.json still matches the lock installs from the lock alone. Every
// release in the set is downloaded by its locked release id and asset id,
// verified against its digest, and copied to the place the resolver chose:
// addons/<dir> for the one copy the project shares (hoisted), or
// <consumer>/.gdam/<dir> for a consumer whose exact pin differs from the
// project's (nested). Each copy that declares dependencies gets a generated
// .gdam/deps.gd naming where its dependencies landed.
func Install(ctx context.Context, opts InstallOptions) error {
	startDir, err := os.Getwd()
	if err != nil {
		return err
	}
	projectDir, ok := project.FindManifestDir(startDir)
	if !ok {
		return fmt.Errorf("%w: no gdam.json found (run `gdam init`)", ErrUserInput)
	}
	return installProject(ctx, projectDir, opts)
}

func installProject(ctx context.Context, projectDir string, opts InstallOptions) error {
	manifestPath := filepath.Join(projectDir, "gdam.json")
	m, err := manifest.Load(manifestPath)
	if err != nil {
		return err
	}
	for pluginKey := range m.Addons {
		addonDirName, err := addonDirNameForPluginKey(pluginKey)
		if err != nil {
			return fmt.Errorf("%w: invalid addon key in gdam.json: %s (%v)", ErrUserInput, pluginKey, err)
		}
		if err := validateNoAddonDirCollision(m, pluginKey, addonDirName); err != nil {
			return err
		}
	}

	linked := map[string]string{}
	for name, addon := range m.Addons {
		if pluginLinkEnabled(addon) {
			abs, err := pluginAbsPath(projectDir, addon.Link.Path)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrUserInput, err)
			}
			linked[name] = abs
		}
	}
	pins := manifest.Pins(m)

	lockPath := filepath.Join(projectDir, lockfile.Filename)
	previous, err := lockfile.Load(lockPath)
	hasLock := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	var plan resolve.Plan
	var lock lockfile.Lock
	// A linked addon is a developer's tree whose declaration can change
	// without gdam.json changing, so links always re-resolve.
	if hasLock && len(linked) == 0 && previous.Matches(pins) {
		plan, err = planFromLock(previous)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		}
		lock = previous
	} else {
		if opts.FrozenLockfile {
			if !hasLock {
				return fmt.Errorf("%w: --frozen-lockfile requires %s; run `gdam install` once to create it", ErrUserInput, lockfile.Filename)
			}
			return fmt.Errorf("%w: %s does not answer gdam.json; run `gdam install` without --frozen-lockfile to re-resolve", ErrUserInput, lockfile.Filename)
		}
		// A pin without a tag asks the registry for its newest release; it
		// becomes exact here, and the lock records what it became.
		for name, tag := range pins {
			if tag != "" {
				continue
			}
			owner, repo := splitAddonName(name)
			resolved, err := resolveAddonFromRegistry(ctx, owner, repo, "")
			if err != nil {
				return fmt.Errorf("%w: unable to resolve %s: %v", ErrUserInput, name, err)
			}
			pins[name] = resolved.TagName
		}
		roots := map[string]string{}
		for name, tag := range pins {
			roots[name] = tag
		}
		for name := range linked {
			roots[name] = "linked"
		}
		plan, err = resolve.Resolve(ctx, roots, registryFetcher(linked))
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		}
		lock = lockFromPlan(pins, plan)
		if err := lockfile.Save(lockPath, lock); err != nil {
			return err
		}
	}

	if len(plan.Copies) == 0 && m.Package == "" {
		if hasLock {
			if err := prune(projectDir, previous, lock); err != nil {
				return err
			}
		}
		return nil
	}

	addonsDir := filepath.Join(projectDir, "addons")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		return err
	}
	projectGodotPath := filepath.Join(projectDir, "project.godot")
	hasProjectGodot := false
	if _, err := os.Stat(projectGodotPath); err == nil {
		hasProjectGodot = true
	} else if !os.IsNotExist(err) {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "gdam-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	gh := githubapi.NewClient(os.Getenv("GITHUB_TOKEN"))

	for i, c := range plan.Copies {
		if c.Release.Local {
			continue
		}
		dst := filepath.Join(projectDir, filepath.FromSlash(c.Path))
		pkgTmpDir := filepath.Join(tmpDir, fmt.Sprintf("pkg-%d", i))
		if err := os.MkdirAll(pkgTmpDir, 0o755); err != nil {
			return err
		}
		pkgRootDir, err := preparePackageRoot(ctx, gh, resolvedFromRelease(c.Release), pkgTmpDir)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		}
		if ok, err := pluginCfgExistsAtDirRoot(pkgRootDir); err != nil {
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		} else if !ok {
			return fmt.Errorf("%w: package is missing plugin.cfg in release asset %s (expected to install it to res://%s/plugin.cfg)", ErrUserInput, c.Release.AssetName, c.Path)
		}
		if err := fsutil.RemoveAll(dst); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := fsutil.CopyPath(pkgRootDir, dst); err != nil {
			return err
		}
		if ok, err := pluginCfgExistsAtDirRoot(dst); err != nil {
			_ = fsutil.RemoveAll(dst)
			return fmt.Errorf("%w: %v", ErrUserInput, err)
		} else if !ok {
			_ = fsutil.RemoveAll(dst)
			return fmt.Errorf("%w: installed addon is missing plugin.cfg at %s", ErrUserInput, filepath.Join(dst, "plugin.cfg"))
		}
		if c.Nested {
			// A second copy of the same scripts needs its own uids, or the
			// editor sees two files claiming one id.
			if _, err := godotuid.Regenerate(dst); err != nil {
				return fmt.Errorf("regenerating uids in %s: %w", c.Path, err)
			}
		}
		if hasProjectGodot && c.Release.EditorPlugin && !c.Nested {
			pluginCfgResPath := "res://" + path.Join(c.Path, "plugin.cfg")
			updated, err := project.SetEditorPluginEnabled(projectGodotPath, pluginCfgResPath, true)
			if err != nil {
				return err
			}
			if updated {
				fmt.Printf("enabled %s\n", pluginCfgResPath)
			}
		}
		if c.Nested {
			fmt.Printf("installed %s@%s nested at %s (%s pins it)\n", c.Release.Name, c.Release.Tag, c.Path, strings.Join(c.RequiredBy, ", "))
		} else {
			fmt.Printf("installed %s@%s\n", c.Release.Name, c.Release.Tag)
		}
	}

	// Dependencies are reached through generated files, written once every
	// copy they point at exists.
	for _, c := range plan.Copies {
		if c.Release.Local || len(c.Dependencies) == 0 {
			continue
		}
		dst := filepath.Join(projectDir, filepath.FromSlash(c.Path))
		if err := writeDepsFile(projectDir, dst, c.Release.Name+"@"+c.Release.Tag, c.Dependencies); err != nil {
			return err
		}
	}
	if m.Package != "" {
		if err := writePackageDeps(projectDir, m.Package, plan); err != nil {
			return err
		}
	}

	if hasLock {
		if err := prune(projectDir, previous, lock); err != nil {
			return err
		}
	}
	return nil
}

// registryFetcher answers the resolver from the registry, except for
// linked addons, which are read from their own tree.
func registryFetcher(linked map[string]string) resolve.Fetcher {
	return func(ctx context.Context, name, tag string) (resolve.Release, error) {
		if dir, ok := linked[name]; ok {
			deps, err := packageDependencies(dir)
			if err != nil {
				return resolve.Release{}, err
			}
			return resolve.Release{Name: name, Tag: tag, Dependencies: deps, Local: true}, nil
		}
		owner, repo := splitAddonName(name)
		resolved, err := resolveAddonFromRegistry(ctx, owner, repo, tag)
		if err != nil {
			return resolve.Release{}, err
		}
		if tag != "" && resolved.TagName != tag {
			return resolve.Release{}, fmt.Errorf("registry answered %s for tag %s", resolved.TagName, tag)
		}
		return releaseFromResolved(resolved), nil
	}
}

func splitAddonName(name string) (string, string) {
	owner, repo, _ := strings.Cut(strings.TrimPrefix(name, "@"), "/")
	return owner, repo
}

func releaseFromResolved(r gdamdb.ResolvedAddon) resolve.Release {
	return resolve.Release{
		Name: r.Name, Tag: r.TagName, GitHubOwner: r.GitHubOwner, GitHubRepo: r.GitHubRepo,
		GitHubReleaseID: r.GitHubReleaseID, CommitSHA: r.CommitSHA, AssetID: r.AssetID, AssetName: r.AssetName,
		AssetDigest: r.AssetDigest, PublishedAt: r.PublishedAt, Prerelease: r.Prerelease, EditorPlugin: r.EditorPlugin,
		Dependencies: r.Dependencies, GlobalClasses: r.GlobalClasses,
	}
}

func resolvedFromRelease(r resolve.Release) gdamdb.ResolvedAddon {
	return gdamdb.ResolvedAddon{
		Name: r.Name, GitHubOwner: r.GitHubOwner, GitHubRepo: r.GitHubRepo, TagName: r.Tag,
		GitHubReleaseID: r.GitHubReleaseID, CommitSHA: r.CommitSHA, AssetID: r.AssetID, AssetName: r.AssetName,
		AssetDigest: r.AssetDigest, PublishedAt: r.PublishedAt, Prerelease: r.Prerelease, EditorPlugin: r.EditorPlugin,
		Dependencies: r.Dependencies, GlobalClasses: r.GlobalClasses,
	}
}

// lockFromPlan records a plan: the pins it answers and every copy.
func lockFromPlan(pins map[string]string, plan resolve.Plan) lockfile.Lock {
	lock := lockfile.New()
	for name, tag := range pins {
		lock.Requirements[name] = tag
	}
	for _, c := range plan.Copies {
		if c.Release.Local {
			continue
		}
		key := lockfile.Key(c.Release.Name, c.Release.Tag)
		entry, ok := lock.Addons[key]
		if !ok {
			r := c.Release
			entry = &lockfile.Release{
				Name: r.Name, Tag: r.Tag, GitHubOwner: r.GitHubOwner, GitHubRepo: r.GitHubRepo,
				GitHubReleaseID: r.GitHubReleaseID, CommitSHA: r.CommitSHA, AssetID: r.AssetID, AssetName: r.AssetName,
				AssetDigest: r.AssetDigest, PublishedAt: r.PublishedAt, Prerelease: r.Prerelease, EditorPlugin: r.EditorPlugin,
				Dependencies: r.Dependencies,
			}
			lock.Addons[key] = entry
		}
		entry.Installs = append(entry.Installs, lockfile.Install{Path: c.Path, RequiredBy: append([]string(nil), c.RequiredBy...)})
	}
	return lock
}

// planFromLock rebuilds the plan a lock records, so install needs no
// registry. Dependency paths are recovered from the install paths: a
// consumer's dependency is the hoisted copy when its tag is on the shelf,
// else the copy nested directly under the consumer.
func planFromLock(lock lockfile.Lock) (resolve.Plan, error) {
	plan := resolve.Plan{Hoisted: map[string]string{}}
	byPath := map[string]*resolve.Copy{}
	for _, entry := range lock.Addons {
		release := resolve.Release{
			Name: entry.Name, Tag: entry.Tag, GitHubOwner: entry.GitHubOwner, GitHubRepo: entry.GitHubRepo,
			GitHubReleaseID: entry.GitHubReleaseID, CommitSHA: entry.CommitSHA, AssetID: entry.AssetID, AssetName: entry.AssetName,
			AssetDigest: entry.AssetDigest, PublishedAt: entry.PublishedAt, Prerelease: entry.Prerelease, EditorPlugin: entry.EditorPlugin,
			Dependencies: entry.Dependencies,
		}
		for _, install := range entry.Installs {
			nested := strings.Contains(install.Path, "/"+depsDir+"/")
			if !nested {
				plan.Hoisted[entry.Name] = entry.Tag
			}
			c := &resolve.Copy{Release: release, Path: install.Path, Nested: nested, RequiredBy: append([]string(nil), install.RequiredBy...), Dependencies: map[string]string{}}
			byPath[install.Path] = c
			plan.Copies = append(plan.Copies, c)
		}
	}
	// Hoisted first, then by depth, so parents precede children.
	sort.Slice(plan.Copies, func(i, j int) bool {
		di, dj := strings.Count(plan.Copies[i].Path, "/"), strings.Count(plan.Copies[j].Path, "/")
		if di != dj {
			return di < dj
		}
		return plan.Copies[i].Path < plan.Copies[j].Path
	})
	for _, c := range plan.Copies {
		for depName, depTag := range c.Release.Dependencies {
			hoistedPath := path.Join("addons", resolve.DirName(depName))
			if plan.Hoisted[depName] == depTag {
				c.Dependencies[depName] = hoistedPath
				continue
			}
			nestedPath := path.Join(c.Path, depsDir, resolve.DirName(depName))
			if _, ok := byPath[nestedPath]; !ok {
				return resolve.Plan{}, fmt.Errorf("%s records %s depending on %s@%s but installs it nowhere; delete the lock and run `gdam install`", lockfile.Filename, c.Path, depName, depTag)
			}
			c.Dependencies[depName] = nestedPath
		}
	}
	return plan, nil
}

// prune removes the copies a previous lock installed that the new one does
// not: an addon dropped from gdam.json, or a nested copy whose consumer now
// agrees with the shelf. Only paths under addons/ that the old lock named
// are touched; nothing gdam did not install is removed.
func prune(projectDir string, previous, current lockfile.Lock) error {
	keep := map[string]bool{}
	for _, p := range current.Paths() {
		keep[p] = true
	}
	for _, p := range previous.Paths() {
		if keep[p] || !strings.HasPrefix(p, "addons/") {
			continue
		}
		abs := filepath.Join(projectDir, filepath.FromSlash(p))
		if err := fsutil.RemoveAll(abs); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", p)
	}
	return nil
}

// packageDependencies reads an addon source directory's own gdam.json: the
// declaration a release asset ships, in the same shape as a project's.
func packageDependencies(dir string) (map[string]string, error) {
	p := filepath.Join(dir, "gdam.json")
	raw, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	declared, err := manifest.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	deps := map[string]string{}
	for name, addon := range declared.Addons {
		if strings.TrimSpace(addon.Tag) == "" {
			return nil, fmt.Errorf("%s: dependency %s needs an exact tag", p, name)
		}
		deps[name] = addon.Tag
	}
	return deps, nil
}

// writePackageDeps serves the addon under development in its own
// repository: its declaration must agree with the project's shelf, and its
// deps file points at the hoisted copies, exactly as an installed copy's
// would.
func writePackageDeps(projectDir, pkg string, plan resolve.Plan) error {
	pkgDir := filepath.Join(projectDir, filepath.FromSlash(pkg))
	if info, err := os.Stat(pkgDir); err != nil || !info.IsDir() {
		return fmt.Errorf("%w: gdam.json names package directory %q, which does not exist", ErrUserInput, pkg)
	}
	deps, err := packageDependencies(pkgDir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUserInput, err)
	}
	paths := map[string]string{}
	for name, tag := range deps {
		if plan.Hoisted[name] != tag {
			have := plan.Hoisted[name]
			if have == "" {
				return fmt.Errorf("%w: %s/gdam.json declares %s@%s but the project does not install it; add it with `gdam add %s@%s`", ErrUserInput, pkg, name, tag, name, tag)
			}
			return fmt.Errorf("%w: %s/gdam.json declares %s@%s but the project installs %s; pin the same tag in gdam.json", ErrUserInput, pkg, name, tag, have)
		}
		paths[name] = path.Join("addons", resolve.DirName(name))
	}
	return writeDepsFile(projectDir, pkgDir, pkg, paths)
}
