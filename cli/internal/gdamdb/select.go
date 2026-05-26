package gdamdb

import (
	"strings"

	"github.com/aviorstudio/gdam/cli/internal/semver"
)

func selectVersion(rows []releaseRow, requested string) (releaseRow, bool) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		reqVer, ok := semver.Parse(requested)
		if !ok || len(reqVer.Pre) > 0 {
			return releaseRow{}, false
		}
		for _, row := range rows {
			if strings.TrimSpace(row.ReleaseTag) == "" {
				continue
			}
			if row.Major == reqVer.Major && row.Minor == reqVer.Minor && row.Patch == reqVer.Patch {
				return row, true
			}
		}
		return releaseRow{}, false
	}

	var best releaseRow
	var bestSet bool

	for _, row := range rows {
		if strings.TrimSpace(row.ReleaseTag) == "" {
			continue
		}
		if row.Major < 0 || row.Minor < 0 || row.Patch < 0 {
			continue
		}
		if !bestSet || compareVersion(row, best) > 0 {
			best = row
			bestSet = true
		}
	}
	if bestSet {
		return best, true
	}

	for _, row := range rows {
		if strings.TrimSpace(row.ReleaseTag) == "" {
			continue
		}
		return row, true
	}

	return releaseRow{}, false
}

func compareVersion(a, b releaseRow) int {
	if a.Major != b.Major {
		return cmpInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmpInt(a.Minor, b.Minor)
	}
	return cmpInt(a.Patch, b.Patch)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
