package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
)

type PyPIAdapter struct {
	*BaseAdapter
	repoRepo  *repository.RepositoryRepository
	uploadSvc *service.UploadService
}

func NewPyPIAdapter(
	pkgRepo *repository.PackageRepository,
	repoRepo *repository.RepositoryRepository,
	storageSvc *service.StorageService,
	auditSvc *service.AuditService,
	pkgCache *cache.PackageCache,
) *PyPIAdapter {
	return &PyPIAdapter{
		BaseAdapter: NewBaseAdapter(pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache),
		repoRepo:    repoRepo,
		uploadSvc:   service.NewUploadService(pkgRepo, storageSvc),
	}
}

func (a *PyPIAdapter) Type() PackageType   { return PyPIType }
func (a *PyPIAdapter) RoutePrefix() string { return "/pypi" }

func (a *PyPIAdapter) BuildRemotePath(name, version, filename string) string {
	if filename != "" {
		return fmt.Sprintf("packages/%s", filename)
	}
	if version != "" {
		return fmt.Sprintf("simple/%s/", name)
	}
	return fmt.Sprintf("simple/%s/", name)
}

func (a *PyPIAdapter) ParsePackagePath(path string) (*PackageIdentity, error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid pypi path")
	}
	return &PackageIdentity{
		Name:    parts[0],
		Version: parts[1],
		Type:    PyPIType,
	}, nil
}

func (a *PyPIAdapter) ListPackages(c *gin.Context) {
	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/vnd.pypi.simple") || strings.Contains(accept, "application/json") {
		a.listPackagesJSON(c)
	} else {
		a.listPackagesHTML(c)
	}
}

func (a *PyPIAdapter) listPackagesJSON(c *gin.Context) {
	packages, _, err := a.pkgRepo.List(1, 10000, "pypi", "")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	type project struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}

	result := make([]project, len(packages))
	for i, pkg := range packages {
		result[i] = project{
			Name: normalizePackageName(pkg.Name),
			URL:  fmt.Sprintf("/pypi/simple/%s/", normalizePackageName(pkg.Name)),
		}
	}

	c.JSON(200, gin.H{"projects": result})
}

func (a *PyPIAdapter) listPackagesHTML(c *gin.Context) {
	packages, _, err := a.pkgRepo.List(1, 10000, "pypi", "")
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var sb strings.Builder
	sb.Grow(100 + len(packages)*80)
	sb.WriteString("<!DOCTYPE html>\n<html><head><title>Simple Index</title></head><body>\n")
	for _, pkg := range packages {
		normalized := normalizePackageName(pkg.Name)
		sb.WriteString(`<a href="/pypi/simple/`)
		sb.WriteString(normalized)
		sb.WriteString(`/">`)
		sb.WriteString(normalized)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	c.Data(200, "text/html", []byte(sb.String()))
}

func (a *PyPIAdapter) PackageFiles(c *gin.Context) {
	pkgName := c.Param("package")

	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/json") {
		a.packageFilesJSON(c, pkgName)
	} else {
		a.packageFilesHTML(c, pkgName)
	}
}

func (a *PyPIAdapter) packageFilesJSON(c *gin.Context, pkgName string) {
	pkg, err := a.pkgRepo.FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "pypi", pkgName, "")
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						c.Data(200, "application/vnd.pypi.simple.v1+json", body)
						return
					}
				}
			}
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	type file struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}

	files := make([]file, 0)
	for _, ver := range pkg.Versions {
		files = append(files, file{
			URL:      fmt.Sprintf("/pypi/packages/%s", filepath.Base(ver.StoragePath)),
			Filename: filepath.Base(ver.StoragePath),
		})
	}

	c.JSON(200, gin.H{
		"files": files,
		"meta": gin.H{
			"api-version": "1.0",
		},
	})
}

