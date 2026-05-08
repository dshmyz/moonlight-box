package adapter

import (
	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/types"

	"github.com/gin-gonic/gin"
)

type RouterAdapter interface {
	types.Adapter
	RegisterRoutes(r *gin.RouterGroup, authMiddleware gin.HandlerFunc, permMiddleware func(resource, action string) gin.HandlerFunc)
}

type RepoAwareAdapter interface {
	types.Adapter
	HandleRepoRequest(c *gin.Context, repo *model.Repository, path string)
	HandleRepoPublish(c *gin.Context, repo *model.Repository)
	HandleRepoDelete(c *gin.Context, repo *model.Repository)
	SetPackageCache(pkgCache *cache.PackageCache)
}
