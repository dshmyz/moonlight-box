package adapter

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/moonlight-box/registry/internal/cache"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/moonlight-box/registry/internal/types"
	"github.com/moonlight-box/registry/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type YumAdapter struct {
	*BaseAdapter
	repoRepo *repository.RepositoryRepository
}

// 路径遍历防护：检查路径是否包含危险字符或路径遍历
func validateYumPath(path string) error {
	// 检查空路径
	if path == "" {
		return fmt.Errorf("invalid yum path: empty path")
	}

	// 检查路径遍历攻击
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid yum path: path traversal not allowed")
	}

	// 检查危险字符
	dangerousPatterns := []string{
		"~", "$", "`", "|", ";", "&", "(", ")", "<", ">", "\n", "\r", "\x00",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(path, pattern) {
			return fmt.Errorf("invalid yum path: dangerous character not allowed")
		}
	}

	// 检查绝对路径
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "/repos/") {
		return fmt.Errorf("invalid yum path: absolute path not allowed")
	}

	return nil
}

// isValidRPMFilename 验证 RPM 文件名格式
func isValidRPMFilename(filename string) bool {
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9._-]*\.rpm$`, filename)
	return matched
}

type RepoMD struct {
	XMLName  xml.Name     `xml:"repomd"`
	Xmlns    string       `xml:"xmlns,attr"`
	XmlnsRpm string       `xml:"xmlns:rpm,attr"`
	Revision string       `xml:"revision"`
	Data     []RepoMDData `xml:"data"`
}

type RepoMDData struct {
	Type         string         `xml:"type,attr"`
	Checksum     Checksum       `xml:"checksum"`
	OpenChecksum Checksum       `xml:"open-checksum"`
	Location     RepoMDLocation `xml:"location"`
	Timestamp    int64          `xml:"timestamp"`
	Size         int64          `xml:"size"`
	OpenSize     int64          `xml:"open-size"`
}

type Checksum struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type RepoMDLocation struct {
	Href string `xml:"href,attr"`
}

type PrimaryData struct {
	XMLName  xml.Name     `xml:"metadata"`
	Xmlns    string       `xml:"xmlns,attr"`
	XmlnsRpm string       `xml:"xmlns:rpm,attr"`
	Packages []YumPackage `xml:"package"`
}

type YumPackage struct {
	Type        string      `xml:"type,attr"`
	Name        string      `xml:"name"`
	Arch        string      `xml:"arch"`
	Version     YumVersion  `xml:"version"`
	Checksum    YumChecksum `xml:"checksum"`
	Summary     string      `xml:"summary"`
	Description string      `xml:"description"`
	Packager    string      `xml:"packager"`
	URL         string      `xml:"url"`
	Time        YumTime     `xml:"time"`
	Size        YumSize     `xml:"size"`
	Location    YumLocation `xml:"location"`
	Format      YumFormat   `xml:"format"`
}

type YumVersion struct {
	Epoch string `xml:"epoch,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

type YumChecksum struct {
	Type string `xml:"type,attr"`
	Hash string `xml:",chardata"`
}

type YumTime struct {
	File  int64 `xml:"file,attr"`
	Build int64 `xml:"build,attr"`
}

type YumSize struct {
	Package   int64 `xml:"package,attr"`
	Installed int64 `xml:"installed,attr"`
	Archive   int64 `xml:"archive,attr"`
}

type YumLocation struct {
	Href string `xml:"href,attr"`
}

type YumFormat struct {
	License   string          `xml:"license"`
	Vendor    string          `xml:"vendor"`
	Group     string          `xml:"group"`
	Buildhost string          `xml:"buildhost"`
	Requires  []YumDependency `xml:"requires>rpm:entry"`
	Provides  []YumDependency `xml:"provides>rpm:entry"`
}

type YumDependency struct {
	Name  string `xml:"name,attr"`
	Flags string `xml:"flags,attr"`
	Epoch string `xml:"epoch,attr,omitempty"`
	Ver   string `xml:"ver,attr,omitempty"`
	Rel   string `xml:"rel,attr,omitempty"`
	Pre   bool   `xml:"pre,attr,omitempty"`
}

// FilelistsData filelists.xml.gz 结构
type FilelistsData struct {
	XMLName xml.Name           `xml:"filelists"`
	Xmlns   string             `xml:"xmlns,attr"`
	Packages []FilelistsPackage `xml:"package"`
}

