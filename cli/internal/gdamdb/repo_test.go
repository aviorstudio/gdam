package gdamdb

import "testing"

func TestParseGitHubOwnerRepo(t *testing.T) {
	tests := []struct {
		in    string
		owner string
		repo  string
	}{
		{"github.com/aviorstudio/revik", "aviorstudio", "revik"},
		{"https://github.com/aviorstudio/revik", "aviorstudio", "revik"},
		{"https://github.com/aviorstudio/revik.git", "aviorstudio", "revik"},
		{"git@github.com:aviorstudio/revik.git", "aviorstudio", "revik"},
	}

	for _, tt := range tests {
		owner, repo, err := ParseGitHubOwnerRepo(tt.in)
		if err != nil {
			t.Fatalf("ParseGitHubOwnerRepo(%q) error: %v", tt.in, err)
		}
		if owner != tt.owner || repo != tt.repo {
			t.Fatalf("ParseGitHubOwnerRepo(%q) = %s/%s, want %s/%s", tt.in, owner, repo, tt.owner, tt.repo)
		}
	}
}

func TestParseGitHubRepoURL(t *testing.T) {
	tests := []struct {
		in     string
		owner  string
		repo   string
		subdir string
	}{
		{"github.com/aviorstudio/revik", "aviorstudio", "revik", ""},
		{"https://github.com/aviorstudio/revik/godot_core", "aviorstudio", "revik", "godot_core"},
		{"https://github.com/aviorstudio/revik/godot_core/", "aviorstudio", "revik", "godot_core"},
		{"https://github.com/aviorstudio/revik/tree/main/godot_core", "aviorstudio", "revik", "godot_core"},
		{"git@github.com:aviorstudio/revik/godot_core.git", "aviorstudio", "revik", "godot_core"},
	}

	for _, tt := range tests {
		owner, repo, subdir, err := ParseGitHubRepoURL(tt.in)
		if err != nil {
			t.Fatalf("ParseGitHubRepoURL(%q) error: %v", tt.in, err)
		}
		if owner != tt.owner || repo != tt.repo || subdir != tt.subdir {
			t.Fatalf("ParseGitHubRepoURL(%q) = %s/%s/%s, want %s/%s/%s", tt.in, owner, repo, subdir, tt.owner, tt.repo, tt.subdir)
		}
	}
}
