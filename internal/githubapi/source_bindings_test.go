package githubapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewedReleaseSourcesAreExactAndScoped(t *testing.T) {
	records, err := loadReviewedSources(reviewedReleaseSources)
	if err != nil || len(records) == 0 {
		t.Fatalf("missing valid reviewed source bindings: %v", err)
	}
	for key, source := range records {
		owner, repo, _ := strings.Cut(key.repository, "/")
		if !hasReviewedReleaseSource(owner, repo, key.tag, key.publisher, source) {
			t.Fatal("reviewed binding rejected")
		}
		for _, value := range [][5]string{
			{"unreviewed", repo, key.tag, key.publisher, source},
			{owner, repo, key.tag + "-other", key.publisher, source},
			{owner, repo, key.tag, strings.Repeat("0", 40), source},
			{owner, repo, key.tag, key.publisher, strings.Repeat("0", 40)},
		} {
			if hasReviewedReleaseSource(value[0], value[1], value[2], value[3], value[4]) {
				t.Fatal("unreviewed binding accepted")
			}
		}
		break
	}
}

func TestReviewedSourceLoaderRejectsMalformedBindings(t *testing.T) {
	record := reviewedReleaseSource{"dev/addon", "v1", strings.Repeat("a", 40), strings.Repeat("b", 40)}
	valid, _ := json.Marshal([]reviewedReleaseSource{record})
	duplicate, _ := json.Marshal([]reviewedReleaseSource{record, record})
	for _, raw := range [][]byte{duplicate, append(append([]byte{}, valid...), []byte(" {}")...), []byte(`[{"repository":"dev/addon","extra":true}]`)} {
		if _, err := loadReviewedSources(raw); err == nil {
			t.Fatal("invalid source bindings accepted")
		}
	}
	for _, edit := range []func(*reviewedReleaseSource){
		func(r *reviewedReleaseSource) { r.Repository = "../addon" },
		func(r *reviewedReleaseSource) { r.Tag = "" },
		func(r *reviewedReleaseSource) { r.PublisherCommit = "short" },
		func(r *reviewedReleaseSource) { r.SourceCommit = r.PublisherCommit },
	} {
		changed := record
		edit(&changed)
		raw, _ := json.Marshal([]reviewedReleaseSource{changed})
		if _, err := loadReviewedSources(raw); err == nil {
			t.Fatal("invalid identity accepted")
		}
	}
}

func TestBoundReleaseStillVerifiesOriginalAssetIdentity(t *testing.T) {
	original := reviewedSourceCommits
	t.Cleanup(func() { reviewedSourceCommits = original })
	reviewedSourceCommits = map[releaseSourceKey]string{{"dev/addon", "Release-1", strings.Repeat("a", 40)}: strings.Repeat("b", 40)}
	for _, changedAsset := range []bool{false, true} {
		id, server := testIdentityServer(t, []byte("verified archive"), func(r *ReleaseIdentity) {
			r.CommitSHA = strings.Repeat("b", 40)
			if changedAsset {
				r.Digest = "sha256:" + strings.Repeat("0", 64)
			}
		})
		client := NewClient("")
		client.apiBaseURL = server.URL
		err := client.DownloadVerifiedReleaseAsset(context.Background(), "dev", "addon", id, filepath.Join(t.TempDir(), "asset.zip"))
		server.Close()
		if changedAsset && (err == nil || !strings.Contains(err.Error(), "asset drift")) {
			t.Fatalf("changed asset accepted: %v", err)
		}
		if !changedAsset && err != nil {
			t.Fatalf("reviewed source failed: %v", err)
		}
	}
}
