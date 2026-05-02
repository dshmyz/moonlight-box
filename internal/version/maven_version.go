package version

import (
	"regexp"
	"strings"
)

var mavenVersionRegex = regexp.MustCompile(`^(\d+)(\.\d+)*(-[a-zA-Z0-9]+)*$`)

func NormalizeMavenVersion(version string) string {
	return strings.TrimSpace(version)
}

func IsValidMavenVersion(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}

	return mavenVersionRegex.MatchString(version) || strings.Contains(version, "-SNAPSHOT")
}
