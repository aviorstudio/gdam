package semver

import "testing"

func TestBestTag(t *testing.T) {
	tags := []string{
		"v1.2.0",
		"v1.10.0",
		"v2.0.0-alpha.1",
		"v2.0.0",
		"not-a-version",
	}

	got, ok := BestTag(tags)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got != "v2.0.0" {
		t.Fatalf("expected v2.0.0, got %q", got)
	}
}

func TestComparePrerelease(t *testing.T) {
	a, ok := Parse("v1.0.0-alpha")
	if !ok {
		t.Fatalf("parse failed")
	}
	b, ok := Parse("v1.0.0-alpha.1")
	if !ok {
		t.Fatalf("parse failed")
	}
	if Compare(a, b) >= 0 {
		t.Fatalf("expected alpha < alpha.1")
	}
}
