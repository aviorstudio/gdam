package spec

import "testing"

func TestParsePackageSpec(t *testing.T) {
	got, err := ParsePackageSpec("@my-user/my-package@1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Owner != "my-user" || got.Repo != "my-package" || got.Tag != "1.2.3" {
		t.Fatalf("unexpected parsed spec: %#v", got)
	}
	if got.Name() != "@my-user/my-package" {
		t.Fatalf("unexpected name: %s", got.Name())
	}
}

func TestParsePackageSpecNoTag(t *testing.T) {
	got, err := ParsePackageSpec("@my-user/my-package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Tag != "" {
		t.Fatalf("expected empty tag, got %q", got.Tag)
	}
}

func TestParsePackageSpecInvalid(t *testing.T) {
	for _, input := range []string{
		"",
		"my-user/my-package",
		"@my-user",
		"@my-user/",
		"@/my-package",
		"@my-user/my-package@1@2",
		"@my-user/my-package@",
	} {
		if _, err := ParsePackageSpec(input); err == nil {
			t.Fatalf("expected error for %q", input)
		}
	}
}

func TestParsePackageSpecPreservesExactTags(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "1.2.3", "Release-Candidate.1", "deadbeef", "feature/test", "release+build", "release_1"} {
		got, err := ParsePackageSpec("@Owner/addon@" + tag)
		if err != nil {
			t.Fatalf("tag %q: %v", tag, err)
		}
		if got.Tag != tag {
			t.Fatalf("tag %q became %q", tag, got.Tag)
		}
	}
}
