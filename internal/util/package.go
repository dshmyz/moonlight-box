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
