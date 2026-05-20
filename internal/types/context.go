package types

import (
	"io"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
)

type DownloadContext struct {
	Repo         *model.Repository
	PkgType      model.PackageType
	Name         string
	Version      string
	Filename     string
	UserID       uint
	ClientIP     string
	ResolvedPath *PackagePathInfo
	GinCtx       *gin.Context
}

type PublishContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	UserID   uint
	ClientIP string
}

type DeleteContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	Name     string
	Version  string
	UserID   uint
	ClientIP string
}

type DownloadResult struct {
	Content     io.ReadCloser
	Size        int64
	FromCache   bool
	RepoID      uint
	Filename    string
	Name        string
	Version     string
	ContentType string
}

// ContentResult 表示 adapter 返回的内容结果
type ContentResult struct {
	Content     io.ReadCloser
	Size        int64
	ContentType string
	StatusCode  int
	Headers     map[string]string
	ExtraData   map[string]interface{} // 用于 JSON/XML 等非流式响应
}
