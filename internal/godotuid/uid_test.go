package godotuid

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTextMatchesGodotEncoding(t *testing.T) {
	// ResourceUID::id_to_text: base 34, 'a'..'y' then '0'..'8', most
	// significant digit first.
	cases := map[int64]string{1: "uid://b", 24: "uid://y", 25: "uid://0", 33: "uid://8", 34: "uid://ba", 35: "uid://bb"}
	for id, want := range cases {
		if got := Text(id); got != want {
			t.Errorf("Text(%d) = %s, want %s", id, got, want)
		}
	}
	fresh, err := New()
	if err != nil || !regexp.MustCompile(`^uid://[a-y0-8]{1,13}$`).MatchString(fresh) {
		t.Fatalf("New() = %q, %v", fresh, err)
	}
}

func TestRegenerateRewritesEveryReference(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/a.gd.uid", "uid://b0cjekr1r63dq\n")
	write("src/b.gd.uid", "uid://cabcdefghijkl\n")
	write("src/a.gd", "extends Node\n")
	write("scene.tscn", `[ext_resource type="Script" uid="uid://b0cjekr1r63dq" path="res://addons/x/src/a.gd" id="1"]`+"\n"+`[ext_resource uid="uid://cabcdefghijkl"]`+"\n")
	write("notes.txt", "uid://b0cjekr1r63dq stays, this is not a Godot text file\n")

	n, err := Regenerate(dir)
	if err != nil || n != 2 {
		t.Fatalf("Regenerate: %d %v", n, err)
	}
	a, _ := os.ReadFile(filepath.Join(dir, "src/a.gd.uid"))
	b, _ := os.ReadFile(filepath.Join(dir, "src/b.gd.uid"))
	newA, newB := strings.TrimSpace(string(a)), strings.TrimSpace(string(b))
	if newA == "uid://b0cjekr1r63dq" || newB == "uid://cabcdefghijkl" || newA == newB {
		t.Fatalf("uids not regenerated: %s %s", newA, newB)
	}
	scene, _ := os.ReadFile(filepath.Join(dir, "scene.tscn"))
	if !strings.Contains(string(scene), newA) || !strings.Contains(string(scene), newB) || strings.Contains(string(scene), "b0cjekr1r63dq") {
		t.Fatalf("scene not rewritten: %s", scene)
	}
	notes, _ := os.ReadFile(filepath.Join(dir, "notes.txt"))
	if !strings.Contains(string(notes), "uid://b0cjekr1r63dq") {
		t.Fatalf("non-Godot file touched: %s", notes)
	}
}
