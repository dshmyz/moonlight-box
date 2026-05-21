package adapter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type AptAdapter struct {
	*BaseAdapter
	repoRepo *repository.RepositoryRepository
}

type AptReleaseFile struct {
	Origin     string
	Label      string
	Arch       string
	Date       string
	Suites     string
	Components string
	MD5Sum     []AptHashEntry
	SHA1       []AptHashEntry
	SHA256     []AptHashEntry
}

type AptHashEntry struct {
	MD5    string
	SHA1   string
	SHA256 string
	Size   int64
	Name   string
}

type AptPackageEntry struct {
	Package       string
	Version       string
	Architecture  string
	Maintainer    string
	Description   string
	Section       string
	Priority      string
	InstalledSize string
	Filename      string
	Size          int64
	MD5Sum        string
	SHA1          string
	SHA256        string
}

func NewAptAdapter(
	storageSvc *service.StorageService,
	pkgCache *cache.PackageCache,
) *AptAdapter {
	adapter := &AptAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
	}
	return adapter
}

func (a *AptAdapter) Type() PackageType { return AptType }

// validateAptPath 验证 APT 路径安全性
func validateAptPath(path string) error {
	// 检测路径遍历攻击
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal detected")
	}

	// 检测 Windows 绝对路径
	if strings.Contains(path, ":\\") || strings.HasPrefix(path, "\\") {
		return fmt.Errorf("absolute paths not allowed")
	}

	// 检测 null 字符
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("null characters not allowed")
	}

	return nil
}

// isValidDebFilename 验证 .deb 文件名格式
func isValidDebFilename(filename string) bool {
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9._+-]*\.deb$`, filename)
	return matched
}

func (a *AptAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	// 路径安全验证
	if err := validateAptPath(path); err != nil {
		return nil, err
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid apt path: %s", path)
	}

	filename := parts[len(parts)-1]
	name := strings.Join(parts[:len(parts)-1], "/")
	version := ""

	if strings.Contains(filename, ".deb") {
		base := strings.TrimSuffix(filename, ".deb")
		pkgParts := strings.Split(base, "_")
		if len(pkgParts) >= 2 {
			version = pkgParts[1]
		}
	}

	storageName := name
	storageVersion := filename
	remotePath := path

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       filename,
		StorageName:    storageName,
		StorageVersion: storageVersion,
		RemotePath:     remotePath,
	}, nil
}

func (a *AptAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetRepositoryPackageMetadata(ctx, repositoryFromContext(ctx), name, model.PackageTypeApt, AptType)
}

func (a *AptAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByRepoNameAndVersionContext(ctx, repositoryID(repositoryFromContext(ctx)), identity.Name, identity.Version, model.PackageTypeApt)
}

func (a *AptAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersionsByRepoContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypeApt)
}

func formatPackageEntry(entry AptPackageEntry) string {
	return fmt.Sprintf(
		`Package: %s
