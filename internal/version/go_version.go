package version

import (
	"strings"

	"golang.org/x/mod/semver"
)

func NormalizeGoVersion(version string) string {
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	if !semver.IsValid(version) {
		return version
	}

	return semver.Canonical(version)
}

func IsValidGoVersion(version string) bool {
	if !strings.HasPrefix(version, "v") {
		return false
	}
	return semver.IsValid(version)
}

func CompareGoVersions(v, w string) int {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !strings.HasPrefix(w, "v") {
		w = "v" + w
	}
	return semver.Compare(v, w)
}