type FilelistsPackage struct {
	Name     string            `xml:"name,attr"`
	Arch     string            `xml:"arch,attr"`
	Version  YumVersion        `xml:"version"`
	Files    []FilelistsFile   `xml:"file"`
}

type FilelistsFile struct {
	Type string `xml:"type,attr"`
	Name string `xml:",chardata"`
}

// OtherData other.xml.gz 结构
type OtherData struct {
	XMLName xml.Name          `xml:"otherdata"`
	Xmlns   string            `xml:"xmlns,attr"`
	Packages []OtherPackage   `xml:"package"`
}

type OtherPackage struct {
	Name     string   `xml:"name,attr"`
	Arch     string   `xml:"arch,attr"`
	Version  YumVersion `xml:"version"`
	Changelogs []Changelog `xml:"changelog"`
}

type Changelog struct {
	Date  int64  `xml:"date,attr"`
	Author string `xml:"author,attr"`
	Version string `xml:"version,attr"`
	Text  string `xml:",chardata"`
}

func NewYumAdapter(args ...interface{}) *YumAdapter {
	var repoRepo *repository.RepositoryRepository
	var storageSvc *service.StorageService
	var pkgCache *cache.PackageCache

	// New signature: (repoRepo, storageSvc, pkgCache)
	if len(args) >= 1 {
		if r, ok := args[0].(*repository.RepositoryRepository); ok {
			repoRepo = r
		}
	}
	if len(args) >= 2 {
		if s, ok := args[1].(*service.StorageService); ok {
			storageSvc = s
		}
	}
	if len(args) >= 3 {
		if c, ok := args[2].(*cache.PackageCache); ok {
			pkgCache = c
		}
	}

	// Legacy signature: (pkgRepo, repoRepo, storageSvc, auditSvc, pkgCache)
	if repoRepo == nil && len(args) >= 2 {
		if r, ok := args[1].(*repository.RepositoryRepository); ok {
			repoRepo = r
		}
	}
	if storageSvc == nil && len(args) >= 3 {
		if s, ok := args[2].(*service.StorageService); ok {
			storageSvc = s
		}
	}
	if pkgCache == nil && len(args) >= 1 {
		if pkgRepo, ok := args[0].(*repository.PackageRepository); ok {
			pkgCache = cache.NewPackageCache(pkgRepo, 5*time.Minute)
		}
	}

	adapter := &YumAdapter{
		BaseAdapter: NewBaseAdapter(storageSvc, pkgCache),
		repoRepo:    repoRepo,
	}
	return adapter
}

func (a *YumAdapter) Type() PackageType { return YumType }

func (a *YumAdapter) ParsePath(path string) (*types.PackagePathInfo, error) {
	path = strings.Trim(path, "/")

	// 路径遍历防护
	if err := validateYumPath(path); err != nil {
		return nil, err
	}

	if path == "" {
		return nil, fmt.Errorf("invalid yum path: empty path")
	}

	if strings.Contains(path, ".rpm") {
		return a.resolveRpmPath(path)
	}

	parts := strings.Split(path, "/")

	name := parts[0]
	version := ""
	if len(parts) >= 2 {
		version = parts[1]
	}

	remotePath := name
	if version != "" {
		remotePath = name + "/" + version
	}

	return &types.PackagePathInfo{
		Name:           name,
		Version:        version,
		Filename:       "",
		StorageName:    name,
		StorageVersion: version,
		RemotePath:     remotePath,
	}, nil
}

func (a *YumAdapter) resolveRpmPath(path string) (*types.PackagePathInfo, error) {
	// 路径遍历防护
	if err := validateYumPath(path); err != nil {
		return nil, err
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid yum rpm path: %s", path)
	}

	filename := parts[len(parts)-1]

	// 验证 RPM 文件名格式
	if !isValidRPMFilename(filename) {
		return nil, fmt.Errorf("invalid rpm filename: %s", filename)
	}

	name := strings.TrimSuffix(filename, ".rpm")
	version := ""

	base := strings.TrimSuffix(filename, ".rpm")
	pkgParts := strings.Split(base, "-")
	if len(pkgParts) >= 2 {
		version = pkgParts[1]
	}

	pkgName, _, _, _ := parseRpmFilename(filename)
	storageName := pkgName
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

func (a *YumAdapter) RepoMetadata(c *gin.Context, repo string) *types.ContentResult {
	repomdXML, err := a.generateRepomdXML(c.Request.Context(), repo)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}
	}

	return &types.ContentResult{
		ContentType: "application/xml",
		StatusCode:  200,
		Content:     io.NopCloser(bytes.NewReader([]byte(repomdXML))),
		Size:        int64(len(repomdXML)),
	}
}