func (a *PyPIAdapter) packageFilesHTML(c *gin.Context, pkgName string) {
	pkg, err := a.pkgRepo.FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "pypi", pkgName, "")
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						c.Data(200, "text/html", body)
						return
					}
				}
			}
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><title>Links for %s</title></head><body>
<h1>Links for %s</h1>
`, pkgName, pkgName)

	var sb strings.Builder
	sb.Grow(len(html) + len(pkg.Versions)*60)
	sb.WriteString(html)

	for _, ver := range pkg.Versions {
		filename := filepath.Base(ver.StoragePath)
		sb.WriteString(`<a href="/pypi/packages/`)
		sb.WriteString(filename)
		sb.WriteString(`">`)
		sb.WriteString(filename)
		sb.WriteString(`</a><br>` + "\n")
	}
	sb.WriteString("</body></html>")

	c.Data(200, "text/html", []byte(sb.String()))
}

func (a *PyPIAdapter) DownloadPackage(c *gin.Context) {
	filename := c.Param("filename")
	slog.Info("DownloadPackage called", "filename", filename)

	if strings.HasSuffix(filename, ".sha256") {
		a.handleChecksumRequest(c, filename)
		return
	}

	actualFilename := filepath.Base(filename)
	name, version := parseWheelFilename(actualFilename)
	slog.Info("Parsed filename", "name", name, "version", version, "actualFilename", actualFilename)
	if name == "" {
		response.BadRequest(c, "invalid filename", "unable to parse package name from filename")
		return
	}

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypePyPI, name, version, actualFilename)
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", name, actualFilename)
	if err == nil {
		defer content.Close()

		contentType := a.storageSvc.GetContentType(actualFilename)
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, actualFilename))
		c.DataFromReader(200, size, contentType, content, nil)
		return
	}

	if a.fetcher != nil && repo != nil && repo.Type == "proxy" {
		slog.Info("PyPI proxy: fetching from remote", "filename", actualFilename, "name", name)
		result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "pypi", name, actualFilename)
		if fetchErr == nil && result != nil {
			defer result.Content.Close()
			slog.Info("PyPI proxy: successfully fetched from remote", "filename", actualFilename, "size", result.Size)
			contentType := a.storageSvc.GetContentType(actualFilename)
			c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, actualFilename))
			c.DataFromReader(200, result.Size, contentType, result.Content, nil)
			return
		}
		slog.Warn("PyPI proxy: failed to fetch from remote", "filename", actualFilename, "error", fetchErr)
	}

	response.NotFound(c, "package not found")
}

func (a *PyPIAdapter) handleChecksumRequest(c *gin.Context, filename string) {
	// 移除 .sha256 后缀获取实际文件名
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	// 从数据库查找文件记录
	files, err := a.pkgRepo.FindFilesByFilename(actualFilename)
	if err != nil || len(files) == 0 {
		response.NotFound(c, "checksum not found")
		return
	}

	// 获取第一个匹配的文件
	file := files[0]
	if file.ChecksumSHA256 == "" {
		// 如果数据库中没有校验和，尝试从存储中读取文件并计算
		name, _ := parseWheelFilename(actualFilename)
		if name == "" {
			response.NotFound(c, "invalid filename")
			return
		}

		content, _, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", name, actualFilename)
		if err != nil {
			response.NotFound(c, "file not found")
			return
		}
		defer content.Close()

		// 计算SHA256
		body, err := io.ReadAll(content)
		if err != nil {
			response.InternalError(c, "failed to read file")
			return
		}

		hash := sha256.Sum256(body)
		checksum := hex.EncodeToString(hash[:])

		// 返回校验和
		c.Data(200, "text/plain", []byte(checksum))
		return
	}

	// 返回数据库中的校验和
	c.Data(200, "text/plain", []byte(file.ChecksumSHA256))
}

