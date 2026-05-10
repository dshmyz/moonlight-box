# 下载流程重构实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 精简包下载流程，消除代码重复，统一职责划分

**架构：** 将下载流程统一到 ProxyDownloadService，Adapter 层只负责协议解析，ProxyRouter 层只负责路由解析，存储、日志、计数等操作在最底层统一处理。

**技术栈：** Go 1.21+, Gin, GORM

---

## 文件结构

**将要修改的文件：**

1. `internal/service/proxy_download_service.go` - 增强为统一下载入口
2. `internal/adapter/base_adapter.go` - 删除冗余方法
3. `internal/adapter/npm_adapter.go` - 简化下载逻辑
4. `internal/adapter/maven_adapter.go` - 简化下载逻辑
5. `internal/adapter/pypi_adapter.go` - 简化下载逻辑
6. `internal/adapter/go_adapter.go` - 简化下载逻辑
7. `internal/adapter/apt_adapter.go` - 简化下载逻辑
8. `internal/adapter/yum_adapter.go` - 简化下载逻辑
9. `internal/adapter/nuget_adapter.go` - 简化下载逻辑
10. `internal/adapter/generic_adapter.go` - 简化下载逻辑

**职责划分：**

- `ProxyDownloadService` - 下载流程编排、存储、日志、计数
- `Adapter` - 协议解析、请求验证
- `ProxyRouter` - 路由解析、缓存策略
- `StorageService` - 文件存储
- `PackageRepository` - 数据库操作

---

## 任务 1：增强 ProxyDownloadService

**文件：**
- 修改：`internal/service/proxy_download_service.go`

**目标：** 增强 ProxyDownloadService，使其成为统一的下载入口，处理所有下载、存储、日志、计数逻辑。

- [ ] **步骤 1：添加 URLBuilder 支持**

在 `ProxyDownloadRequest` 结构体中添加 `URLBuilder` 字段：

```go
type ProxyDownloadRequest struct {
	PkgType        string
	Name           string
	Version        string
	Filename       string
	Repo           *model.Repository
	URLBuilder     proxy.URLBuilder  // 新增
	PackageType    model.PackageType
	RepositoryType model.RepositoryType
	FileType       model.PackageFileType
	Metadata       map[string]interface{}
	ResolutionMode ResolutionMode
	IPAddress      string
	UserAgent      string
	UserID         *uint
	RemoteURL      string
}
```

- [ ] **步骤 2：修改 Download 方法，使用 URLBuilder**

修改 `Download` 方法中的路由解析部分：

