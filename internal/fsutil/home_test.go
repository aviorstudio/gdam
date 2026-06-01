package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExpandHome(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := ExpandHome("~")
	if err != nil {
		t.Fatalf("ExpandHome(~): %v", err)
	}
	if got != home {
		t.Fatalf("ExpandHome(~) = %q, want %q", got, home)
	}

	got, err = ExpandHome("~/dev/plugin")
	if err != nil {
		t.Fatalf("ExpandHome(~/...): %v", err)
	}
	want := filepath.Join(home, "dev", "plugin")
	if got != want {
		t.Fatalf("ExpandHome(~/...) = %q, want %q", got, want)
	}

	// Windows also accepts "~\\...".
	if runtime.GOOS == "windows" {
		got, err = ExpandHome("~\\dev\\plugin")
		if err != nil {
			t.Fatalf("ExpandHome(~\\\\...): %v", err)
		}
		if got != want {
			t.Fatalf("ExpandHome(~\\\\...) = %q, want %q", got, want)
		}
	}
}

func TestAbbrevHome(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	p := filepath.Join(home, "dev", "plugin")
	got, err := AbbrevHome(p)
	if err != nil {
		t.Fatalf("AbbrevHome: %v", err)
	}
	want := "~" + string(os.PathSeparator) + filepath.Join("dev", "plugin")
	if got != want {
		t.Fatalf("AbbrevHome = %q, want %q", got, want)
	}
}
