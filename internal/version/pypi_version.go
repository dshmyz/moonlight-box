package version

import (
	"regexp"
	"strings"
)

var pypiVersionRegex = regexp.MustCompile(`^(\d+)(\.\d+)*((a|b|rc|post|dev)\d*)?(\+[a-zA-Z0-9]+)?$`)

func NormalizePyPIVersion(version string) string {
	version = strings.TrimSpace(version)

	if strings.Contains(version, "!") {
		parts := strings.SplitN(version, "!", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}

	return version
}

func IsValidPyPIVersion(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}

	return pypiVersionRegex.MatchString(version)
}