```go
func (s *ProxyDownloadService) Download(ctx context.Context, req *ProxyDownloadRequest) (*ProxyDownloadResult, error) {
	startTime := time.Now()

	// 1. 尝试从本地存储获取
	content, size, err := s.storageSvc.GetPackage(ctx, req.PkgType, req.Name, req.Version)
	if err == nil {
		body, readErr := io.ReadAll(content)
		content.Close()
		if readErr == nil {
			s.incrementDownloadCount(req)
			s.recordLog(req, model.DownloadStatusCached, 0, size, int(time.Since(startTime).Milliseconds()), true, nil)
			return &ProxyDownloadResult{
				Content:   body,
				Size:      size,
				FromCache: true,
			}, nil
		}
	}

	if s.proxyRouter == nil {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, proxy.ErrPackageNotFound)
		return nil, proxy.ErrPackageNotFound
	}

	// 2. 从远程仓库下载
	var result *proxy.RouteResult
	var resolveErr error

	switch req.ResolutionMode {
	case ResolutionModeProxyOnly:
		result, resolveErr = s.proxyRouter.ResolveProxyOnlyForRepo(ctx, req.Repo, req.Name, req.Version, req.URLBuilder)
	case ResolutionModeVirtualRepo:
		result, resolveErr = s.proxyRouter.ResolveForVirtualRepo(ctx, req.Repo, req.PkgType, req.Name, req.Version, req.URLBuilder)
	default:
		// 使用 URLBuilder 优先
		if req.URLBuilder != nil && req.Repo != nil {
			result, resolveErr = s.proxyRouter.ResolveProxyOnlyForRepo(ctx, req.Repo, req.Name, req.Version, req.URLBuilder)
		} else {
			result, resolveErr = s.proxyRouter.ResolveSmart(ctx, req.Repo, req.PkgType, req.Name, req.Version, req.URLBuilder)
		}
	}

	if resolveErr != nil {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, resolveErr)
		return nil, resolveErr
	}
	defer result.Content.Close()

	body, readErr := io.ReadAll(result.Content)
	if readErr != nil {
		s.recordLog(req, model.DownloadStatusFailed, 0, 0, int(time.Since(startTime).Milliseconds()), false, readErr)
		return nil, readErr
	}

	// 3. 存储到本地
	storageVersion := req.Version
	if (req.PkgType == "maven" || req.PkgType == "maven2") && req.Filename != "" {
		storageVersion = req.Version + "/" + req.Filename
	}

	storageKey, storeErr := s.storageSvc.StorePackage(ctx, req.PkgType, req.Name, storageVersion, bytes.NewReader(body), result.Size)
	if storeErr != nil {
		logrus.Warnf("failed to store proxy package %s: %v", req.Name, storeErr)
	} else if storageKey != "" {
		// 4. 更新数据库
		_, _, _, dbErr := s.pkgRepo.StorePackageFileAndIncrementDownload(ctx, &model.Package{
			Name:           req.Name,
			Type:           req.PackageType,
			RepositoryID:   result.RepoID,
			RepositoryType: req.RepositoryType,
		}, &model.PackageVersion{
			Version:     req.Version,
			Status:      model.StatusPublished,
			StoragePath: filepath.Dir(storageKey),
		}, &model.PackageFile{
			Filename:    req.Filename,
			FileType:    req.FileType,
			StoragePath: storageKey,
			SizeBytes:   result.Size,
		})
		if dbErr != nil {
			logrus.Warnf("failed to store proxy package file to database %s: %v", req.Name, dbErr)
		}
	}

	// 5. 记录日志和计数
	s.recordLog(req, model.DownloadStatusSuccess, 200, result.Size, int(time.Since(startTime).Milliseconds()), result.FromCache, nil)

	return &ProxyDownloadResult{
		Content:    body,
		Size:       result.Size,
		StorageKey: storageKey,
		FromCache:  result.FromCache,
		RepoID:     result.RepoID,
	}, nil
}
```

- [ ] **步骤 3：运行测试验证**

运行：`go test ./internal/service -v -run TestProxyDownloadService`
预期：PASS（如果测试存在）或无错误

- [ ] **步骤 4：Commit**

```bash
git add internal/service/proxy_download_service.go
git commit -m "refactor: enhance ProxyDownloadService to support URLBuilder and unified download flow"
```

---

## 任务 2：重构 NPM 适配器

**文件：**
- 修改：`internal/adapter/npm_adapter.go`

**目标：** 简化 NPM 适配器的下载逻辑，统一使用 ProxyDownloadService。

- [ ] **步骤 1：修改 downloadTarballForRepo 方法**

找到 `downloadTarballForRepo` 方法，替换为：

```go
func (a *NpmAdapter) downloadTarballForRepo(c *gin.Context, repo *model.Repository, path string) {
	parts := strings.Split(strings.Trim(path, "/"), "/-/")
	if len(parts) != 2 {
		response.NotFound(c, "invalid tarball path")
		return
	}

	pkgPath := parts[0]
	filename := parts[1]

	pkgParts := strings.Split(pkgPath, "/")
	var name string
	if len(pkgParts) >= 2 && strings.HasPrefix(pkgParts[0], "@") {
		name = strings.Join(pkgParts[:2], "/")
	} else {
		name = pkgParts[len(pkgParts)-1]
	}

	version := strings.TrimSuffix(filename, ".tgz")
	version = strings.SplitN(version, "-", 2)[0]
	if len(strings.SplitN(version, "-", 2)) > 1 {
		versionParts := strings.SplitN(version, "-", 2)
		if len(versionParts) > 1 {
			version = versionParts[0]
		}
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeNPM, name, version, filename)
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
		baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
		return fmt.Sprintf("%s/%s/-/%s", baseURL, pkgName, filename)
	}

	result, err := a.proxyDownloadSvc.Download(c.Request.Context(), &service.ProxyDownloadRequest{
		PkgType:        "npm",
		Name:           name,
		Version:        version,
		Filename:       filename,
		Repo:           repo,
		URLBuilder:     urlBuilder,
		PackageType:    model.PackageTypeNPM,
		RepositoryType: repo.Type,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeSmart,
		IPAddress:      c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		UserID:         getUintPtr(c.GetUint("userID")),
	})

	if err != nil {
		response.NotFound(c, "package not found")
		return
	}

	contentType := "application/octet-stream"
	c.Data(200, contentType, result.Content)
}
```