func (a *YumAdapter) RepoDataFile(c *gin.Context, repo string, filePath string) *types.ContentResult {
	// 路径遍历防护
	if err := validateYumPath(filePath); err != nil {
		return &types.ContentResult{
			StatusCode: 400,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}
	}

	storageKey := fmt.Sprintf("repos/%s/repodata/%s", repo, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err == nil {
		defer content.Close()
		size, _ := backend.Size(c.Request.Context(), storageKey)
		contentType := "application/xml"
		if strings.HasSuffix(filePath, ".gz") {
			contentType = "application/gzip"
		}

		// 读取内容用于计算 ETag
		body, readErr := io.ReadAll(content)
		if readErr == nil {
			etag := util.GenerateETag(body)
			lastModified := time.Now().UTC().Format(time.RFC1123)

			return &types.ContentResult{
				Content:     io.NopCloser(bytes.NewReader(body)),
				Size:        int64(len(body)),
				ContentType: contentType,
				StatusCode:  200,
				Headers: map[string]string{
					"ETag":          etag,
					"Last-Modified": lastModified,
					"Cache-Control": "public, max-age=86400",
				},
			}
		}

		// 回退：如果无法读取内容
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: contentType,
			StatusCode:  200,
		}
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "metadata file not found"},
	}
}

func (a *YumAdapter) DownloadRPM(c *gin.Context, repoName string, filePath string) *types.ContentResult {
	// 路径遍历防护
	if err := validateYumPath(filePath); err != nil {
		return &types.ContentResult{
			StatusCode: 400,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}
	}

	storageKey := fmt.Sprintf("repos/%s/Packages/%s", repoName, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(c.Request.Context(), storageKey)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "RPM not found"},
		}
	}
	defer content.Close()

	// 读取内容用于计算 ETag
	body, readErr := io.ReadAll(content)
	if readErr != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": "failed to read content"},
		}
	}

	// 生成 ETag（基于内容 SHA256）
	etag := util.GenerateETag(body)
	lastModified := time.Now().UTC().Format(time.RFC1123)

	filename := filepath.Base(filePath)
	headers := map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename),
		"ETag":                etag,
		"Last-Modified":       lastModified,
		"Cache-Control":       "public, max-age=86400",
	}

	return &types.ContentResult{
		Content:     io.NopCloser(bytes.NewReader(body)),
		Size:        int64(len(body)),
		ContentType: "application/x-rpm",
		StatusCode:  200,
		Headers:     headers,
	}
}

func (a *YumAdapter) UploadRPM(c *gin.Context) {
	userID := c.GetUint("userID")
	repo := c.Param("repo")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.BadRequest(c, "missing file", err.Error())
		return
	}
	defer file.Close()

	rpmData, err := io.ReadAll(file)
	if err != nil {
		response.BadRequest(c, "failed to read file", err.Error())
		return
	}

	rpmName := header.Filename
	if !strings.HasSuffix(rpmName, ".rpm") {
		response.BadRequest(c, "invalid file type", "file must be .rpm")
		return
	}

	packageName, version, release, arch := parseRpmFilename(rpmName)

	packagesDir := fmt.Sprintf("repos/%s/Packages/%s", repo, arch)
	storageKey := fmt.Sprintf("%s/%s", packagesDir, rpmName)

	backend := a.storageSvc.GetDefaultBackend()
	if err := backend.Put(c.Request.Context(), storageKey, bytes.NewReader(rpmData), header.Size); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	pkg, _, err := a.GetPackageRepository().CreateOrUpdate(c.Request.Context(), &model.Package{
		Name:           packageName,
		Type:           model.PackageTypeYum,
		RepositoryType: model.RepoTypeLocal,
		CreatedBy:      userID,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		SizeBytes:   header.Size,
		PublishedBy: userID,
		Metadata:    marshalMetadata(map[string]interface{}{"repo": repo, "arch": arch, "release": release}),
	})
	if err != nil {
		backend.Delete(c.Request.Context(), storageKey)
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{
		"success":    true,
		"repo":       repo,
		"filename":   rpmName,
		"storageKey": storageKey,
		"result": &PackageVersionResult{
			PackageID:  pkg.ID,
			Version:    version,
			StorageKey: storageKey,
			Size:       header.Size,
		},
	})
}

