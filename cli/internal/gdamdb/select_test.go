package gdamdb

import "testing"

func TestSelectVersionRequested(t *testing.T) {
	rows := []releaseRow{
		{Major: 0, Minor: 1, Patch: 0, ReleaseTag: "v0.1.0"},
		{Major: 0, Minor: 2, Patch: 0, ReleaseTag: "release-0.2.0"},
	}

	got, ok := selectVersion(rows, "0.2.0")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.ReleaseTag != "release-0.2.0" {
		t.Fatalf("expected release tag release-0.2.0, got %q", got.ReleaseTag)
	}
}

func TestSelectVersionLatestSemver(t *testing.T) {
	rows := []releaseRow{
		{Major: 0, Minor: 1, Patch: 0, ReleaseTag: "v0.1.0"},
		{Major: 0, Minor: 2, Patch: 0, ReleaseTag: "v0.2.0"},
		{Major: 0, Minor: 10, Patch: 0, ReleaseTag: "stable-0.10.0"},
	}

	got, ok := selectVersion(rows, "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.ReleaseTag != "stable-0.10.0" {
		t.Fatalf("expected release tag stable-0.10.0, got %q", got.ReleaseTag)
	}
}