Version: %s
Architecture: %s
Maintainer: %s
Description: %s
Section: %s
Priority: %s
Installed-Size: %s
Filename: %s
Size: %d
`,
		entry.Package,
		entry.Version,
		entry.Architecture,
		entry.Maintainer,
		entry.Description,
		entry.Section,
		entry.Priority,
		entry.InstalledSize,
		entry.Filename,
		entry.Size,
	)
}

func parseDebPackageName(filename string) string {
	base := strings.TrimSuffix(filename, ".deb")
	parts := strings.SplitN(base, "_", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return base
}

func parseDebPackageVersion(filename string) string {
	base := strings.TrimSuffix(filename, ".deb")
	parts := strings.SplitN(base, "_", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return "1.0.0"
}

func (a *AptAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}
	defer file.Close()

	debName := header.Filename
	if !strings.HasSuffix(debName, ".deb") {
		return nil, fmt.Errorf("invalid file type: file must be .deb")
	}

	packageName := parseDebPackageName(debName)
	packageVersion := parseDebPackageVersion(debName)

	return &types.PublishResult{
		PackageName: packageName,
		Version:     packageVersion,
		Filename:    debName,
		Content:     file,
		Size:        header.Size,
		FileType:    model.FileTypePrimary,
		Response: &types.AptPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  packageName,
				Version:  packageVersion,
				Filename: debName,
				Size:     header.Size,
			},
		},
	}, nil
}

func (a *AptAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	filePath := strings.TrimPrefix(c.Param("path"), "/")
	filePath = strings.TrimPrefix(filePath, "pool/")

	packageName := parseDebPackageName(filepath.Base(filePath))
	packageVersion := parseDebPackageVersion(filepath.Base(filePath))

	identity := &PackageIdentity{
		Name:    packageName,
		Version: packageVersion,
		Type:    AptType,
	}

	if err := a.Delete(context.WithValue(c.Request.Context(), "repo", ctx.Repo), identity); err != nil {
		return err
	}

	return nil
}

// ParseIntent 解析请求路径为意图
func (a *AptAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = strings.TrimPrefix(path, "/")
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
	}

	if strings.HasPrefix(path, "pool/") {
		filePath := strings.TrimPrefix(path, "pool/")
		filename := filepath.Base(filePath)
		version := parseDebPackageVersion(filename)
		name := parseDebPackageName(filename)

		intent.Type = types.RequestDownload
		intent.Filename = filename
		intent.Name = name
		intent.Version = version
		return intent
	}

	if strings.HasPrefix(path, "dists/") {
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) >= 3 {
			dist := parts[1]
			switch parts[2] {
			case "Release":
				intent.Type = types.RequestMetadata
				intent.Extra["dist"] = dist
				intent.Extra["file"] = "Release"
				return intent
			case "InRelease":
				intent.Type = types.RequestMetadata
				intent.Extra["dist"] = dist
				intent.Extra["file"] = "InRelease"
				return intent
			case "Release.gpg":
				intent.Type = types.RequestMetadata
				intent.Extra["file"] = "Release.gpg"
				return intent
			}
		}

		if len(parts) >= 5 {
			fileName := parts[len(parts)-1]
			if fileName == "Packages" || fileName == "Packages.gz" {
				intent.Type = types.RequestMetadata
				intent.Filename = fileName
				return intent
			}
		}

		// Other dists paths
		intent.Type = types.RequestMetadata
		return intent
	}

	// Fallback
	pathInfo, _ := a.ParsePath(path)
	intent.Type = types.RequestDownload
	if pathInfo != nil {
		intent.Name = pathInfo.Name
		intent.Filename = pathInfo.Filename
		intent.PkgPathInfo = pathInfo
	}

	return intent
}

// FetchContent 根据意图获取内容
func (a *AptAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	if strings.HasPrefix(intent.Path, "pool/") {
		return a.downloadDeb(ctx, intent.Path, repo)
	}

	if strings.HasPrefix(intent.Path, "dists/") {
		parts := strings.Split(strings.Trim(intent.Path, "/"), "/")
		if len(parts) >= 3 {
			dist := parts[1]
			switch parts[2] {
			case "Release":
				return a.releaseFile(dist)
			case "InRelease":
				return a.inReleaseFile(dist)
			case "Release.gpg":
				return a.releaseGPG()
			}
		}

		if len(parts) >= 6 {
			fileName := parts[4]
			if fileName == "Packages" {
				return a.packagesFile(ctx, repo, intent.Path)
			}
			if fileName == "Packages.gz" {
				return a.packagesFileGz(ctx, repo, intent.Path)
			}
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "APT resource not found"},
	}, nil
}

// releaseFile 生成 Release 文件内容（不依赖 gin.Context）
func (a *AptAdapter) releaseFile(dist string) (*types.ContentResult, error) {
	release := fmt.Sprintf(
		`Origin: Moonlight Registry
Label: Moonlight
Suite: %s
Codename: %s
Architectures: amd64 arm64 i386
Components: main
Date: %s
Description: Moonlight Registry APT Repository
`,
		dist, dist, time.Now().UTC().Format(time.RFC1123),
	)

	release += "\nMD5Sum:\n"
	release += "SHA1:\n"
	release += "SHA256:\n"

	// 生成 ETag
	etag := util.GenerateETag([]byte(release))

	return &types.ContentResult{
		ContentType: "text/plain; charset=utf-8",
		StatusCode:  200,
		Content:     io.NopCloser(bytes.NewReader([]byte(release))),
		Size:        int64(len(release)),
		Headers: map[string]string{
			"ETag":          etag,
			"Cache-Control": "public, max-age=300",
		},
	}, nil
}

// inReleaseFile 生成 InRelease 文件内容（不依赖 gin.Context）
func (a *AptAdapter) inReleaseFile(dist string) (*types.ContentResult, error) {
	release := fmt.Sprintf(
		`-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

