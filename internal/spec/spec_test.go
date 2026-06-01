package spec

import "testing"

func TestParsePackageSpec(t *testing.T) {
	got, err := ParsePackageSpec("@my-user/my-package@1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Owner != "my-user" || got.Repo != "my-package" || got.Version != "1.2.3" {
		t.Fatalf("unexpected parsed spec: %#v", got)
	}
	if got.Name() != "@my-user/my-package" {
		t.Fatalf("unexpected name: %s", got.Name())
	}
}

func TestParsePackageSpecNoVersion(t *testing.T) {
	got, err := ParsePackageSpec("@my-user/my-package")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version != "" {
		t.Fatalf("expected empty version, got %q", got.Version)
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
	} {
		if _, err := ParsePackageSpec(input); err == nil {
			t.Fatalf("expected error for %q", input)
		}
	}
}
