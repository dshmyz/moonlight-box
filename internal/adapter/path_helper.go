package adapter

import "strings"

func trimLeadingSlash(path string) string {
	return strings.TrimPrefix(path, "/")
}

func cutBeforeMarker(path string, marker string) string {
	if marker == "" {
		return path
	}
	if idx := strings.Index(path, marker); idx >= 0 {
		return path[:idx]
	}
	return path
}

func splitPath(path string) []string {
	return strings.Split(trimLeadingSlash(path), "/")
}

func splitPathN(path string, n int) []string {
	return strings.SplitN(trimLeadingSlash(path), "/", n)
}

func joinVersionFilename(version string, filename string) string {
	return version + "/" + filename
}
