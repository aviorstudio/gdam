package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aviorstudio/gdam/internal/gdamdb"
	"github.com/aviorstudio/gdam/internal/githubapi"
	"github.com/aviorstudio/gdam/internal/lockfile"
	"github.com/aviorstudio/gdam/internal/manifest"
)

// fakeRelease is one release the fake registry knows: its declaration and
// the files its asset would unpack to.
type fakeRelease struct {
	deps    map[string]string
	files   map[string]string
	editor  bool
	publish time.Time
}

// withFakeRegistryGraph serves a whole dependency graph: resolve answers
// from the map and "download" writes the release's files.
func withFakeRegistryGraph(t *testing.T, graph map[string]fakeRelease) *int {
	t.Helper()
	previousResolve := resolveAddonFromRegistry
	previousPrepare := preparePackageRoot
	resolves := 0
	resolveAddonFromRegistry = func(ctx context.Context, owner, addon, requestedTag string) (gdamdb.ResolvedAddon, error) {
		resolves++
		name := "@" + owner + "/" + addon
		tag := requestedTag
		if tag == "" {
			// Newest: the fake keeps one "latest" per addon under tag "".
			for key := range graph {
				if strings.HasPrefix(key, name+"@") {
					tag = strings.TrimPrefix(key, name+"@")
				}
			}
		}
		release, ok := graph[name+"@"+tag]
		if !ok {
			return gdamdb.ResolvedAddon{}, &notPublished{name + "@" + tag}
		}
		return gdamdb.ResolvedAddon{
			Name: name, GitHubOwner: owner, GitHubRepo: addon, TagName: tag,
			GitHubReleaseID: 100, CommitSHA: strings.Repeat("c", 40), AssetID: 200, AssetName: "@" + owner + "_" + addon + ".zip",
			AssetDigest: "sha256:" + strings.Repeat("0", 64), PublishedAt: release.publish, EditorPlugin: release.editor,
			Dependencies: release.deps,
		}, nil
	}
	preparePackageRoot = func(ctx context.Context, gh *githubapi.Client, resolved gdamdb.ResolvedAddon, tmpDir string) (string, error) {
		root := filepath.Join(tmpDir, "pkg")
		release := graph[resolved.Name+"@"+resolved.TagName]
		files := map[string]string{"plugin.cfg": "[plugin]\nname=\"" + resolved.Name + "\"\nversion=\"" + resolved.TagName + "\"\n"}
		for name, body := range release.files {
			files[name] = body
		}
		for name, body := range files {
			p := filepath.Join(root, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				return "", err
			}
		}
		return root, nil
	}
	t.Cleanup(func() {
		resolveAddonFromRegistry = previousResolve
		preparePackageRoot = previousPrepare
	})
	return &resolves
}

type notPublished struct{ key string }

func (n *notPublished) Error() string { return n.key + " is not published" }

func chdir(t *testing.T, dir string) {
	t.Helper()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
}

