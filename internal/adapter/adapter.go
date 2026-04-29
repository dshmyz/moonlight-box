package adapter

import (
	"context"

	"github.com/moonlight-box/registry/internal/types"

	"github.com/gin-gonic/gin"
)

type Adapter interface {
	Type() types.PackageType
	RoutePrefix() string
	RegisterRoutes(r *gin.RouterGroup, authMiddleware gin.HandlerFunc, permMiddleware func(resource, action string) gin.HandlerFunc)
	ParsePackagePath(path string) (*types.PackageIdentity, error)
	Upload(ctx context.Context, req *types.UploadRequest) (*types.PackageVersionResult, error)
	Download(ctx context.Context, identity *types.PackageIdentity) (*types.PackageContent, error)
	GetMetadata(ctx context.Context, name string) (*types.PackageMeta, error)
	Delete(ctx context.Context, identity *types.PackageIdentity) error
	ListVersions(ctx context.Context, name string) ([]string, error)
}