- [ ] **步骤 2：添加辅助函数**

在文件末尾添加：

```go
func getUintPtr(val uint) *uint {
	if val == 0 {
		return nil
	}
	return &val
}
```

- [ ] **步骤 3：删除旧的下载逻辑**

删除 `getPackageForRepo` 方法中调用 `DownloadFromProxyAndCache` 的代码，替换为：

```go
func (a *NpmAdapter) getPackageForRepo(c *gin.Context, repo *model.Repository, path string) {
	name := strings.Trim(path, "/")

	if strings.Contains(name, "/-/") {
		a.downloadTarballForRepo(c, repo, name)
		return
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeNPM, name, "", "")
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	pkg, err := a.pkgRepo.FindByNameAndType(name, model.PackageTypeNPM)
	if err != nil {
		if repo.Type == model.RepoTypeProxy && a.proxyRouter != nil {
			urlBuilder := func(repo *model.Repository, pkgName, _ string) string {
				baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
				return fmt.Sprintf("%s/%s", baseURL, pkgName)
			}

			result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "npm", name, "", urlBuilder)
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

	metadata := a.buildNpmMetadata(pkg)
	c.JSON(200, metadata)
}
```

- [ ] **步骤 4：运行测试验证**

运行：`go test ./internal/adapter -v -run TestNpmAdapter`
预期：PASS（如果测试存在）或无错误

- [ ] **步骤 5：Commit**

```bash
git add internal/adapter/npm_adapter.go
git commit -m "refactor: simplify NPM adapter to use ProxyDownloadService"
```

---

## 任务 3：重构 Maven 适配器

**文件：**
- 修改：`internal/adapter/maven_adapter.go`

**目标：** 简化 Maven 适配器的下载逻辑，统一使用 ProxyDownloadService。

- [ ] **步骤 1：修改 handleDownloadArtifact 方法**

找到 `handleDownloadArtifact` 方法，替换为：

```go
func (a *MavenAdapter) handleDownloadArtifact(c *gin.Context, fullPath string) {
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.NotFound(c, "artifact not found")
		return
	}

	artifactParts := parts[:len(parts)-1]
	filename := parts[len(parts)-1]

	var groupID, artifactID, version string
	for i := 0; i < len(artifactParts); i++ {
		if artifactParts[i] != "" {
			if groupID == "" {
				groupID = artifactParts[i]
			} else if artifactID == "" {
				artifactID = artifactParts[i]
			} else if version == "" {
				version = artifactParts[i]
				break
			}
		}
	}

	if groupID == "" || artifactID == "" || version == "" {
		response.NotFound(c, "artifact not found")
		return
	}

	groupID = strings.Join(strings.Split(groupID, "."), "/")
	name := groupID + "/" + artifactID

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	decision := a.CheckDownloadPermission(c, repo, model.PackageTypeMaven, name, version, filename)
	if !decision.Allow {
		c.JSON(decision.Code, gin.H{"error": decision.Message})
		return
	}

	urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
		baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
		return fmt.Sprintf("%s/%s/%s/%s", baseURL, groupID, artifactID, filename)
	}

	result, err := a.proxyDownloadSvc.Download(c.Request.Context(), &service.ProxyDownloadRequest{
		PkgType:        "maven",
		Name:           name,
		Version:        version,
		Filename:       filename,
		Repo:           repo,
		URLBuilder:     urlBuilder,
		PackageType:    model.PackageTypeMaven,
		RepositoryType: repo.Type,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeSmart,
		IPAddress:      c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		UserID:         getUintPtr(c.GetUint("userID")),
	})

	if err != nil {
		response.NotFound(c, "artifact not found")
		return
	}

	contentType := "application/java-archive"
	if strings.HasSuffix(filename, ".pom") {
		contentType = "application/xml"
	} else if strings.HasSuffix(filename, ".xml") {
		contentType = "application/xml"
	}

	c.Data(200, contentType, result.Content)
}
```

