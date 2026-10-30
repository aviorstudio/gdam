package githubapi

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// The reviewed history cleanup preserved every non-Markdown file at these
// published tags. Publisher commits remain the original release provenance;
// source commits identify their cleaned public histories. Asset IDs, digests,
// release IDs, publication timestamps and tag names are still checked normally.
//
//go:embed reviewed_release_sources.json
var reviewedReleaseSources []byte

type reviewedReleaseSource struct {
	Repository      string `json:"repository"`
	Tag             string `json:"tag"`
	PublisherCommit string `json:"publisher_commit"`
	SourceCommit    string `json:"source_commit"`
}

type releaseSourceKey struct{ repository, tag, publisher string }

var reviewedSourceCommits = mustLoadReviewedSources(reviewedReleaseSources)

func loadReviewedSources(raw []byte) (map[releaseSourceKey]string, error) {
	var records []reviewedReleaseSource
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&records); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("trailing source binding data")
	}
	repositoryPattern := regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*/[a-z0-9][a-z0-9_.-]*$`)
	commitPattern := regexp.MustCompile(`^[a-f0-9]{40}$`)
	result := make(map[releaseSourceKey]string, len(records))
	for _, record := range records {
		if !repositoryPattern.MatchString(record.Repository) || record.Tag == "" || strings.TrimSpace(record.Tag) != record.Tag ||
			!commitPattern.MatchString(record.PublisherCommit) || !commitPattern.MatchString(record.SourceCommit) || record.PublisherCommit == record.SourceCommit {
			return nil, fmt.Errorf("invalid reviewed release source")
		}
		key := releaseSourceKey{record.Repository, record.Tag, record.PublisherCommit}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate reviewed release source")
		}
		result[key] = record.SourceCommit
	}
	return result, nil
}

func mustLoadReviewedSources(raw []byte) map[releaseSourceKey]string {
	result, err := loadReviewedSources(raw)
	if err != nil {
		panic(err)
	}
	return result
}

func hasReviewedReleaseSource(owner, repo, tag, publisher, source string) bool {
	expected, ok := reviewedSourceCommits[releaseSourceKey{strings.ToLower(owner + "/" + repo), tag, strings.ToLower(publisher)}]
	return ok && strings.EqualFold(expected, source)
}