func (a *PyPIAdapter) JSONAPI(c *gin.Context) {
	pkgName := c.Param("package")
	version := c.Param("version")

	pkg, err := a.pkgRepo.FindByNameAndType(pkgName, model.PackageTypePyPI)
	if err != nil {
		if util.IsErr(err, util.ErrPackageNotFound) {
			if a.fetcher != nil {
				var repo *model.Repository
				if r, ok := c.Get("repo"); ok {
					repo = r.(*model.Repository)
				}

				result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), repo, "pypi", pkgName, version)
				if resolveErr == nil && result != nil {
					defer result.Content.Close()
					body, readErr := io.ReadAll(result.Content)
					if readErr == nil {
						c.Data(200, "application/json", body)
						return
					}
				}
			}
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	var versionInfo *model.PackageVersion
	for _, ver := range pkg.Versions {
		if ver.Version == version {
			v := ver
			versionInfo = &v
			break
		}
	}

	if versionInfo == nil {
		response.NotFound(c, "version not found")
		return
	}

	type urlInfo struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		MD5      string `json:"md5_digest,omitempty"`
		SHA256   string `json:"sha256_digest,omitempty"`
		Size     int64  `json:"size"`
	}

	c.JSON(200, gin.H{
		"info": gin.H{
			"name":    pkg.Name,
			"version": version,
			"summary": pkg.Description,
		},
		"releases": gin.H{
			version: []urlInfo{{
				URL:    fmt.Sprintf("/pypi/packages/%s", filepath.Base(versionInfo.StoragePath)),
				Size:   versionInfo.SizeBytes,
				MD5:    versionInfo.ChecksumMD5,
				SHA256: versionInfo.ChecksumSHA256,
			}},
		},
	})
}

func (a *PyPIAdapter) UploadPackage(c *gin.Context) {
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("content")
	if err != nil {
		response.BadRequest(c, "missing file", "no file uploaded")
		return
	}
	defer file.Close()

	pkgName := c.PostForm("name")
	version := c.PostForm("version")
	if pkgName == "" || version == "" {
		response.BadRequest(c, "missing metadata", "name and version are required")
		return
	}

	repoName := c.PostForm("repository")
	var repositoryID uint
	if repoName != "" {
		repo, err := a.repoRepo.FindByName(repoName)
		if err != nil {
			response.BadRequest(c, "invalid repository", "repository not found: "+repoName)
			return
		}
		if repo.Type != model.RepoTypeLocal {
			response.BadRequest(c, "invalid repository", "only local repositories support uploading")
			return
		}
		repositoryID = repo.ID
	}

	req := &UploadRequest{
		Package:  file,
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"name":         pkgName,
			"version":      version,
			"description":  c.PostForm("summary"),
			"filename":     header.Filename,
			"repositoryID": repositoryID,
		},
		UploadedBy: userID,
	}

	result, err := a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{"success": true, "result": result})
}

func (a *PyPIAdapter) Upload(ctx context.Context, req *UploadRequest) (*PackageVersionResult, error) {
	reader, ok := req.Package.(io.Reader)
	if !ok {
		return nil, fmt.Errorf("invalid package type")
	}

	name, _ := req.Metadata["name"].(string)
	version, _ := req.Metadata["version"].(string)
	filename, _ := req.Metadata["filename"].(string)
	if filename == "" {
		filename = req.Filename
	}
	if name == "" || version == "" {
		return nil, fmt.Errorf("missing name or version")
	}

	repositoryID, _ := req.Metadata["repositoryID"].(uint)

	result, err := a.uploadSvc.Upload(ctx, &service.UploadContext{
		PkgType:        "pypi",
		Name:           name,
		Version:        version,
		StorageVersion: filename,
		Filename:       filename,
		Content:        reader,
		Size:           req.Size,
		PackageType:    model.PackageTypePyPI,
		RepositoryID:   repositoryID,
		RepositoryType: model.RepoTypeLocal,
		UploadedBy:     req.UploadedBy,
		Metadata:       req.Metadata,
		FileType:       model.FileTypePrimary,
	})

	if err != nil {
		return nil, err
	}

	return &PackageVersionResult{
		PackageID:  result.PackageID,
		VersionID:  result.VersionID,
		Version:    result.Version,
		StorageKey: result.StorageKey,
		Size:       result.Size,
		Checksum:   result.ChecksumSHA256,
	}, nil
}

