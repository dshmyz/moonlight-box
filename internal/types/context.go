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

type RepoRequestContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	Path     string
	UserID   uint
	ClientIP string
}

type DownloadResult struct {
	Content   io.ReadCloser
	Size      int64
	FromCache bool
	RepoID    uint
	Filename  string
	Name      string
	Version   string
}

type PublishResult struct {
	PackageName    string
	Version        string
	Filename       string
	Content        io.Reader
	Size           int64
	FileType       model.PackageFileType
	StorageVersion string
	Metadata       map[string]interface{}
	Response       interface{}
}