Origin: Moonlight Registry
Label: Moonlight
Suite: %s
Codename: %s
Architectures: amd64 arm64 i386
Components: main
Date: %s
Description: Moonlight Registry APT Repository
-----BEGIN PGP SIGNATURE-----
(placeholder - not signed)
-----END PGP SIGNATURE-----
`,
		dist, dist, time.Now().UTC().Format(time.RFC1123),
	)

	// 生成 ETag
	etag := util.GenerateETag([]byte(release))

	return &types.ContentResult{
		ContentType: "text/plain; charset=utf-8",
		StatusCode:  200,
		Content:     io.NopCloser(bytes.NewReader([]byte(release))),
		Size:        int64(len(release)),
		Headers: map[string]string{
			"ETag":          etag,
			"Cache-Control": "public, max-age=300",
		},
	}, nil
}

// releaseGPG 返回 GPG 签名不可用的响应（不依赖 gin.Context）
func (a *AptAdapter) releaseGPG() (*types.ContentResult, error) {
	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "GPG signature not available"},
	}, nil
}

// packagesFile 生成 Packages 文件内容（不依赖 gin.Context）
func (a *AptAdapter) packagesFile(ctx context.Context, repo *model.Repository, remotePath string) (*types.ContentResult, error) {
	// Try upstream first for proxy repos — package index is dynamic.
	// FetchFromRemote has HTTP-level cache, so repeat requests within TTL won't hit upstream.
	if repo != nil && repo.Type == model.RepoTypeProxy && a.fetcher != nil {
		remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), strings.TrimPrefix(remotePath, "/"))
		result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
		if fetchErr == nil && result != nil {
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				etag := util.GenerateETag(body)
				return &types.ContentResult{
					ContentType: "text/plain; charset=utf-8",
					StatusCode:  200,
					Content:     io.NopCloser(bytes.NewReader(body)),
					Size:        int64(len(body)),
					Headers: map[string]string{
						"ETag":          etag,
						"Cache-Control": "public, max-age=300",
					},
				}, nil
			}
		}
	}

	// Fallback: return from local DB
	packages, _, err := a.GetPackageRepository().ListContext(ctx, 1, 10000, string(model.PackageTypeApt), "")
	if err != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}, nil
	}

	var output strings.Builder
	for _, pkg := range packages {
		for _, ver := range pkg.Versions {
			filename := filepath.Base(ver.Version)
			entry := AptPackageEntry{
				Package:       pkg.Name,
				Version:       ver.Version,
				Architecture:  "amd64",
				Maintainer:    "Moonlight Registry",
				Description:   pkg.Description,
				Section:       "misc",
				Priority:      "optional",
				InstalledSize: fmt.Sprintf("%d", ver.SizeBytes/1024),
				Filename:      fmt.Sprintf("pool/%s", filename),
				Size:          ver.SizeBytes,
				MD5Sum:        ver.ChecksumMD5,
				SHA256:        ver.ChecksumSHA256,
			}
			output.WriteString(formatPackageEntry(entry))
		}
	}

	if output.Len() == 0 {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "packages not found"},
		}, nil
	}

	content := output.String()

	// 生成 ETag
	etag := util.GenerateETag([]byte(content))

	return &types.ContentResult{
		ContentType: "text/plain; charset=utf-8",
		StatusCode:  200,
		Content:     io.NopCloser(bytes.NewReader([]byte(content))),
		Size:        int64(len(content)),
		Headers: map[string]string{
			"ETag":          etag,
			"Cache-Control": "public, max-age=300",
		},
	}, nil
}

// packagesFileGz 生成 Packages.gz 文件内容（不依赖 gin.Context）
func (a *AptAdapter) packagesFileGz(ctx context.Context, repo *model.Repository, remotePath string) (*types.ContentResult, error) {
	// Try upstream first for proxy repos — package index is dynamic.
	// FetchFromRemote has HTTP-level cache, so repeat requests within TTL won't hit upstream.
	if repo != nil && repo.Type == model.RepoTypeProxy && a.fetcher != nil {
		remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), strings.TrimPrefix(remotePath, "/"))
		result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
		if fetchErr == nil && result != nil {
			defer result.Content.Close()
			body, readErr := io.ReadAll(result.Content)
			if readErr == nil {
				etag := util.GenerateETag(body)
				return &types.ContentResult{
					ContentType: "application/gzip",
					StatusCode:  200,
					Content:     io.NopCloser(bytes.NewReader(body)),
					Size:        int64(len(body)),
					Headers: map[string]string{
						"Content-Disposition": `attachment; filename="Packages.gz"`,
						"ETag":                etag,
						"Cache-Control":       "public, max-age=300",
					},
				}, nil
			}
		}
	}

	// Fallback: return from local DB
	packages, _, err := a.GetPackageRepository().ListContext(ctx, 1, 10000, string(model.PackageTypeApt), "")
	if err != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}, nil
	}

	var output strings.Builder
	for _, pkg := range packages {
		for _, ver := range pkg.Versions {
			filename := filepath.Base(ver.Version)
			entry := AptPackageEntry{
				Package:       pkg.Name,
				Version:       ver.Version,
				Architecture:  "amd64",
				Maintainer:    "Moonlight Registry",
				Description:   pkg.Description,
				Section:       "misc",
				Priority:      "optional",
				InstalledSize: fmt.Sprintf("%d", ver.SizeBytes/1024),
				Filename:      fmt.Sprintf("pool/%s", filename),
				Size:          ver.SizeBytes,
			}
			output.WriteString(formatPackageEntry(entry))
		}
	}

	content := output.String()

	// 生成 ETag
	etag := util.GenerateETag([]byte(content))

	headers := map[string]string{
		"Content-Disposition": `attachment; filename="Packages.gz"`,
		"ETag":                etag,
		"Cache-Control":       "public, max-age=300",
	}

	return &types.ContentResult{
		ContentType: "application/gzip",
		StatusCode:  200,
		Content:     io.NopCloser(bytes.NewReader([]byte(content))),
		Size:        int64(len(content)),
		Headers:     headers,
	}, nil
}

// downloadDeb 下载 deb 文件（不依赖 gin.Context）
func (a *AptAdapter) downloadDeb(ctx context.Context, filePath string, repo *model.Repository) (*types.ContentResult, error) {
	filePath = strings.TrimPrefix(filePath, "/")

	// 路径安全验证
	if err := validateAptPath(filePath); err != nil {
		return &types.ContentResult{
			StatusCode: 400,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}, nil
	}

	// 验证文件名格式
	filename := filepath.Base(filePath)
	if !strings.HasSuffix(filename, ".deb") || !isValidDebFilename(filename) {
		return &types.ContentResult{
			StatusCode: 400,
			ExtraData:  map[string]interface{}{"message": "invalid deb filename"},
		}, nil
	}

	storageKey := filePath

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(ctx, storageKey)
	if err == nil {
		defer content.Close()

		// 读取内容用于计算 ETag
		body, readErr := io.ReadAll(content)
		if readErr != nil {
			return &types.ContentResult{
				StatusCode: 500,
				ExtraData:  map[string]interface{}{"message": "failed to read content"},
			}, nil
		}

		etag := util.GenerateETag(body)

		headers := map[string]string{
			"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)),
			"ETag":                etag,
			"Cache-Control":       "public, max-age=86400",
		}

		return &types.ContentResult{
			Content:     io.NopCloser(bytes.NewReader(body)),
			Size:        int64(len(body)),
			ContentType: "application/vnd.debian.binary-package",
			StatusCode:  200,
			Headers:     headers,
		}, nil
	}

	if a.fetcher != nil && repo != nil && repo.Type == model.RepoTypeProxy {
		slog.Info("APT proxy: fetching from remote", "filePath", filePath)
		pathInfo, pathErr := a.ParsePath(filePath)
		if pathErr != nil {
			slog.Warn("APT proxy: failed to resolve path", "filePath", filePath, "error", pathErr)
		} else {
			remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), pathInfo.RemotePath)
			result, fetchErr := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
			if fetchErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					etag := util.GenerateETag(body)
					slog.Info("APT proxy: successfully fetched from remote", "filePath", filePath, "size", result.Size)
					headers := map[string]string{
						"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, url.PathEscape(filename)),
						"ETag":                etag,
						"Cache-Control":       "public, max-age=86400",
					}
					return &types.ContentResult{
						Content:     io.NopCloser(bytes.NewReader(body)),
						Size:        int64(len(body)),
						ContentType: "application/vnd.debian.binary-package",
						StatusCode:  200,
						Headers:     headers,
					}, nil
				}
			}
			slog.Warn("APT proxy: failed to fetch from remote", "filePath", filePath, "error", fetchErr)
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "DEB not found"},
	}, nil
}