func (a *YumAdapter) RegenerateMetadata(c *gin.Context) {
	repo := c.Param("repo")

	if err := a.regenerateRepodata(c.Request.Context(), repo); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.JSON(200, gin.H{
		"success": true,
		"message": "metadata regenerated",
		"repo":    repo,
	})
}

func (a *YumAdapter) GetMetadata(ctx context.Context, name string) (*PackageMeta, error) {
	pkg, err := a.GetPackageRepository().FindByRepoNameAndTypeContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypeYum)
	if err != nil {
		return nil, err
	}

	return packageMetaFromModel(pkg, YumType), nil
}

func (a *YumAdapter) Delete(ctx context.Context, identity *PackageIdentity) error {
	return a.GetPackageRepository().DeleteByRepoNameAndVersionContext(ctx, repositoryID(repositoryFromContext(ctx)), identity.Name, identity.Version, model.PackageTypeYum)
}

func (a *YumAdapter) ListVersions(ctx context.Context, name string) ([]string, error) {
	return a.GetPackageRepository().ListVersionsByRepoContext(ctx, repositoryID(repositoryFromContext(ctx)), name, model.PackageTypeYum)
}

// ParseIntent 解析请求路径为意图
func (a *YumAdapter) ParseIntent(path string, method string) *types.RequestIntent {
	path = strings.TrimPrefix(path, "/")
	intent := &types.RequestIntent{
		Path:  path,
		Extra: make(map[string]interface{}),
	}

	if strings.HasPrefix(path, "repodata/") {
		filePath := strings.TrimPrefix(path, "repodata/")
		intent.Type = types.RequestMetadata
		intent.Filename = filePath
		return intent
	}

	if strings.HasPrefix(path, "Packages/") {
		filePath := strings.TrimPrefix(path, "Packages/")
		intent.Type = types.RequestDownload
		intent.Filename = filepath.Base(filePath)
		if strings.HasSuffix(intent.Filename, ".rpm") {
			name, version, _, _ := parseRpmFilename(intent.Filename)
			intent.Name = name
			intent.Version = version
		}
		if pathInfo, err := a.ParsePath(path); err == nil {
			intent.PkgPathInfo = pathInfo
		}
		return intent
	}

	// Generic storage access
	intent.Type = types.RequestDownload
	pathInfo, _ := a.ParsePath(path)
	if pathInfo != nil {
		intent.Name = pathInfo.Name
		intent.Version = pathInfo.Version
		intent.Filename = pathInfo.Filename
		intent.PkgPathInfo = pathInfo
	}

	return intent
}

// FetchContent 根据意图获取内容
func (a *YumAdapter) HandleGet(ctx context.Context, repo *model.Repository, intent *types.RequestIntent) (*types.ContentResult, error) {
	path := strings.TrimPrefix(intent.Path, "/")

	// 提取请求头用于缓存协商
	reqHeaders := make(map[string]string)
	if v, ok := intent.Extra["If-None-Match"].(string); ok {
		reqHeaders["If-None-Match"] = v
	}

	// Proxy repo: fetch all content from remote upstream
	if repo.Type == model.RepoTypeProxy && a.fetcher != nil {
		remoteURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(repo.RemoteURL, "/"), path)
		result, err := a.fetcher.FetchFromRemote(ctx, repo, remoteURL)
		if err != nil {
			return nil, err
		}
		return &types.ContentResult{
			Content:     result.Content,
			Size:        result.Size,
			ContentType: a.storageSvc.GetContentType(path),
			StatusCode:  200,
		}, nil
	}

	if strings.HasPrefix(path, "repodata/") {
		filePath := strings.TrimPrefix(path, "repodata/")
		return a.repoDataFile(ctx, repo.Name, filePath, reqHeaders)
	}

	if strings.HasPrefix(path, "Packages/") {
		filePath := strings.TrimPrefix(path, "Packages/")
		return a.downloadRPM(ctx, repo.Name, filePath, reqHeaders)
	}

	// Generic storage access
	storageKey := fmt.Sprintf("repos/%s/%s", repo.Name, path)
	backend := a.storageSvc.GetDefaultBackend()

	content, err := backend.Get(ctx, storageKey)
	if err == nil {
		defer content.Close()
		size, _ := backend.Size(ctx, storageKey)
		contentType := a.storageSvc.GetContentType(path)
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: contentType,
			StatusCode:  200,
		}, nil
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "file not found"},
	}, nil
}

