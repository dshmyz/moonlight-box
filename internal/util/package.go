package util

import "strings"

func GenerateDisplayName(name, pkgType string) string {
	if name == "" {
		return ""
	}

	if pkgType == "maven" {
		lastSlash := strings.LastIndex(name, "/")
		if lastSlash > 0 {
			groupId := strings.ReplaceAll(name[:lastSlash], "/", ".")
			artifactId := name[lastSlash+1:]
			return groupId + ":" + artifactId
		}
	}

	return name
}
