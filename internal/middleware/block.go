package middleware

import (
	"fmt"
	"strings"

	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"

	"github.com/gin-gonic/gin"
)

func BlockCheck(blockSvc *service.BlockRuleService) gin.HandlerFunc {
	return func(c *gin.Context) {
		pkgType, pkgName, version := extractPackageInfo(c)
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

func extractPackageInfo(c *gin.Context) (pkgType, pkgName, version string) {
	path := c.Request.URL.Path

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
