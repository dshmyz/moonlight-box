package types

import (
	"io"

	"github.com/moonlight-box/registry/internal/model"
)

type DownloadContext struct {
	Repo     *model.Repository
	PkgType  model.PackageType
	Name     string
	Version  string
	Filename string
	UserID   uint
	ClientIP string
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
	PackageName string
	Version     string
	Size        int64
	Filename    string
	Response    interface{}
}