func (a *PyPIAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	return a.BaseAdapter.GetPackageMetadata(ctx, name, model.PackageTypePyPI, PyPIType)
}

func (a *PyPIAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.pkgRepo.DeleteByNameAndVersion(identity.Name, identity.Version, model.PackageTypePyPI)
}

func (a *PyPIAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.pkgRepo.ListVersions(name, model.PackageTypePyPI)
}

func (a *PyPIAdapter) HandleRepoRequest(c *gin.Context, ctx *types.RepoRequestContext) {
	c.Set("repo", ctx.Repo)
	if strings.HasPrefix(ctx.Path, "simple/") {
		pkgPath := strings.TrimPrefix(ctx.Path, "simple/")
		if pkgPath == "" || pkgPath == "/" {
			a.ListPackages(c)
		} else {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: strings.Trim(pkgPath, "/")})
			a.PackageFiles(c)
		}
	} else if strings.HasPrefix(ctx.Path, "packages/") {
		filename := strings.TrimPrefix(ctx.Path, "packages/")
		c.Params = append(c.Params, gin.Param{Key: "filename", Value: filename})
		a.DownloadPackage(c)
	} else if strings.Contains(ctx.Path, "/json") {
		parts := strings.Split(ctx.Path, "/")
		if len(parts) >= 2 {
			c.Params = append(c.Params, gin.Param{Key: "package", Value: parts[0]})
			c.Params = append(c.Params, gin.Param{Key: "version", Value: parts[1]})
			a.JSONAPI(c)
		}
	} else {
		if a.fetcher != nil {
			result, resolveErr := a.fetcher.FetchFromRemote(c.Request.Context(), ctx.Repo, "pypi", ctx.Path, "")
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					contentType := a.storageSvc.GetContentType(ctx.Path)
					c.Data(200, contentType, body)
					return
				}
			}
		}

		response.NotFound(c, "path not found")
	}
}

func (a *PyPIAdapter) HandleRepoPublish(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
	userID := c.GetUint("userID")

	file, header, err := c.Request.FormFile("content")
	if err != nil {
		response.BadRequest(c, "missing file", err.Error())
		return nil
	}
	defer file.Close()

	pkgData, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file", err.Error())
		return nil
	}

	name, version := parseWheelFilename(header.Filename)
	if name == "" {
		name = strings.TrimSuffix(header.Filename, ".whl")
		name = strings.TrimSuffix(name, ".tar.gz")
	}

	req := &UploadRequest{
		Package:  bytes.NewReader(pkgData),
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"name":      name,
			"version":   version,
			"repo_name": repo.Name,
		},
		UploadedBy: userID,
	}

	_, err = a.Upload(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, err.Error())
		return nil
	}

	result := &types.RepoOperationResult{
		PackageName: name,
		Version:     version,
		Size:        header.Size,
		Filename:    header.Filename,
		ExtraData: map[string]interface{}{
			"size":     header.Size,
			"filename": header.Filename,
		},
	}

	result.Response = &types.PypiPublishResponse{
		PublishResponse: types.PublishResponse{
			Success:  true,
			Message:  "Package published successfully",
			Package:  name,
			Version:  version,
			Filename: header.Filename,
			Size:     header.Size,
		},
	}

	return result
}

func (a *PyPIAdapter) HandleRepoDelete(c *gin.Context, repo *model.Repository) *types.RepoOperationResult {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 2 {
		response.BadRequest(c, "invalid path", "expected name/version")
		return nil
	}

	name := parts[0]
	version := parts[1]

	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    PyPIType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		response.InternalError(c, err.Error())
		return nil
	}

	pkg, _ := a.pkgRepo.FindByNameAndType(name, model.PackageTypePyPI)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, repo.Name, name, version, pkgID)

	c.JSON(200, gin.H{"ok": true})

	return &types.RepoOperationResult{
		PackageName: name,
		Version:     version,
	}
}

