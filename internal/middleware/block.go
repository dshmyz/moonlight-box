package middleware

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

func BlockCheck(blockSvc *service.BlockRuleService, repoSvc *service.RepositoryService) gin.HandlerFunc {
	return func(c *gin.Context) {
		pkgType, pkgName, version := extractPackageInfo(c, repoSvc)
		if pkgName == "" {
			c.Next()
			return
		}

		result, err := blockSvc.IsBlocked(pkgType, pkgName, version)
		if err != nil {
			c.Next()
			return
		}

		if result.Blocked {
			ipAddress := c.ClientIP()
			userAgent := c.Request.UserAgent()
			_ = blockSvc.LogBlock(c.Request.Context(), pkgName, version, result.Rule, ipAddress, userAgent)

			reason := result.Rule.Reason
			if reason == "" {
				reason = "该版本已被管理员阻断"
			}
			msg := fmt.Sprintf("包 %s@%s 已被阻断: %s", pkgName, version, reason)
			response.Forbidden(c, msg)
			c.Abort()
			return
		}

		c.Next()
	}
}

func extractPackageInfo(c *gin.Context, repoSvc *service.RepositoryService) (pkgType, pkgName, version string) {
	path := c.Request.URL.Path

	if strings.HasPrefix(path, "/repo/") {
		return extractRepoPackageInfo(c, repoSvc)
	}

	if strings.HasPrefix(path, "/npm") {
		pkgType = "npm"
		pkgName, version = extractNpmInfo(c)
	} else if strings.HasPrefix(path, "/maven2") {
		pkgType = "maven"
		pkgName, version = extractMavenInfo(c)
	} else if strings.HasPrefix(path, "/pypi") {
		pkgType = "pypi"
		pkgName, version = extractPyPIInfo(c)
	} else if strings.HasPrefix(path, "/go") {
		pkgType = "go"
		pkgName, version = extractGoInfo(c)
	} else if strings.HasPrefix(path, "/nuget") {
		pkgType = "nuget"
		pkgName, version = extractNuGetInfo(c)
	} else if strings.HasPrefix(path, "/yum") {
		pkgType = "yum"
		pkgName, version = extractYumInfo(c)
	} else if strings.HasPrefix(path, "/apt") {
		pkgType = "apt"
		pkgName, version = extractAptInfo(c)
	} else if strings.HasPrefix(path, "/files") {
		pkgType = "generic"
		pkgName, version = extractGenericInfo(c)
	}

	return
}

func extractRepoPackageInfo(c *gin.Context, repoSvc *service.RepositoryService) (pkgType, pkgName, version string) {
	repoName := c.Param("repoName")
	if repoName == "" {
		path := c.Request.URL.Path
		parts := strings.Split(strings.TrimPrefix(path, "/repo/"), "/")
		if len(parts) > 0 {
			repoName = parts[0]
		}
	}

	if repoName == "" {
		return
	}

	repo, err := repoSvc.Get(repoName)
	if err != nil || repo == nil {
		return
	}

	pkgType = repo.PackageType
	subPath := strings.TrimPrefix(c.Param("path"), "/")

	switch pkgType {
	case "npm":
		pkgName, version = extractNpmFromPath(subPath)
	case "maven":
		pkgName, version = extractMavenFromPath(subPath)
	case "pypi":
		pkgName, version = extractPyPIFromPath(subPath)
	case "go":
		pkgName, version = extractGoFromPath(subPath)
	case "nuget":
		pkgName, version = extractNuGetFromPath(subPath)
	case "yum":
		pkgName, version = extractYumFromPath(subPath)
	case "apt":
		pkgName, version = extractAptFromPath(subPath)
	case "generic":
		pkgName, version = extractGenericFromPath(subPath)
	}

	return
}

func extractNpmFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	if strings.Contains(path, "/-/tarball/") {
		parts := strings.Split(path, "/-/tarball/")
		if len(parts) > 0 {
			pkgName = parts[0]
		}
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return
	}

	if strings.HasPrefix(parts[0], "@") && len(parts) >= 2 {
		pkgName = parts[0] + "/" + parts[1]
		if len(parts) >= 3 {
			version = parts[2]
		}
	} else {
		pkgName = parts[0]
		if len(parts) >= 2 {
			version = parts[1]
		}
	}

	return
}

func extractMavenFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		return
	}

	groupParts := []string{}
	artifact := ""
	ver := ""

	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == len(parts)-1 {
			continue
		}
		if strings.Contains(p, ".") && i > 0 {
			artifact = parts[i-1]
			ver = p
			break
		}
		groupParts = append(groupParts, p)
	}

	if len(groupParts) > 0 && artifact != "" {
		pkgName = strings.Join(groupParts, "/") + "/" + artifact
		version = ver
	}

	return
}

func extractPyPIFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	if strings.HasPrefix(path, "simple/") {
		path = strings.TrimPrefix(path, "simple/")
	}

	if strings.HasPrefix(path, "packages/") {
		filename := filepath.Base(path)
		filename = strings.Split(filename, "#")[0]
		pkgName, version = parsePyPIFilename(filename)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) >= 1 {
		pkgName = parts[0]
	}
	if len(parts) >= 2 {
		for _, p := range parts[1:] {
			if strings.Contains(p, ".whl") || strings.Contains(p, ".tar.gz") {
				version = p
				break
			}
		}
	}

	return
}

func parsePyPIFilename(filename string) (pkgName, version string) {
	if strings.HasSuffix(filename, ".whl") {
		parts := strings.Split(filename, "-")
		if len(parts) >= 2 {
			pkgName = parts[0]
			version = parts[1]
		}
	} else if strings.HasSuffix(filename, ".tar.gz") {
		name := strings.TrimSuffix(filename, ".tar.gz")
		idx := strings.LastIndex(name, "-")
		if idx > 0 {
			pkgName = name[:idx]
			version = name[idx+1:]
		}
	} else if strings.HasSuffix(filename, ".zip") {
		name := strings.TrimSuffix(filename, ".zip")
		idx := strings.LastIndex(name, "-")
		if idx > 0 {
			pkgName = name[:idx]
			version = name[idx+1:]
		}
	}
	return
}

func extractGoFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		pkgName = strings.Join(parts[:len(parts)-1], "/")
		version = parts[len(parts)-1]
		if strings.HasPrefix(version, "v") {
			version = version[1:]
		}
	}
	return
}

func extractNuGetFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 1 {
		pkgName = parts[0]
	}
	if len(parts) >= 2 {
		version = parts[1]
	}
	return
}

func extractYumFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 1 {
		pkgName = parts[0]
	}
	return
}

func extractAptFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 1 {
		pkgName = parts[0]
	}
	return
}

func extractGenericFromPath(path string) (pkgName, version string) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		pkgName = parts[0]
		version = parts[1]
	} else if len(parts) == 1 && parts[0] != "" {
		pkgName = parts[0]
	}
	return
}

func extractNpmInfo(c *gin.Context) (pkgName, version string) {
	scope := c.Param("scope")
	pkg := c.Param("package")

	if scope != "" {
		pkgName = scope + "/" + pkg
	} else {
		pkgName = pkg
	}

	filename := c.Param("filename")
	if filename != "" {
		return pkgName, ""
	}

	version = c.Param("version")
	return pkgName, version
}

func extractMavenInfo(c *gin.Context) (pkgName, version string) {
	group := c.Param("group")
	artifact := c.Param("artifact")
	version = c.Param("version")

	if group != "" && artifact != "" {
		pkgName = group + "/" + artifact
	}

	return pkgName, version
}

func extractPyPIInfo(c *gin.Context) (pkgName, version string) {
	pkg := c.Param("package")
	if pkg != "" {
		pkgName = pkg
	}
	version = c.Param("version")
	return pkgName, version
}

func extractGoInfo(c *gin.Context) (pkgName, version string) {
	path := strings.TrimPrefix(c.Request.URL.Path, "/go/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		pkgName = strings.Join(parts[:len(parts)-1], "/")
		version = parts[len(parts)-1]
		if strings.HasPrefix(version, "v") {
			version = version[1:]
		}
	}
	return pkgName, version
}

func extractNuGetInfo(c *gin.Context) (pkgName, version string) {
	pkgName = c.Param("id")
	version = c.Param("version")
	return pkgName, version
}

func extractYumInfo(c *gin.Context) (pkgName, version string) {
	pkgName = c.Param("package")
	version = c.Param("version")
	return pkgName, version
}

func extractAptInfo(c *gin.Context) (pkgName, version string) {
	pkgName = c.Param("package")
	version = ""
	return pkgName, version
}

func extractGenericInfo(c *gin.Context) (pkgName, version string) {
	path := strings.TrimPrefix(c.Request.URL.Path, "/files/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 {
		pkgName = parts[0]
		version = parts[1]
	} else if len(parts) == 1 && parts[0] != "" {
		pkgName = parts[0]
	}
	return pkgName, version
}
