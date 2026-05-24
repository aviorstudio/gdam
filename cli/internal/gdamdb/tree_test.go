package gdamdb

import "testing"

func TestParseGitHubTreeURL(t *testing.T) {
	owner, repo, ref, err := ParseGitHubTreeURL("https://github.com/aviorstudio/revik/tree/abc123")
	if err != nil {
		t.Fatalf("ParseGitHubTreeURL: %v", err)
	}
	if owner != "aviorstudio" || repo != "revik" || ref != "abc123" {
		t.Fatalf("unexpected result: %s/%s@%s", owner, repo, ref)
	}
}

func TestParseGitHubTreeURLWithPath(t *testing.T) {
	owner, repo, ref, repoPath, err := ParseGitHubTreeURLWithPath("https://github.com/aviorstudio/revik/tree/abc1234/godot_core")
	if err != nil {
		t.Fatalf("ParseGitHubTreeURLWithPath: %v", err)
	}
	if owner != "aviorstudio" || repo != "revik" || ref != "abc1234" || repoPath != "godot_core" {
		t.Fatalf("unexpected result: %s/%s@%s (%s)", owner, repo, ref, repoPath)
	}
}

func TestParseGitHubTreeURLWithPath_AllowsBranchRefWithPath(t *testing.T) {
	owner, repo, ref, repoPath, err := ParseGitHubTreeURLWithPath("https://github.com/aviorstudio/gd-playwright/tree/main/gd-playwright-emitter")
	if err != nil {
		t.Fatalf("ParseGitHubTreeURLWithPath: %v", err)
	}
	if owner != "aviorstudio" || repo != "gd-playwright" || ref != "main" || repoPath != "gd-playwright-emitter" {
		t.Fatalf("unexpected result: %s/%s@%s (%s)", owner, repo, ref, repoPath)
	}
}

func TestParseGitHubTreeURLWithPath_AllowsEscapedRefWithSlash(t *testing.T) {
	u := GitHubTreeURLWithPath("aviorstudio", "revik", "feature/foo", "godot_core")
	owner, repo, ref, repoPath, err := ParseGitHubTreeURLWithPath(u)
	if err != nil {
		t.Fatalf("ParseGitHubTreeURLWithPath: %v", err)
	}
	if owner != "aviorstudio" || repo != "revik" || ref != "feature/foo" || repoPath != "godot_core" {
		t.Fatalf("unexpected result: %s/%s@%s (%s)", owner, repo, ref, repoPath)
	}
}