// repoDataFile 获取 repodata 文件（不依赖 gin.Context）
func (a *YumAdapter) repoDataFile(ctx context.Context, repo string, filePath string, reqHeaders map[string]string) (*types.ContentResult, error) {
	// 路径遍历防护
	if err := validateYumPath(filePath); err != nil {
		return &types.ContentResult{
			StatusCode: 400,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}, nil
	}

	storageKey := fmt.Sprintf("repos/%s/repodata/%s", repo, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(ctx, storageKey)
	if err == nil {
		defer content.Close()
		size, _ := backend.Size(ctx, storageKey)
		contentType := "application/xml"
		if strings.HasSuffix(filePath, ".gz") {
			contentType = "application/gzip"
		}

		// 读取内容用于计算 ETag
		body, readErr := io.ReadAll(content)
		if readErr == nil {
			etag := util.GenerateETag(body)
			lastModified := time.Now().UTC().Format(time.RFC1123)

			// 检查 If-None-Match 头，如果匹配则返回 304
			if result, matched := util.CheckIfNotModified(reqHeaders, etag); matched {
				return &types.ContentResult{
					StatusCode: result.StatusCode,
					Headers:    result.Headers,
				}, nil
			}

			return &types.ContentResult{
				Content:     io.NopCloser(bytes.NewReader(body)),
				Size:        int64(len(body)),
				ContentType: contentType,
				StatusCode:  200,
				Headers: map[string]string{
					"ETag":          etag,
					"Last-Modified": lastModified,
					"Cache-Control": "public, max-age=86400",
				},
			}, nil
		}

		// 回退
		return &types.ContentResult{
			Content:     content,
			Size:        size,
			ContentType: contentType,
			StatusCode:  200,
		}, nil
	}

	return &types.ContentResult{
		StatusCode: 404,
		ExtraData:  map[string]interface{}{"message": "metadata file not found"},
	}, nil
}

// downloadRPM 下载 RPM 文件（不依赖 gin.Context）
func (a *YumAdapter) downloadRPM(ctx context.Context, repoName string, filePath string, reqHeaders map[string]string) (*types.ContentResult, error) {
	// 路径遍历防护
	if err := validateYumPath(filePath); err != nil {
		return &types.ContentResult{
			StatusCode: 400,
			ExtraData:  map[string]interface{}{"message": err.Error()},
		}, nil
	}

	storageKey := fmt.Sprintf("repos/%s/Packages/%s", repoName, filePath)

	backend := a.storageSvc.GetDefaultBackend()
	content, err := backend.Get(ctx, storageKey)
	if err != nil {
		return &types.ContentResult{
			StatusCode: 404,
			ExtraData:  map[string]interface{}{"message": "RPM not found"},
		}, nil
	}
	defer content.Close()

	// 读取内容用于计算 ETag
	body, readErr := io.ReadAll(content)
	if readErr != nil {
		return &types.ContentResult{
			StatusCode: 500,
			ExtraData:  map[string]interface{}{"message": "failed to read content"},
		}, nil
	}

	// 生成 ETag
	etag := util.GenerateETag(body)
	lastModified := time.Now().UTC().Format(time.RFC1123)

	// 检查 If-None-Match 头，如果匹配则返回 304
	if result, matched := util.CheckIfNotModified(reqHeaders, etag); matched {
		return &types.ContentResult{
			StatusCode: result.StatusCode,
			Headers:    result.Headers,
		}, nil
	}

	filename := filepath.Base(filePath)
	headers := map[string]string{
		"Content-Disposition": fmt.Sprintf(`attachment; filename="%s"`, filename),
		"ETag":                etag,
		"Last-Modified":       lastModified,
		"Cache-Control":       "public, max-age=86400",
	}

	return &types.ContentResult{
		Content:     io.NopCloser(bytes.NewReader(body)),
		Size:        int64(len(body)),
		ContentType: "application/x-rpm",
		StatusCode:  200,
		Headers:     headers,
	}, nil
}

func (a *YumAdapter) generateRepomdXML(ctx context.Context, repo string) (string, error) {
	repomd := RepoMD{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		XmlnsRpm: "http://linux.duke.edu/metadata/rpm",
		Revision: fmt.Sprintf("%d", time.Now().Unix()),
	}

	backend := a.storageSvc.GetDefaultBackend()
	entries, err := backend.List(ctx, fmt.Sprintf("repos/%s/repodata/", repo))
	if err != nil {
		entries = nil
	}

	for _, entry := range entries {
		if strings.Contains(entry.Key, "primary.xml") {
			// 读取文件计算 checksum
			content, err := backend.Get(ctx, entry.Key)
			if err == nil {
				defer content.Close()
				body, readErr := io.ReadAll(content)
				if readErr == nil {
					hash := sha256.Sum256(body)
					checksum := hex.EncodeToString(hash[:])
					repomd.Data = append(repomd.Data, RepoMDData{
						Type: "primary",
						Checksum: Checksum{
							Type:  "sha256",
							Value: checksum,
						},
						OpenChecksum: Checksum{
							Type:  "sha256",
							Value: checksum,
						},
						Location: RepoMDLocation{
							Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
						},
						Size:     int64(len(body)),
						OpenSize: int64(len(body)),
					})
					continue
				}
			}
			// 回退：没有 checksum
			repomd.Data = append(repomd.Data, RepoMDData{
				Type: "primary",
				Checksum: Checksum{
					Type:  "sha256",
					Value: "",
				},
				Location: RepoMDLocation{
					Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
				},
			})
		} else if strings.Contains(entry.Key, "filelists.xml") {
			content, err := backend.Get(ctx, entry.Key)
			if err == nil {
				defer content.Close()
				body, readErr := io.ReadAll(content)
				if readErr == nil {
					hash := sha256.Sum256(body)
					checksum := hex.EncodeToString(hash[:])
					repomd.Data = append(repomd.Data, RepoMDData{
						Type: "filelists",
						Checksum: Checksum{
							Type:  "sha256",
							Value: checksum,
						},
						OpenChecksum: Checksum{
							Type:  "sha256",
							Value: checksum,
						},
						Location: RepoMDLocation{
							Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
						},
						Size:     int64(len(body)),
						OpenSize: int64(len(body)),
					})
					continue
				}
			}
			repomd.Data = append(repomd.Data, RepoMDData{
				Type: "filelists",
				Location: RepoMDLocation{
					Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
				},
			})
		} else if strings.Contains(entry.Key, "other.xml") {
			content, err := backend.Get(ctx, entry.Key)
			if err == nil {
				defer content.Close()
				body, readErr := io.ReadAll(content)
				if readErr == nil {
					hash := sha256.Sum256(body)
					checksum := hex.EncodeToString(hash[:])
					repomd.Data = append(repomd.Data, RepoMDData{
						Type: "other",
						Checksum: Checksum{
							Type:  "sha256",
							Value: checksum,
						},
						OpenChecksum: Checksum{
							Type:  "sha256",
							Value: checksum,
						},
						Location: RepoMDLocation{
							Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
						},
						Size:     int64(len(body)),
						OpenSize: int64(len(body)),
					})
					continue
				}
			}
			repomd.Data = append(repomd.Data, RepoMDData{
				Type: "other",
				Location: RepoMDLocation{
					Href: fmt.Sprintf("repodata/%s", filepath.Base(entry.Key)),
				},
			})
		}
	}

	output, err := xml.MarshalIndent(repomd, "", "  ")
	if err != nil {
		return "", err
	}

	return xml.Header + string(output), nil
}

func (a *YumAdapter) regenerateRepodata(ctx context.Context, repo string) error {
	packages, _, err := a.GetPackageRepository().ListContext(ctx, 1, 10000, string(model.PackageTypeYum), "")
	if err != nil {
		return err
	}

	var yumPkgs []YumPackage
	var filelistsPkgs []FilelistsPackage
	var otherPkgs []OtherPackage

	for _, pkg := range packages {
		for _, ver := range pkg.Versions {
			meta := unmarshalMetadata(ver.Metadata)

			release := ""
			if meta != nil {
				if r, ok := meta["release"].(string); ok {
					release = r
				}
			}

			version := ver.Version
			if release != "" && !strings.Contains(version, "-") {
				version = version + "-" + release
			}

			// 完整的版本号 (EVR: Epoch-Version-Release)
			epoch := "0"
			if meta != nil {
				if e, ok := meta["epoch"].(string); ok && e != "" {
					epoch = e
				}
			}

			arch := "x86_64"
			if meta != nil {
				if a, ok := meta["arch"].(string); ok {
					arch = a
				}
			}

			// 构建完整的 RPM 文件名: name-version-release.arch.rpm
			rpmFilename := fmt.Sprintf("%s-%s.%s.rpm", pkg.Name, version, arch)

			// 读取 RPM 内容计算 checksum
			storageKey := fmt.Sprintf("repos/%s/Packages/%s/%s", repo, arch, rpmFilename)
			backend := a.storageSvc.GetDefaultBackend()
			content, err := backend.Get(ctx, storageKey)
			var rpmChecksum string
			var rpmSize int64 = ver.SizeBytes
			if err == nil {
				defer content.Close()
				body, readErr := io.ReadAll(content)
				if readErr == nil {
					hash := sha256.Sum256(body)
					rpmChecksum = hex.EncodeToString(hash[:])
					rpmSize = int64(len(body))
				}
			}

			// primary.xml
			yumPkg := YumPackage{
				Type:    "rpm",
				Name:    pkg.Name,
				Arch:    arch,
				Version: YumVersion{Epoch: epoch, Ver: ver.Version, Rel: release},
				Checksum: YumChecksum{
					Type: "sha256",
					Hash: rpmChecksum,
				},
				Size: YumSize{Package: rpmSize},
				Location: YumLocation{
					// 使用完整的 RPM 文件名
					Href: fmt.Sprintf("Packages/%s/%s", arch, rpmFilename),
				},
			}
			yumPkgs = append(yumPkgs, yumPkg)

			// filelists.xml
			filelistsPkg := FilelistsPackage{
				Name:    pkg.Name,
				Arch:    arch,
				Version: YumVersion{Epoch: epoch, Ver: ver.Version, Rel: release},
			}
			filelistsPkgs = append(filelistsPkgs, filelistsPkg)

			// other.xml (changelog)
			otherPkg := OtherPackage{
				Name:    pkg.Name,
				Arch:    arch,
				Version: YumVersion{Epoch: epoch, Ver: ver.Version, Rel: release},
				Changelogs: []Changelog{
					{
						Date:    time.Now().Unix(),
						Author:  "packager",
						Version: ver.Version,
						Text:    fmt.Sprintf("Package %s-%s released", pkg.Name, version),
					},
				},
			}
			otherPkgs = append(otherPkgs, otherPkg)
		}
	}

	// 生成 primary.xml.gz
	primaryData := PrimaryData{
		Xmlns:    "http://linux.duke.edu/metadata/repo",
		XmlnsRpm: "http://linux.duke.edu/metadata/rpm",
		Packages: yumPkgs,
	}

	primaryXML, err := xml.MarshalIndent(primaryData, "", "  ")
	if err != nil {
		return err
	}

	primaryBuf, err := compressXML(primaryXML)
	if err != nil {
		return err
	}

	primaryChecksum := sha256.Sum256(primaryBuf)
	primaryFilename := fmt.Sprintf("%s-primary.xml.gz", hex.EncodeToString(primaryChecksum[:8]))
	primaryChecksumValue := hex.EncodeToString(primaryChecksum[:])

	backend := a.storageSvc.GetDefaultBackend()
	repodataDir := fmt.Sprintf("repos/%s/repodata/", repo)
	if err := backend.Put(ctx, repodataDir+primaryFilename, bytes.NewReader(primaryBuf), int64(len(primaryBuf))); err != nil {
		return err
	}

	logrus.Infof("Generated primary.xml.gz: %s, checksum: %s", primaryFilename, primaryChecksumValue)

	// 生成 filelists.xml.gz
	filelistsData := FilelistsData{
		Xmlns:   "http://linux.duke.edu/metadata/filelists",
		Packages: filelistsPkgs,
	}

	filelistsXML, err := xml.MarshalIndent(filelistsData, "", "  ")
	if err != nil {
		return err
	}

	filelistsBuf, err := compressXML(filelistsXML)
	if err != nil {
		return err
	}

	filelistsChecksum := sha256.Sum256(filelistsBuf)
	filelistsFilename := fmt.Sprintf("%s-filelists.xml.gz", hex.EncodeToString(filelistsChecksum[:8]))
	filelistsChecksumValue := hex.EncodeToString(filelistsChecksum[:])

	if err := backend.Put(ctx, repodataDir+filelistsFilename, bytes.NewReader(filelistsBuf), int64(len(filelistsBuf))); err != nil {
		return err
	}

	logrus.Infof("Generated filelists.xml.gz: %s, checksum: %s", filelistsFilename, filelistsChecksumValue)

	// 生成 other.xml.gz
	otherData := OtherData{
		Xmlns:   "http://linux.duke.edu/metadata/other",
		Packages: otherPkgs,
	}

	otherXML, err := xml.MarshalIndent(otherData, "", "  ")
	if err != nil {
		return err
	}

	otherBuf, err := compressXML(otherXML)
	if err != nil {
		return err
	}

	otherChecksum := sha256.Sum256(otherBuf)
	otherFilename := fmt.Sprintf("%s-other.xml.gz", hex.EncodeToString(otherChecksum[:8]))
	otherChecksumValue := hex.EncodeToString(otherChecksum[:])

	if err := backend.Put(ctx, repodataDir+otherFilename, bytes.NewReader(otherBuf), int64(len(otherBuf))); err != nil {
		return err
	}

	logrus.Infof("Generated other.xml.gz: %s, checksum: %s", otherFilename, otherChecksumValue)

	return nil
}

// compressXML gzip 压缩 XML 数据
func compressXML(xmlData []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(xmlData); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func detectRpmArch(filename string) string {
	if strings.Contains(filename, ".x86_64") {
		return "x86_64"
	} else if strings.Contains(filename, ".aarch64") {
		return "aarch64"
	} else if strings.Contains(filename, ".i686") {
		return "i686"
	} else if strings.Contains(filename, ".noarch") {
		return "noarch"
	} else if strings.Contains(filename, ".armv7hl") {
		return "armv7hl"
	}
	return "x86_64"
}

func parseRpmFilename(filename string) (name, version, release, arch string) {
	filename = strings.TrimSuffix(filename, ".rpm")

	arch = detectRpmArch(filename)

	archSuffix := "." + arch
	if idx := strings.LastIndex(filename, archSuffix); idx > 0 {
		filename = filename[:idx]
	}

	parts := strings.Split(filename, "-")
	if len(parts) >= 3 {
		name = strings.Join(parts[:len(parts)-2], "-")
		version = parts[len(parts)-2]
		release = parts[len(parts)-1]
	} else if len(parts) == 2 {
		name = parts[0]
		version = parts[1]
		release = "1"
	} else {
		name = filename
		version = "1.0.0"
		release = "1"
	}

	return name, version, release, arch
}

func unmarshalMetadata(data string) map[string]interface{} {
	if data == "" {
		return nil
	}
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(data), &meta); err != nil {
		return nil
	}
	return meta
}

func (a *YumAdapter) HandlePut(c *gin.Context, ctx *types.PublishContext) (*types.PublishResult, error) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		return nil, fmt.Errorf("missing file: %v", err)
	}
	defer file.Close()

	rpmData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	rpmName := header.Filename
	if !strings.HasSuffix(rpmName, ".rpm") {
		return nil, fmt.Errorf("invalid file type: file must be .rpm")
	}

	packageName, version, release, arch := parseRpmFilename(rpmName)
	storageVersion := rpmName

	return &types.PublishResult{
		PackageName:    packageName,
		Version:        version,
		Size:           header.Size,
		Filename:       rpmName,
		Content:        bytes.NewReader(rpmData),
		FileType:       model.FileTypePrimary,
		StorageVersion: storageVersion,
		DownloadURL:    fmt.Sprintf("/Packages/%s/%s", arch, rpmName),
		Metadata:       map[string]interface{}{"repo": ctx.Repo.Name, "arch": arch, "release": release},
		Response: &types.YumPublishResponse{
			PublishResponse: types.PublishResponse{
				Success:  true,
				Message:  "Package published successfully",
				Package:  packageName,
				Version:  version,
				Filename: rpmName,
				Size:     header.Size,
			},
			Repo:       ctx.Repo.Name,
			Arch:       arch,
			Release:    release,
			StorageKey: "",
			PackageId:  0,
		},
	}, nil
}

func (a *YumAdapter) HandleDelete(c *gin.Context, ctx *types.DeleteContext) error {
	fullPath := trimLeadingSlash(c.Param("path"))
	parts := strings.Split(fullPath, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid path: expected name/version")
	}

	name := parts[0]
	version := parts[1]

	identity := &PackageIdentity{
		Name:    name,
		Version: version,
		Type:    YumType,
	}

	if err := a.Delete(context.WithValue(c.Request.Context(), "repo", ctx.Repo), identity); err != nil {
		return err
	}

	return nil
}
