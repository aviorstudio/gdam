package spec

import (
	"fmt"
	"strings"
)

type PackageSpec struct {
	Owner   string
	Repo    string
	Version string
}

func (p PackageSpec) Name() string {
	return "@" + p.Owner + "/" + p.Repo
}

func (p PackageSpec) RepoPath() string {
	return p.Owner + "/" + p.Repo
}

func ParsePackageSpec(s string) (PackageSpec, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PackageSpec{}, fmt.Errorf("empty spec")
	}
	if !strings.HasPrefix(s, "@") {
		return PackageSpec{}, fmt.Errorf("spec must start with @ (got %q)", s)
	}

	rest := strings.TrimPrefix(s, "@")
	parts := strings.Split(rest, "@")
	if len(parts) > 2 {
		return PackageSpec{}, fmt.Errorf("invalid spec %q (expected @owner/repo[@version])", s)
	}

	repoPart := parts[0]
	if repoPart == "" {
		return PackageSpec{}, fmt.Errorf("invalid spec %q (missing owner/repo)", s)
	}

	repoBits := strings.Split(repoPart, "/")
	if len(repoBits) != 2 || repoBits[0] == "" || repoBits[1] == "" {
		return PackageSpec{}, fmt.Errorf("invalid repo %q (expected owner/repo)", repoPart)
	}

	spec := PackageSpec{
		Owner: strings.ToLower(repoBits[0]),
		Repo:  repoBits[1],
	}
	if len(parts) == 2 {
		spec.Version = parts[1]
	}
	return spec, nil
}
