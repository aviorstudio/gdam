package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aviorstudio/gdam/internal/gdamdb"
	"github.com/aviorstudio/gdam/internal/githubapi"
	"github.com/aviorstudio/gdam/internal/lockfile"
	"github.com/aviorstudio/gdam/internal/manifest"
)

func TestUpdateFailurePreservesManifestLockAndInstalledTree(t *testing.T) {
	withFakeRegistryGraph(t, map[string]fakeRelease{
		"@audit/root@v1": {deps: map[string]string{"@audit/old": "v1"}, publish: t0},
		"@audit/root@v2": {publish: t0}, "@audit/old@v1": {publish: t0},
	})
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{}, "")
	chdir(t, dir)
	if err := Add(context.Background(), AddOptions{Spec: "@audit/root@v1"}); err != nil {
		t.Fatal(err)
	}
	beforeManifest := read(t, filepath.Join(dir, "gdam.json"))
	beforeLock := read(t, filepath.Join(dir, "gdam.lock"))
	beforeAddon := read(t, filepath.Join(dir, "addons/@audit_root/plugin.cfg"))
	prepare := preparePackageRoot
	preparePackageRoot = func(context.Context, *githubapi.Client, gdamdb.ResolvedAddon, string) (string, error) {
		return "", errors.New("download failed")
	}
	err := Add(context.Background(), AddOptions{Spec: "@audit/root@v2"})
	preparePackageRoot = prepare
	if err == nil {
		t.Fatal("failed download accepted")
	}
	if read(t, filepath.Join(dir, "gdam.json")) != beforeManifest || read(t, filepath.Join(dir, "gdam.lock")) != beforeLock || read(t, filepath.Join(dir, "addons/@audit_root/plugin.cfg")) != beforeAddon {
		t.Fatal("failed update changed project")
	}
	if err = Add(context.Background(), AddOptions{Spec: "@audit/root@v2"}); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(dir, "addons/@audit_old")) {
		t.Fatal("retry retained obsolete dependency")
	}
}

func TestMutationRollsBackWriteFailureAndSerializesWriters(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{}, "")
	chdir(t, dir)
	before := read(t, filepath.Join(dir, "gdam.json"))
	err := withProjectMutation(context.Background(), func(context.Context) error {
		if nested := Install(context.Background(), InstallOptions{}); nested == nil {
			t.Fatal("concurrent writer accepted")
		}
		if e := os.WriteFile(filepath.Join(dir, "gdam.json"), []byte("partial"), 0600); e != nil {
			return e
		}
		if e := os.MkdirAll(filepath.Join(dir, "addons/new"), 0755); e != nil {
			return e
		}
		return errors.New("copy failed")
	})
	if err == nil || read(t, filepath.Join(dir, "gdam.json")) != before || exists(filepath.Join(dir, "addons")) {
		t.Fatal("partial write was not rolled back")
	}
}

func TestInterruptedMutationRecoversBeforeReadingManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{}, "")
	if err := os.MkdirAll(filepath.Join(dir, "addons/unmanaged"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "addons/unmanaged/keep"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	before := read(t, filepath.Join(dir, "gdam.json"))
	cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptedMutationHelper$")
	cmd.Env = append(os.Environ(), "GDAM_INTERRUPTION_FIXTURE="+dir)
	output, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 37 {
		t.Fatalf("helper: %v %s", err, output)
	}
	chdir(t, dir)
	if err = Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(dir, "gdam.json")) != before || read(t, filepath.Join(dir, "addons/unmanaged/keep")) != "keep" || exists(filepath.Join(dir, transactionDir)) {
		t.Fatal("interrupted update did not recover")
	}
}
func TestInterruptedMutationHelper(t *testing.T) {
	dir := os.Getenv("GDAM_INTERRUPTION_FIXTURE")
	if dir == "" {
		return
	}
	if err := os.Chdir(dir); err != nil {
		os.Exit(4)
	}
	_ = withProjectMutation(context.Background(), func(context.Context) error {
		_ = os.WriteFile("gdam.json", []byte("interrupted"), 0600)
		_ = os.RemoveAll("addons")
		os.Exit(37)
		return nil
	})
	os.Exit(5)
}

func TestPruningRejectsTraversalDuplicateAndSymlinkParents(t *testing.T) {
	for _, scenario := range []string{"traversal", "duplicate", "wrong-leaf", "symlink-parent"} {
		t.Run(scenario, func(t *testing.T) {
			base := t.TempDir()
			dir := filepath.Join(base, "project")
			outside := filepath.Join(base, "outside")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(outside, 0755); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(outside, "keep")
			if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			writeManifest(t, dir, map[string]string{}, "")
			chdir(t, dir)
			p := "addons/@audit_old"
			if scenario == "traversal" {
				p = "addons/../../outside"
			}
			if scenario == "wrong-leaf" {
				p = "addons/@audit_unmanaged"
			}
			if scenario == "symlink-parent" {
				if err := os.Symlink(outside, filepath.Join(dir, "addons")); err != nil {
					t.Skip("directory symlink privilege is unavailable")
				}
			}
			installs := []lockfile.Install{{Path: p, RequiredBy: []string{"gdam.json"}}}
			if scenario == "duplicate" {
				installs = append(installs, installs[0])
			}
			old := lockfile.New()
			old.Requirements["@audit/old"] = "v1"
			old.Addons["@audit/old@v1"] = &lockfile.Release{Name: "@audit/old", Tag: "v1", GitHubOwner: "audit", GitHubRepo: "old", GitHubReleaseID: 1, CommitSHA: strings.Repeat("c", 40), AssetID: 2, AssetName: "old.zip", AssetDigest: "sha256:" + strings.Repeat("0", 64), Installs: installs}
			if err := lockfile.Save(filepath.Join(dir, "gdam.lock"), old); err != nil {
				t.Fatal(err)
			}
			manifestBefore := read(t, filepath.Join(dir, "gdam.json"))
			lockBefore := read(t, filepath.Join(dir, "gdam.lock"))
			if err := Install(context.Background(), InstallOptions{}); err == nil {
				t.Fatal("unsafe destination accepted")
			}
			if read(t, marker) != "keep" || read(t, filepath.Join(dir, "gdam.json")) != manifestBefore || read(t, filepath.Join(dir, "gdam.lock")) != lockBefore {
				t.Fatal("refusal mutated project or outside files")
			}
		})
	}
}

