package util

import "strings"

func NormalizePackageType(pkgType string) string {
	switch strings.ToLower(pkgType) {
	case "maven2":
		return "maven"
	case "raw":
		return "generic"
	default:
		return strings.ToLower(pkgType)
	}
}

func ExpandPackageTypeAliases(pkgType string) []string {
	normalized := NormalizePackageType(pkgType)
	switch normalized {
	case "maven":
		return []string{"maven", "maven2"}
	case "generic":
		return []string{"generic", "raw"}
	default:
		return []string{normalized}
	}
}

func GenerateDisplayName(name, pkgType string) string {
	if name == "" {
		return ""
	}

	if NormalizePackageType(pkgType) == "maven" {
		lastSlash := strings.LastIndex(name, "/")
		if lastSlash > 0 {
			groupId := strings.ReplaceAll(name[:lastSlash], "/", ".")
			artifactId := name[lastSlash+1:]
			return groupId + ":" + artifactId
		}
	}

	return name
}
