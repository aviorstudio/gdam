package commands

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/aviorstudio/gdam/internal/lockfile"
	"github.com/aviorstudio/gdam/internal/manifest"
)

const stateDir = ".gdam-state"
const transactionDir = stateDir + "/transaction"

type snapshotEntry struct {
	Path    string `json:"path"`
	Present bool   `json:"present"`
	Digest  string `json:"digest,omitempty"`
}
type mutationJournal struct {
	Version int             `json:"version"`
	State   string          `json:"state"`
	Entries []snapshotEntry `json:"entries"`
}

func withProjectMutation(ctx context.Context, action func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, ok := findMutationProject(cwd)
	if !ok {
		return fmt.Errorf("%w: no gdam.json found (run `gdam init`)", ErrUserInput)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if info, e := root.Lstat(stateDir); e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0 || runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return errors.New("gdam state directory must be a private regular directory")
	}
	if err = root.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	if info, e := root.Lstat(stateDir + "/.gitignore"); e == nil && !info.Mode().IsRegular() {
		return errors.New("invalid gdam state ignore file")
	}
	if err = root.WriteFile(stateDir+"/.gitignore", []byte("*\n"), 0600); err != nil {
		return err
	}
	if info, e := root.Lstat(stateDir + "/operation.lock"); e == nil && !info.Mode().IsRegular() {
		return errors.New("invalid gdam operation lock")
	}
	guard, err := root.OpenFile(stateDir+"/operation.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer guard.Close()
	if err = lockMutation(guard); err != nil {
		return fmt.Errorf("another gdam operation may be running: %w", err)
	}
	if err = recoverMutation(root); err != nil {
		return fmt.Errorf("recovering interrupted gdam operation: %w", err)
	}
	paths, err := mutationPaths(root, dir)
	if err != nil {
		return err
	}
	if err = root.Mkdir(transactionDir, 0700); err != nil {
		return err
	}
	if err = root.Mkdir(transactionDir+"/backup", 0700); err != nil {
		return err
	}
	journal := mutationJournal{Version: 1, State: "preparing"}
	if err = saveJournal(root, journal); err != nil {
		return err
	}
	for i, path := range paths {
		entry := snapshotEntry{Path: path}
		if _, e := root.Lstat(path); e == nil {
			entry.Present = true
			dest := fmt.Sprintf("%s/backup/%d", transactionDir, i)
			if err = copySnapshot(root, path, dest); err != nil {
				return err
			}
			if entry.Digest, err = treeDigest(root, dest); err != nil {
				return err
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		journal.Entries = append(journal.Entries, entry)
	}
	journal.State = "prepared"
	if err = saveJournal(root, journal); err != nil {
		return err
	}
	if err = action(ctx); err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if recoveryErr := recoverMutation(root); recoveryErr != nil {
			return errors.Join(err, fmt.Errorf("previous state retained for recovery: %w", recoveryErr))
		}
		return err
	}
	journal.State = "committed"
	if err = saveJournal(root, journal); err != nil {
		return errors.Join(err, recoverMutation(root))
	}
	// A committed journal is safe to clean again on the next invocation.
	if err = root.RemoveAll(transactionDir); err != nil {
		fmt.Fprintln(os.Stderr, "gdam: update committed; temporary backup cleanup will retry on the next operation")
	}
	return nil
}

func mutationPaths(root *os.Root, dir string) ([]string, error) {
	paths := []string{"gdam.json", manifest.LinkFilename, lockfile.Filename, "project.godot", "addons"}
	for _, p := range paths {
		info, err := root.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || p != "addons" && !info.Mode().IsRegular() || p == "addons" && !info.IsDir() {
			return nil, fmt.Errorf("refusing nonregular project path %s", p)
		}
	}
	m, err := manifest.Load(filepath.Join(dir, "gdam.json"))
	if err != nil {
		return nil, err
	}
	if m.Package != "" {
		if !fs.ValidPath(m.Package) || strings.Contains(m.Package, "\\") || m.Package == "." || strings.Split(m.Package, "/")[0] == stateDir {
			return nil, errors.New("package must name a directory inside the project")
		}
		p := m.Package + "/.gdam"
		if err = checkParents(root, p+"/deps.gd"); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(p, "addons/") {
			paths = append(paths, p)
		}
	}
	lock, err := lockfile.Load(filepath.Join(dir, lockfile.Filename))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, p := range lock.Paths() {
		if err = checkParents(root, p); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func checkParents(root *os.Root, p string) error {
	parts := strings.Split(filepath.ToSlash(p), "/")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[:i], "/")
		info, err := root.Lstat(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing non-directory or linked destination parent %s", parent)
		}
	}
	return nil
}

func saveJournal(root *os.Root, j mutationJournal) error {
	body, err := json.Marshal(j)
	if err != nil {
		return err
	}
	temporary := transactionDir + "/journal.tmp"
	if err = root.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(body)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = root.Rename(temporary, transactionDir+"/journal.json"); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		d, e := root.Open(transactionDir)
		if e != nil {
			return e
		}
		defer d.Close()
		return d.Sync()
	}
	return nil
}

func recoverMutation(root *os.Root) error {
	if info, err := root.Lstat(transactionDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("invalid transaction directory")
	}
	file, err := root.Open(transactionDir + "/journal.json")
	if errors.Is(err, os.ErrNotExist) {
		return root.RemoveAll(transactionDir)
	}
	if err != nil {
		return err
	}
	var j mutationJournal
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&j)
	file.Close()
	if err != nil || j.Version != 1 {
		return errors.New("invalid gdam transaction journal; preserved for inspection")
	}
	if j.State == "preparing" || j.State == "committed" {
		return root.RemoveAll(transactionDir)
	}
	if j.State != "prepared" {
		return errors.New("invalid gdam transaction state")
	}
	seen := map[string]bool{}
	for _, e := range j.Entries {
		allowed := e.Path == "gdam.json" || e.Path == manifest.LinkFilename || e.Path == lockfile.Filename || e.Path == "project.godot" || e.Path == "addons" || fs.ValidPath(e.Path) && strings.HasSuffix(e.Path, "/.gdam") && !strings.Contains(e.Path, "\\") && strings.Split(e.Path, "/")[0] != stateDir
		if !allowed || seen[e.Path] || e.Present && len(e.Digest) != 64 {
			return errors.New("invalid recovery destination")
		}
		seen[e.Path] = true
		if err = checkParents(root, e.Path); err != nil {
			return err
		}
	}
	for i, e := range j.Entries {
		backup := fmt.Sprintf("%s/backup/%d", transactionDir, i)
		if e.Present {
			if _, err = root.Lstat(backup); errors.Is(err, os.ErrNotExist) {
				got, hashErr := treeDigest(root, e.Path)
				if hashErr != nil || got != e.Digest {
					return errors.New("recovery backup missing and destination differs; state preserved")
				}
				continue
			} else if err != nil {
				return err
			}
			got, hashErr := treeDigest(root, backup)
			if hashErr != nil || got != e.Digest {
				return errors.New("recovery backup differs; state preserved")
			}
		}
		if err = root.RemoveAll(e.Path); err != nil {
			return err
		}
		if e.Present {
			if err = root.MkdirAll(filepath.Dir(e.Path), 0755); err != nil {
				return err
			}
			if err = root.Rename(backup, e.Path); err != nil {
				return err
			}
		}
	}
	return root.RemoveAll(transactionDir)
}

