package proxy

import "strings"

// TypeDetector 根据请求路径特征检测包类型
type TypeDetector struct{}

func NewTypeDetector() *TypeDetector {
	return &TypeDetector{}
}

// Detect 根据 URL 路径检测包类型
// 返回空字符串表示无法检测
func (d *TypeDetector) Detect(path string) string {
	if path == "" {
		return ""
	}

	// 1. 路径前缀精确匹配
	prefixMap := map[string]string{
		"npm/":   "npm",
		"maven/": "maven",
		"pypi/":  "pypi",
		"go/":    "go",
		"nuget/": "nuget",
		"yum/":   "yum",
		"apt/":   "apt",
	}
	for prefix, pkgType := range prefixMap {
		if strings.HasPrefix(path, prefix) {
			return pkgType
		}
	}

	// 2. 包类型特有 URL 模式匹配
	return d.matchPatterns(path)
}

func (d *TypeDetector) matchPatterns(path string) string {
	// npm: 包含 /-/ 路径（如 lodash/-/lodash-4.17.21.tgz）
	if strings.Contains(path, "/-/") {
		return "npm"
	}

	// pypi: 包含 /simple/ 路径
	if strings.Contains(path, "/simple/") || strings.Contains(path, "/packages/") {
		return "pypi"
	}

	// go: 包含 /@v/ 或 /mod/ 路径
	if strings.Contains(path, "/@v/") || strings.Contains(path, "/mod/") {
		return "go"
	}

	// nuget: 包含 /odata/ 或 /package/ 路径
	if strings.Contains(path, "/odata/") || strings.Contains(path, "/FindPackagesById") {
		return "nuget"
	}

	// yum: 包含 /repodata/ 路径
	if strings.Contains(path, "/repodata/") {
		return "yum"
	}

	// apt: 包含 /dists/ 或 /pool/ 路径
	if strings.Contains(path, "/dists/") || strings.Contains(path, "/pool/") {
		return "apt"
	}

	// maven: groupId/artifactId/version 格式（如 org/springframework/spring-core/5.3.0/）
	// 至少 3 层路径分隔
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 {
		return "maven"
	}

	return ""
}

// IsSupportedType 检查包类型是否在虚拟仓库支持的类型列表中
func (d *TypeDetector) IsSupportedType(pkgType string, packageTypes string) bool {
	if packageTypes == "" {
		return false
	}

	// packageTypes 是 JSON 数组字符串，如 '["npm","maven","pypi"]'
	types := parsePackageTypes(packageTypes)
	for _, t := range types {
		if t == pkgType {
			return true
		}
	}
	return false
}

func parsePackageTypes(s string) []string {
	s = strings.Trim(s, "[]\"' ")
	if s == "" {
		return nil
	}

	var result []string
	for _, t := range strings.Split(s, ",") {
		t = strings.Trim(t, " \"'")
		if t != "" {
			result = append(result, t)
		}
	}
	return result
}