- [ ] **步骤 2：运行测试验证**

运行：`go test ./internal/adapter -v -run TestMavenAdapter`
预期：PASS（如果测试存在）或无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "refactor: simplify Maven adapter to use ProxyDownloadService"
```

---

## 任务 4：重构 PyPI 适配器

**文件：**
- 修改：`internal/adapter/pypi_adapter.go`

**目标：** 简化 PyPI 适配器的下载逻辑，统一使用 ProxyDownloadService。

- [ ] **步骤 1：修改 DownloadPackage 方法**

找到 `DownloadPackage` 方法，替换为：

```go
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

	urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
		baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
		if strings.Contains(baseURL, "/simple") {
			baseURL = strings.TrimSuffix(baseURL, "/simple")
		}
		return fmt.Sprintf("%s/packages/%s", baseURL, actualFilename)
	}

	result, err := a.proxyDownloadSvc.Download(c.Request.Context(), &service.ProxyDownloadRequest{
		PkgType:        "pypi",
		Name:           name,
		Version:        version,
		Filename:       actualFilename,
		Repo:           repo,
		URLBuilder:     urlBuilder,
		PackageType:    model.PackageTypePyPI,
		RepositoryType: repo.Type,
		FileType:       model.FileTypePrimary,
		ResolutionMode: service.ResolutionModeSmart,
		IPAddress:      c.ClientIP(),
		UserAgent:      c.Request.UserAgent(),
		UserID:         getUintPtr(c.GetUint("userID")),
	})

	if err != nil {
		response.NotFound(c, "package not found")
		return
	}

	contentType := "application/octet-stream"
	c.Data(200, contentType, result.Content)
}
```

- [ ] **步骤 2：运行测试验证**

运行：`go test ./internal/adapter -v -run TestPyPIAdapter`
预期：PASS（如果测试存在）或无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/pypi_adapter.go
git commit -m "refactor: simplify PyPI adapter to use ProxyDownloadService"
```

---

## 任务 5：重构其他适配器

**文件：**
- 修改：`internal/adapter/go_adapter.go`
- 修改：`internal/adapter/apt_adapter.go`
- 修改：`internal/adapter/yum_adapter.go`
- 修改：`internal/adapter/nuget_adapter.go`
- 修改：`internal/adapter/generic_adapter.go`

**目标：** 统一所有适配器的下载逻辑。

- [ ] **步骤 1：重构 Go 适配器**

找到下载相关方法，替换为使用 `ProxyDownloadService.Download`。

- [ ] **步骤 2：重构 APT 适配器**

找到下载相关方法，替换为使用 `ProxyDownloadService.Download`。

- [ ] **步骤 3：重构 YUM 适配器**

找到下载相关方法，替换为使用 `ProxyDownloadService.Download`。

- [ ] **步骤 4：重构 NuGet 适配器**

找到下载相关方法，替换为使用 `ProxyDownloadService.Download`。

- [ ] **步骤 5：重构 Generic 适配器**

找到下载相关方法，替换为使用 `ProxyDownloadService.Download`。

- [ ] **步骤 6：运行测试验证**

运行：`go test ./internal/adapter -v`
预期：PASS（如果测试存在）或无错误

- [ ] **步骤 7：Commit**

```bash
git add internal/adapter/go_adapter.go internal/adapter/apt_adapter.go internal/adapter/yum_adapter.go internal/adapter/nuget_adapter.go internal/adapter/generic_adapter.go
git commit -m "refactor: simplify all adapters to use ProxyDownloadService"
```