func writeManifest(t *testing.T, dir string, pins map[string]string, pkg string) {
	t.Helper()
	m := manifest.New()
	for name, tag := range pins {
		m = manifest.UpsertAddon(m, name, manifest.Addon{Tag: tag})
	}
	m.Package = pkg
	if err := manifest.Save(filepath.Join(dir, "gdam.json"), m); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(raw)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

var (
	t0      = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	session = map[string]string{"src/credential_adapter.gd": "extends RefCounted\n", "src/credential_adapter.gd.uid": "uid://b0cjekr1r63dq\n", "plugin.gd": "@tool\nextends EditorPlugin\n"}
)

func TestInstall_DependenciesHoistWriteDepsFileAndLock(t *testing.T) {
	resolves := withFakeRegistryGraph(t, map[string]fakeRelease{
		"@o/clerk@v1":   {deps: map[string]string{"@o/session": "v1"}, files: map[string]string{"src/clerk.gd": "extends RefCounted\n"}, publish: t0},
		"@o/session@v1": {files: session, publish: t0},
	})
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{"@o/clerk": "v1"}, "")
	chdir(t, dir)

	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dir, "addons/@o_clerk/plugin.cfg")) || !exists(filepath.Join(dir, "addons/@o_session/src/credential_adapter.gd")) {
		t.Fatal("dependency was not installed beside its consumer")
	}
	deps := read(t, filepath.Join(dir, "addons/@o_clerk/.gdam/deps.gd"))
	for _, want := range []string{
		`"@o/session": "res://addons/@o_session"`,
		`const Session_credential_adapter = preload("res://addons/@o_session/src/credential_adapter.gd")`,
	} {
		if !strings.Contains(deps, want) {
			t.Errorf("deps.gd lacks %s:\n%s", want, deps)
		}
	}
	if strings.Contains(deps, "plugin.gd") {
		t.Errorf("deps.gd exposes the editor plugin entry point:\n%s", deps)
	}
	if exists(filepath.Join(dir, "addons/@o_session/.gdam")) {
		t.Error("a release without dependencies got a deps file")
	}

	lock, err := lockfile.Load(filepath.Join(dir, lockfile.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if lock.Requirements["@o/clerk"] != "v1" || len(lock.Addons) != 2 {
		t.Fatalf("lock %+v", lock)
	}
	entry := lock.Addons["@o/session@v1"]
	if entry == nil || entry.AssetID != 200 || entry.AssetDigest != "sha256:"+strings.Repeat("0", 64) || entry.Installs[0].Path != "addons/@o_session" || entry.Installs[0].RequiredBy[0] != "@o/clerk@v1" {
		t.Fatalf("lock entry %+v", entry)
	}

	// A second install answers from the lock: no registry call, same tree.
	before := *resolves
	if err := Install(context.Background(), InstallOptions{FrozenLockfile: true}); err != nil {
		t.Fatal(err)
	}
	if *resolves != before {
		t.Fatalf("install from a matching lock resolved %d times", *resolves-before)
	}
}

func TestInstall_DisagreeingPinNestsWithFreshUIDs(t *testing.T) {
	withFakeRegistryGraph(t, map[string]fakeRelease{
		"@o/clerk@v1":   {deps: map[string]string{"@o/session": "v1"}, files: map[string]string{"src/clerk.gd": "extends RefCounted\n"}, publish: t0},
		"@o/session@v1": {files: session, publish: t0},
		"@o/session@v2": {files: session, publish: t0.Add(time.Hour)},
	})
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{"@o/clerk": "v1", "@o/session": "v2"}, "")
	chdir(t, dir)

	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	hoisted := read(t, filepath.Join(dir, "addons/@o_session/plugin.cfg"))
	nested := read(t, filepath.Join(dir, "addons/@o_clerk/.gdam/@o_session/plugin.cfg"))
	if !strings.Contains(hoisted, `version="v2"`) || !strings.Contains(nested, `version="v1"`) {
		t.Fatalf("hoisted:\n%s\nnested:\n%s", hoisted, nested)
	}
	deps := read(t, filepath.Join(dir, "addons/@o_clerk/.gdam/deps.gd"))
	if !strings.Contains(deps, `preload("res://addons/@o_clerk/.gdam/@o_session/src/credential_adapter.gd")`) {
		t.Fatalf("deps.gd does not point at the nested copy:\n%s", deps)
	}
	hoistedUID := strings.TrimSpace(read(t, filepath.Join(dir, "addons/@o_session/src/credential_adapter.gd.uid")))
	nestedUID := strings.TrimSpace(read(t, filepath.Join(dir, "addons/@o_clerk/.gdam/@o_session/src/credential_adapter.gd.uid")))
	if hoistedUID != "uid://b0cjekr1r63dq" || nestedUID == hoistedUID || !strings.HasPrefix(nestedUID, "uid://") {
		t.Fatalf("uids: hoisted %s nested %s", hoistedUID, nestedUID)
	}

	lock, err := lockfile.Load(filepath.Join(dir, lockfile.Filename))
	if err != nil {
		t.Fatal(err)
	}
	if lock.Addons["@o/session@v1"].Installs[0].Path != "addons/@o_clerk/.gdam/@o_session" || lock.Addons["@o/session@v2"].Installs[0].Path != "addons/@o_session" {
		t.Fatalf("lock %+v", lock.Addons)
	}

	// The project moves its pin to v1: the nested copy is pruned and the
	// shelf serves clerk.
	writeManifest(t, dir, map[string]string{"@o/clerk": "v1", "@o/session": "v1"}, "")
	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "addons/@o_clerk/.gdam/@o_session")) {
		t.Fatal("nested copy survived the project agreeing with the consumer")
	}
	if !strings.Contains(read(t, filepath.Join(dir, "addons/@o_clerk/.gdam/deps.gd")), `preload("res://addons/@o_session/src/credential_adapter.gd")`) {
		t.Fatal("deps.gd not rewritten to the hoisted copy")
	}
}

