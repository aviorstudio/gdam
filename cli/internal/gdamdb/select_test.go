package gdamdb

import "testing"

func TestSelectVersionRequested(t *testing.T) {
	rows := []versionRow{
		{Major: 0, Minor: 1, Patch: 0, SHA: "aaa"},
		{Major: 0, Minor: 2, Patch: 0, SHA: "bbb"},
	}

	got, ok := selectVersion(rows, "0.2.0")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.SHA != "bbb" {
		t.Fatalf("expected sha=bbb, got %q", got.SHA)
	}
}

func TestSelectVersionLatestSemver(t *testing.T) {
	rows := []versionRow{
		{Major: 0, Minor: 1, Patch: 0, SHA: "aaa"},
		{Major: 0, Minor: 2, Patch: 0, SHA: "bbb"},
		{Major: 0, Minor: 10, Patch: 0, SHA: "ccc"},
	}

	got, ok := selectVersion(rows, "")
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if got.SHA != "ccc" {
		t.Fatalf("expected sha=ccc, got %q", got.SHA)
	}
}