var wheelRegex = regexp.MustCompile(`^([A-Za-z0-9_]+)-([^-]+)-.+\.whl$`)
var sdistRegex = regexp.MustCompile(`^([A-Za-z0-9_]+)-([^-]+)\.(tar\.gz|tar\.bz2|zip)$`)

func parseWheelFilename(filename string) (name, version string) {
	basename := filepath.Base(filename)
	basename = strings.Split(basename, "#")[0]

	if matches := wheelRegex.FindStringSubmatch(basename); len(matches) >= 3 {
		return matches[1], matches[2]
	}

	if matches := sdistRegex.FindStringSubmatch(basename); len(matches) >= 3 {
		return matches[1], matches[2]
	}

	parts := strings.SplitN(basename, "-", 2)
	if len(parts) == 2 {
		version = strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
		return parts[0], version
	}
	return "", ""
}

func normalizePackageName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

func (a *PyPIAdapter) FormatDownloadResponse(c *gin.Context, result *types.DownloadResult) {
	contentType := a.storageSvc.GetContentType(result.Filename)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, result.Filename))
	c.DataFromReader(200, result.Size, contentType, result.Content, nil)
}

func (a *PyPIAdapter) HandleDownload(c *gin.Context, ctx *types.DownloadContext) (*types.DownloadResult, error) {
	name := ctx.Name
	version := ctx.Version
	filename := ctx.Filename

	storageName := name + "/" + filename
	content, size, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", storageName, version)
	if err == nil {
		return &types.DownloadResult{
			Content:   content,
			Size:      size,
			FromCache: false,
			RepoID:    ctx.Repo.ID,
			Filename:  filename,
			Name:      name,
			Version:   version,
		}, nil
	}

	if a.fetcher != nil && ctx.Repo.Type == "proxy" {
		result, fetchErr := a.fetcher.FetchFromRemote(c.Request.Context(), ctx.Repo, "pypi", name, version+"/"+filename)
		if fetchErr == nil && result != nil {
			return &types.DownloadResult{
				Content:   result.Content,
				Size:      result.Size,
				FromCache: result.FromCache,
				RepoID:    result.RepoID,
				Filename:  filename,
				Name:      name,
				Version:   version,
			}, nil
		}
	}

	return nil, fmt.Errorf("package not found")
}

func (a *PyPIAdapter) HandlePublish(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	userID := ctx.UserID

	file, header, err := c.Request.FormFile("content")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}
	defer file.Close()

	pkgData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	name, version := parseWheelFilename(header.Filename)
	if name == "" {
		name = strings.TrimSuffix(header.Filename, ".whl")
		name = strings.TrimSuffix(name, ".tar.gz")
	}

	req := &UploadRequest{
		Package:  bytes.NewReader(pkgData),
		Filename: header.Filename,
		Size:     header.Size,
		Metadata: map[string]interface{}{
			"name":      name,
			"version":   version,
			"repo_name": ctx.Repo.Name,
		},
		UploadedBy: userID,
	}

	_, err = a.Upload(c.Request.Context(), req)
	if err != nil {
		return nil, err
	}

	return &types.PublishResult{
		PackageName: name,
		Version:     version,
		Size:        header.Size,
		Filename:    header.Filename,
		Response: &types.PypiPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  name,
				Version:  version,
				Filename: header.Filename,
				Size:     header.Size,
			},
		},
	}, nil
}

func (a *PyPIAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	fullPath := strings.TrimPrefix(c.Param("path"), "/")
	parts := strings.Split(fullPath, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid path: expected name/version")
	}

	name := parts[0]
	version := parts[1]

	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    PyPIType,
	}

	if err := a.Delete(c.Request.Context(), identity); err != nil {
		return err
	}

	pkg, _ := a.pkgRepo.FindByNameAndType(name, model.PackageTypePyPI)
	var pkgID *uint
	if pkg != nil {
		pkgID = &pkg.ID
	}
	a.LogDeleteAudit(c, ctx.Repo.Name, name, version, pkgID)

	return nil
}
