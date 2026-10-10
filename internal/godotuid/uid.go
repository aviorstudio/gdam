// Package godotuid generates and rewrites Godot resource UIDs.
//
// Every script and scene an addon ships has a sidecar .uid file, and scenes
// reference scripts by that uid. Godot's uid cache maps each uid to exactly
// one path, so a second copy of an addon in the same project (a nested
// dependency) must carry fresh uids, rewritten consistently through the
// copy's own files, or the editor warns and remaps on every open.
package godotuid

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Godot's ResourceUID text form: base 34 over 'a'..'y' then '0'..'8'.
const (
	charCount = 'z' - 'a' // 25
	base      = charCount + ('9' - '0')
)

var textRe = regexp.MustCompile(`uid://[a-y0-8]+`)

// Text encodes an id the way ResourceUID::id_to_text does.
func Text(id int64) string {
	if id <= 0 {
		return "uid://<invalid>"
	}
	var out []byte
	for id > 0 {
		c := id % base
		if c < charCount {
			out = append([]byte{byte('a' + c)}, out...)
		} else {
			out = append([]byte{byte('0' + (c - charCount))}, out...)
		}
		id /= base
	}
	return "uid://" + string(out)
}

// New returns a fresh random uid in text form.
func New() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	id := int64(binary.BigEndian.Uint64(buf[:]) & (1<<63 - 1))
	if id == 0 {
		id = 1
	}
	return Text(id), nil
}

// textFileExts are the files that may reference a uid by text.
var textFileExts = map[string]bool{".uid": true, ".tscn": true, ".tres": true, ".gd": true, ".import": true, ".cfg": true, ".godot": true, ".tool": true}

// Regenerate gives every .uid file under dir a fresh uid and rewrites each
// old uid wherever it appears in the directory's text files. It returns the
// number of uids replaced.
func Regenerate(dir string) (int, error) {
	replacements := map[string]string{}
	var uidFiles []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".uid") {
			return nil
		}
		uidFiles = append(uidFiles, p)
		return nil
	})
	if err != nil {
		return 0, err
	}
	for _, p := range uidFiles {
		raw, err := os.ReadFile(p)
		if err != nil {
			return 0, err
		}
		old := strings.TrimSpace(string(raw))
		if !textRe.MatchString(old) || textRe.FindString(old) != old {
			return 0, fmt.Errorf("%s does not hold a uid", p)
		}
		if _, done := replacements[old]; done {
			continue
		}
		fresh, err := New()
		if err != nil {
			return 0, err
		}
		replacements[old] = fresh
	}
	if len(replacements) == 0 {
		return 0, nil
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !textFileExts[filepath.Ext(p)] {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		text := string(raw)
		rewritten := textRe.ReplaceAllStringFunc(text, func(found string) string {
			if fresh, ok := replacements[found]; ok {
				return fresh
			}
			return found
		})
		if rewritten == text {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(p, []byte(rewritten), info.Mode().Perm())
	})
	if err != nil {
		return 0, err
	}
	return len(replacements), nil
}