func TestInstall_FrozenLockfileRefusesToResolve(t *testing.T) {
	withFakeRegistryGraph(t, map[string]fakeRelease{"@o/session@v1": {files: session, publish: t0}})
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{"@o/session": "v1"}, "")
	chdir(t, dir)
	err := Install(context.Background(), InstallOptions{FrozenLockfile: true})
	if err == nil || !strings.Contains(err.Error(), "--frozen-lockfile requires gdam.lock") {
		t.Fatalf("no lock: %v", err)
	}
	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, map[string]string{"@o/session": "v1", "@o/other": ""}, "")
	err = Install(context.Background(), InstallOptions{FrozenLockfile: true})
	if err == nil || !strings.Contains(err.Error(), "does not answer gdam.json") {
		t.Fatalf("stale lock: %v", err)
	}
}

func TestRemove_PrunesDependenciesNothingElseNeeds(t *testing.T) {
	withFakeRegistryGraph(t, map[string]fakeRelease{
		"@o/clerk@v1":   {deps: map[string]string{"@o/session": "v1"}, publish: t0},
		"@o/session@v1": {files: session, publish: t0},
	})
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{"@o/clerk": "v1"}, "")
	chdir(t, dir)
	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Remove(context.Background(), RemoveOptions{Spec: "@o/clerk"}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "addons/@o_clerk")) || exists(filepath.Join(dir, "addons/@o_session")) {
		t.Fatal("removing the consumer left its dependency behind")
	}
	lock, err := lockfile.Load(filepath.Join(dir, lockfile.Filename))
	if err != nil || len(lock.Addons) != 0 {
		t.Fatalf("lock after remove: %v %+v", err, lock)
	}
}

func TestInstall_PackageDirectoryGetsDepsFileAndMustAgree(t *testing.T) {
	withFakeRegistryGraph(t, map[string]fakeRelease{
		"@o/session@v1": {files: session, publish: t0},
		"@o/session@v2": {files: session, publish: t0},
	})
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "addon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "addon", "gdam.json"), []byte(`{"addons":{"@o/session":{"tag":"v1"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, map[string]string{"@o/session": "v1"}, "addon")
	chdir(t, dir)
	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	deps := read(t, filepath.Join(dir, "addon/.gdam/deps.gd"))
	if !strings.Contains(deps, `const Session_credential_adapter = preload("res://addons/@o_session/src/credential_adapter.gd")`) {
		t.Fatalf("package deps.gd:\n%s", deps)
	}

	writeManifest(t, dir, map[string]string{"@o/session": "v2"}, "addon")
	err := Install(context.Background(), InstallOptions{})
	if err == nil || !strings.Contains(err.Error(), "addon/gdam.json declares @o/session@v1 but the project installs v2") {
		t.Fatalf("disagreement: %v", err)
	}
}
