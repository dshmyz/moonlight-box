package adapter

import (
	"github.com/moonlight-box/registry/internal/types"
)

// Type aliases for backward compatibility
type PackageType = types.PackageType
type PackageIdentity = types.PackageIdentity
type UploadRequest = types.UploadRequest
type PackageMeta = types.PackageMeta
type VersionInfo = types.VersionInfo
type PackageVersionResult = types.PackageVersionResult

// Constants
const (
	NpmType     = types.NpmType
	MavenType   = types.MavenType
	PyPIType    = types.PyPIType
	GoType      = types.GoType
	YumType     = types.YumType
	AptType     = types.AptType
	GenericType = types.GenericType
)
