package version

import (
	"regexp"
	"strings"
)

var npmVersionRegex = regexp.MustCompile(`^v?(\d+\.\d+\.\d+)(-[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)

func NormalizeNPMVersion(version string) string {
	version = strings.TrimPrefix(version, "v")

	if !IsValidNPMVersion(version) {
		return version
	}

	return version
}

func IsValidNPMVersion(version string) bool {
	version = strings.TrimPrefix(version, "v")
	return npmVersionRegex.MatchString(version)
}