func TestPackageDependencyOutputRejectsSymlinkParent(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "addon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "addon/.gdam")); err != nil {
		t.Skip("symlink privilege unavailable")
	}
	writeManifest(t, dir, map[string]string{}, "addon")
	chdir(t, dir)
	if err := Install(context.Background(), InstallOptions{}); err == nil {
		t.Fatal("linked dependency output accepted")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("wrote outside project")
	}
}

func TestRecoveryCanResumeAfterAnAlreadyRestoredEntry(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{}, "")
	chdir(t, dir)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.MkdirAll(transactionDir+"/backup", 0700); err != nil {
		t.Fatal(err)
	}
	digest, err := treeDigest(root, "gdam.json")
	if err != nil {
		t.Fatal(err)
	}
	j := mutationJournal{Version: 1, State: "prepared", Entries: []snapshotEntry{{Path: "gdam.json", Present: true, Digest: digest}}}
	if err = saveJournal(root, j); err != nil {
		t.Fatal(err)
	}
	if err = recoverMutation(root); err != nil {
		t.Fatal(err)
	}
	if _, err = manifest.Load(filepath.Join(dir, "gdam.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryFindsProjectWithMissingManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{}, "")
	chdir(t, dir)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = root.MkdirAll(transactionDir+"/backup", 0700); err != nil {
		t.Fatal(err)
	}
	if err = copySnapshot(root, "gdam.json", transactionDir+"/backup/0"); err != nil {
		t.Fatal(err)
	}
	digest, err := treeDigest(root, transactionDir+"/backup/0")
	if err != nil {
		t.Fatal(err)
	}
	if err = saveJournal(root, mutationJournal{Version: 1, State: "prepared", Entries: []snapshotEntry{{Path: "gdam.json", Present: true, Digest: digest}}}); err != nil {
		t.Fatal(err)
	}
	if err = root.Remove("gdam.json"); err != nil {
		t.Fatal(err)
	}
	if err = Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err = manifest.Load(filepath.Join(dir, "gdam.json")); err != nil {
		t.Fatal(err)
	}
}

func TestExternalDirectoryLinkSurvivesFailedMutation(t *testing.T) {
	dir := t.TempDir()
	addon := t.TempDir()
	if err := os.WriteFile(filepath.Join(addon, "plugin.cfg"), []byte("[plugin]\nname=\"fixture\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, map[string]string{}, "")
	chdir(t, dir)
	if err := Link(context.Background(), LinkOptions{Spec: "@audit/linked", Path: addon}); err != nil {
		t.Fatal(err)
	}
	err := withProjectMutation(context.Background(), func(context.Context) error {
		if e := removeProjectPath(dir, filepath.Join(dir, "addons/@audit_linked")); e != nil {
			return e
		}
		return errors.New("synthetic failure after link removal")
	})
	if err == nil {
		t.Fatal("failure accepted")
	}
	ok, err := pluginCfgExistsAtDirRoot(filepath.Join(dir, "addons/@audit_linked"))
	if err != nil || !ok {
		t.Fatalf("directory link did not recover: %v", err)
	}
	if read(t, filepath.Join(addon, "plugin.cfg")) != "[plugin]\nname=\"fixture\"\n" {
		t.Fatal("external source was changed")
	}
}

func TestRecoverySnapshotPreservesPermissions(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{}, "")
	chdir(t, dir)
	file := filepath.Join(dir, "project.godot")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0666); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	err = withProjectMutation(context.Background(), func(context.Context) error {
		if e := os.WriteFile(file, []byte("changed"), 0600); e != nil {
			return e
		}
		return errors.New("synthetic write failure")
	})
	after, statErr := os.Stat(file)
	if err == nil || statErr != nil || before.Mode().Perm() != after.Mode().Perm() || read(t, file) != "original" {
		t.Fatalf("recovery changed permissions: %v %v", err, statErr)
	}
}

func TestFrozenInstallPreservesManifestAndLockBytes(t *testing.T) {
	withFakeRegistryGraph(t, map[string]fakeRelease{"@audit/root@v1": {publish: t0}})
	dir := t.TempDir()
	writeManifest(t, dir, map[string]string{"@audit/root": "v1"}, "")
	chdir(t, dir)
	if err := Install(context.Background(), InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for _, name := range []string{"gdam.json", "gdam.lock"} {
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(read(t, filepath.Join(dir, name)))); err != nil {
			t.Fatal(err)
		}
		value := compact.String() + "\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		before[name] = value
	}
	if err := Install(context.Background(), InstallOptions{FrozenLockfile: true}); err != nil {
		t.Fatal(err)
	}
	for name, value := range before {
		if read(t, filepath.Join(dir, name)) != value {
			t.Errorf("frozen install rewrote %s", name)
		}
	}
}