func copySnapshot(root *os.Root, src, dst string) error {
	info, err := root.Lstat(src)
	if err != nil {
		return err
	}
	if err = root.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, e := root.Readlink(src)
		if e != nil {
			return e
		}
		return copyProjectLink(root, src, target, dst)
	}
	if info.IsDir() {
		if err = root.Mkdir(dst, info.Mode().Perm()); err != nil {
			return err
		}
		dir, e := root.Open(src)
		if e != nil {
			return e
		}
		entries, e := dir.ReadDir(-1)
		dir.Close()
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if err = copySnapshot(root, src+"/"+entry.Name(), dst+"/"+entry.Name()); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported project file %s", src)
	}
	in, err := root.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := root.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func treeDigest(root *os.Root, start string) (string, error) {
	h := sha256.New()
	var walk func(string, string) error
	walk = func(p, relative string) error {
		info, err := root.Lstat(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", relative, info.Mode())
		if info.Mode()&os.ModeSymlink != 0 {
			value, e := root.Readlink(p)
			if e != nil {
				return e
			}
			fmt.Fprint(h, value)
			return nil
		}
		if info.IsDir() {
			d, e := root.Open(p)
			if e != nil {
				return e
			}
			entries, e := d.ReadDir(-1)
			d.Close()
			if e != nil {
				return e
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, entry := range entries {
				if e = walk(p+"/"+entry.Name(), relative+"/"+entry.Name()); e != nil {
					return e
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("unsupported snapshot file")
		}
		f, e := root.Open(p)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = io.Copy(h, f)
		return e
	}
	if err := walk(start, "."); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func removeProjectPath(projectDir, dst string) error {
	relative, err := filepath.Rel(projectDir, dst)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = checkParents(root, relative); err != nil {
		return err
	}
	return root.RemoveAll(relative)
}

func copyProjectPath(projectDir, src, dst string) error {
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return err
	}
	defer root.Close()
	relative, err := filepath.Rel(projectDir, dst)
	if err != nil {
		return err
	}
	if err = checkParents(root, relative); err != nil {
		return err
	}
	var copy func(string, string) error
	copy = func(source, target string) error {
		info, e := os.Lstat(source)
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing package symlink %s", source)
		}
		if info.IsDir() {
			if e = root.MkdirAll(target, 0755); e != nil {
				return e
			}
			entries, e := os.ReadDir(source)
			if e != nil {
				return e
			}
			for _, entry := range entries {
				if e = copy(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); e != nil {
					return e
				}
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("package contains a nonregular file")
		}
		if e = root.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return e
		}
		in, e := os.Open(source)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := root.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if e != nil {
			return e
		}
		_, e = io.Copy(out, in)
		closeErr := out.Close()
		if e != nil {
			return e
		}
		return closeErr
	}
	return copy(src, relative)
}
func linkProjectPath(projectDir, target, dst string) error {
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return err
	}
	defer root.Close()
	relative, err := filepath.Rel(projectDir, dst)
	if err != nil {
		return err
	}
	if err = checkParents(root, relative); err != nil {
		return err
	}
	return createProjectDirectoryLink(root, target, relative)
}

func findMutationProject(start string) (string, bool) {
	for dir := start; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "gdam.json")); err == nil {
			return dir, true
		}
		if _, err := os.Stat(filepath.Join(dir, transactionDir, "journal.json")); err == nil {
			return dir, true
		}
		if filepath.Dir(dir) == dir {
			return "", false
		}
	}
}

func writeProjectFile(projectDir, dst string, data []byte, mode os.FileMode) error {
	root, err := os.OpenRoot(projectDir)
	if err != nil {
		return err
	}
	defer root.Close()
	relative, err := filepath.Rel(projectDir, dst)
	if err != nil {
		return err
	}
	if err = checkParents(root, relative); err != nil {
		return err
	}
	if err = root.MkdirAll(filepath.Dir(relative), 0755); err != nil {
		return err
	}
	temporary := filepath.Join(filepath.Dir(relative), ".gdam-write-"+rand.Text())
	f, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(temporary, relative)
}