---

## 任务 6：删除 BaseAdapter 冗余方法

**文件：**
- 修改：`internal/adapter/base_adapter.go`

**目标：** 删除 BaseAdapter 中的冗余下载方法，统一使用 ProxyDownloadService。

- [ ] **步骤 1：删除 DownloadFromProxyAndCache 方法**

删除 `DownloadFromProxyAndCache` 方法及其相关代码（约 50 行）。

- [ ] **步骤 2：删除 recordProxyDownloadLog 方法**

删除 `recordProxyDownloadLog` 方法（约 30 行）。

- [ ] **步骤 3：删除 IncrementDownloadCountForPackage 方法**

删除 `IncrementDownloadCountForPackage` 和 `incrementDownloadCountAsync` 方法（约 20 行）。

- [ ] **步骤 4：删除 StoreProxyPackage 相关方法**

删除以下方法：
- `StoreProxyPackage`
- `StoreProxyPackageFromReader`
- `StoreProxyPackageFromResult`

这些方法现在由 ProxyDownloadService 统一处理。

- [ ] **步骤 5：简化适配器字段**

修改所有适配器的构造函数，移除重复的依赖注入。确保所有适配器只通过 BaseAdapter 访问公共依赖。

- [ ] **步骤 6：运行测试验证**

运行：`go test ./internal/adapter -v`
预期：PASS（如果测试存在）或无错误

- [ ] **步骤 7：Commit**

```bash
git add internal/adapter/base_adapter.go internal/adapter/npm_adapter.go internal/adapter/maven_adapter.go internal/adapter/pypi_adapter.go
git commit -m "refactor: remove redundant methods from BaseAdapter and simplify adapter structure"
```

---

## 任务 7：运行完整测试验证

**目标：** 确保所有修改后的代码能够正常工作。

- [ ] **步骤 1：运行单元测试**

运行：`go test ./internal/... -v -cover`
预期：所有测试通过

- [ ] **步骤 2：运行集成测试**

运行：`go test ./tests/e2e/... -v`
预期：所有集成测试通过

- [ ] **步骤 3：手动测试 NPM 下载**

```bash
# 启动服务
go run cmd/registry/main.go

# 测试 NPM 包下载
curl -I http://localhost:8080/npm/lodash/-/lodash-4.17.21.tgz
```
预期：返回 200 OK

- [ ] **步骤 4：手动测试 Maven 下载**

```bash
# 测试 Maven 包下载
curl -I http://localhost:8080/maven2/org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar
```
预期：返回 200 OK

- [ ] **步骤 5：手动测试 PyPI 下载**

```bash
# 测试 PyPI 包下载
curl -I http://localhost:8080/pypi/packages/requests-2.28.0-py3-none-any.whl
```
预期：返回 200 OK

- [ ] **步骤 6：验证数据库记录**

检查数据库中的 `proxy_download_logs` 表，确认日志记录正常。

- [ ] **步骤 7：验证下载计数**

检查 `packages`、`package_versions`、`package_files` 表中的 `download_count` 字段，确认计数正常。

- [ ] **步骤 8：Final Commit**

```bash
git add .
git commit -m "refactor: complete download flow refactoring - unified ProxyDownloadService"
```

---

## 预期成果

完成此重构后，将实现：

1. **代码量减少**：删除约 200+ 行重复代码
2. **调用层级简化**：从 7-8 层减少到 4-5 层
3. **职责清晰**：每一层只做自己该做的事
4. **维护成本降低**：修改下载逻辑只需改一处
5. **Bug 风险降低**：统一处理，不易遗漏

## 风险和注意事项

1. **向后兼容**：确保所有现有的 API 端点行为不变
2. **性能影响**：监控重构后的性能，确保没有性能退化
3. **错误处理**：确保所有错误都能正确传递和处理
4. **日志完整性**：确保所有下载操作都有完整的日志记录
5. **测试覆盖**：如果测试不足，需要补充测试用例
