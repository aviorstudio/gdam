package semver

import (
	"strconv"
	"strings"
)

type Version struct {
	Major int
	Minor int
	Patch int
	Pre   []identifier
}

type identifier struct {
	raw     string
	numeric bool
	num     int
}

func BestTag(tags []string) (string, bool) {
	var bestTag string
	var bestVer Version
	var bestSet bool

	for _, t := range tags {
		v, ok := Parse(t)
		if !ok {
			continue
		}
		if !bestSet || Compare(v, bestVer) > 0 {
			bestTag = t
			bestVer = v
			bestSet = true
		}
	}
	return bestTag, bestSet
}

func Parse(s string) (Version, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, false
	}
	if strings.HasPrefix(s, "v") {
		s = s[1:]
	}

	core := s
	pre := ""
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core = s[:i]
		pre = s[i+1:]
	}
	if i := strings.IndexByte(core, '+'); i >= 0 {
		core = core[:i]
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	maj, ok := parseInt(parts[0])
	if !ok {
		return Version{}, false
	}
	min, ok := parseInt(parts[1])
	if !ok {
		return Version{}, false
	}
	pat, ok := parseInt(parts[2])
	if !ok {
		return Version{}, false
	}

	var ids []identifier
	if pre != "" {
		for _, part := range strings.Split(pre, ".") {
			id := identifier{raw: part}
			if n, ok := parseInt(part); ok {
				id.numeric = true
				id.num = n
			}
			ids = append(ids, id)
		}
	}

	return Version{
		Major: maj,
		Minor: min,
		Patch: pat,
		Pre:   ids,
	}, true
}

func Compare(a, b Version) int {
	if a.Major != b.Major {
		return cmpInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmpInt(a.Minor, b.Minor)
	}
	if a.Patch != b.Patch {
		return cmpInt(a.Patch, b.Patch)
	}

	aPre := len(a.Pre) > 0
	bPre := len(b.Pre) > 0
	if !aPre && !bPre {
		return 0
	}
	if !aPre && bPre {
		return 1
	}
	if aPre && !bPre {
		return -1
	}

	max := len(a.Pre)
	if len(b.Pre) > max {
		max = len(b.Pre)
	}
	for i := 0; i < max; i++ {
		if i >= len(a.Pre) {
			return -1
		}
		if i >= len(b.Pre) {
			return 1
		}
		ai := a.Pre[i]
		bi := b.Pre[i]
		if ai.numeric && bi.numeric {
			if ai.num != bi.num {
				return cmpInt(ai.num, bi.num)
			}
			continue
		}
		if ai.numeric != bi.numeric {
			if ai.numeric {
				return -1
			}
			return 1
		}
		if ai.raw != bi.raw {
			if ai.raw < bi.raw {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
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
