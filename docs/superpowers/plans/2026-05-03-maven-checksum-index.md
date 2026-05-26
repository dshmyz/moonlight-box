# Maven校验文件和仓库索引实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为Maven适配器添加校验文件（.sha1/.md5）和仓库索引功能

**架构：** 在现有Maven适配器中添加校验文件处理和索引生成功能，动态计算校验和，提供基础索引端点

**技术栈：** Go、Gin、GORM、crypto/sha1、crypto/md5

---

## 文件结构

**修改文件：**
- `internal/adapter/maven_adapter.go` - Maven适配器主文件
  - 添加校验文件处理方法
  - 添加索引生成方法
  - 添加辅助方法
  - 修改路由注册

---

## 任务 1：添加必要的导入

**文件：**
- 修改：`internal/adapter/maven_adapter.go:1-22`

- [ ] **步骤 1：添加crypto和encoding导入**

在文件开头的import块中添加必要的导入：

```go
import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/proxy"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"

	"github.com/gin-gonic/gin"
)
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): add crypto imports for checksum calculation"
```

---

## 任务 2：添加校验和计算辅助方法

**文件：**
- 修改：`internal/adapter/maven_adapter.go`（在文件末尾添加）

- [ ] **步骤 1：添加calculateChecksum方法**

在文件末尾添加校验和计算方法：

```go
func calculateChecksum(data []byte, checksumType string) string {
	if checksumType == "sha1" {
		hash := sha1.Sum(data)
		return hex.EncodeToString(hash[:])
	}
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): add calculateChecksum helper method"
```

---

## 任务 3：添加校验文件处理方法

**文件：**
- 修改：`internal/adapter/maven_adapter.go`（在handleDownloadArtifact方法后添加）

- [ ] **步骤 1：添加handleChecksumRequest方法**

在handleDownloadArtifact方法后添加新方法：

```go
func (a *MavenAdapter) handleChecksumRequest(c *gin.Context, fullPath string) {
	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.NotFound(c, "checksum not found")
		return
	}

	filename := parts[len(parts)-1]

	var checksumType string
	var actualFilename string

	if strings.HasSuffix(filename, ".sha1") {
		checksumType = "sha1"
		actualFilename = strings.TrimSuffix(filename, ".sha1")
	} else if strings.HasSuffix(filename, ".md5") {
		checksumType = "md5"
		actualFilename = strings.TrimSuffix(filename, ".md5")
	} else {
		response.NotFound(c, "checksum not found")
		return
	}

	groupArtifact := strings.Join(parts[:len(parts)-2], "/")
	version := parts[len(parts)-2]
	storageVersion := version + "/" + actualFilename

	content, _, err := a.storageSvc.GetPackage(c.Request.Context(), "maven", groupArtifact, storageVersion)

	if err != nil {
		if a.proxyRouter != nil {
			urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
				baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
				return fmt.Sprintf("%s/%s/%s/%s", baseURL, groupArtifact, version, actualFilename)
			}

			var repo *model.Repository
			if r, ok := c.Get("repo"); ok {
				repo = r.(*model.Repository)
			}

			result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "maven", groupArtifact, version, urlBuilder)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					checksum := calculateChecksum(body, checksumType)
					c.String(200, "%s  %s", checksum, actualFilename)
					return
				}
			}
		}
		response.NotFound(c, "file not found")
		return
	}
	defer content.Close()

	body, _ := io.ReadAll(content)
	checksum := calculateChecksum(body, checksumType)

	c.String(200, "%s  %s", checksum, actualFilename)
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): add handleChecksumRequest method for checksum files"
```

---

## 任务 4：修改handleDownloadArtifact方法支持校验文件

**文件：**
- 修改：`internal/adapter/maven_adapter.go:284-359`

- [ ] **步骤 1：在handleDownloadArtifact开头添加校验文件检测**

修改handleDownloadArtifact方法，在开头添加校验文件检测：

```go
func (a *MavenAdapter) handleDownloadArtifact(c *gin.Context, fullPath string) {
	// 检查是否是校验文件请求
	if strings.HasSuffix(fullPath, ".sha1") || strings.HasSuffix(fullPath, ".md5") {
		a.handleChecksumRequest(c, fullPath)
		return
	}

	parts := strings.Split(fullPath, "/")
	if len(parts) < 4 {
		response.NotFound(c, "artifact not found")
		return
	}

	// ... 其余代码保持不变
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): integrate checksum handling in handleDownloadArtifact"
```

---

## 任务 5：添加索引数据结构

**文件：**
- 修改：`internal/adapter/maven_adapter.go`（在MavenSnapshotVersion结构体后添加）

- [ ] **步骤 1：添加索引数据结构**

在现有数据结构后添加：

```go
type MavenPackageIndex struct {
	XMLName     xml.Name              `xml:"index"`
	Repository  string                `xml:"repository" json:"repository"`
	GeneratedAt string                `xml:"generatedAt" json:"generatedAt"`
	Packages    []MavenPackageSummary `xml:"packages>package" json:"packages"`
}

type MavenPackageSummary struct {
	GroupID    string   `xml:"groupId" json:"groupId"`
	ArtifactID string   `xml:"artifactId" json:"artifactId"`
	Versions   []string `xml:"versions>version" json:"versions"`
	Latest     string   `xml:"latest" json:"latest"`
	Release    string   `xml:"release" json:"release"`
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): add MavenPackageIndex data structures"
```

---

## 任务 6：添加索引生成方法

**文件：**
- 修改：`internal/adapter/maven_adapter.go`（在generateIndex方法位置添加）

- [ ] **步骤 1：添加generateIndex方法**

在文件中添加索引生成方法：

```go
func (a *MavenAdapter) generateIndex(packages []model.Package, repoName string) *MavenPackageIndex {
	index := &MavenPackageIndex{
		Repository:  repoName,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Packages:    make([]MavenPackageSummary, 0),
	}

	for _, pkg := range packages {
		parts := strings.Split(pkg.Name, "/")
		if len(parts) < 2 {
			continue
		}

		groupID := parts[0]
		artifactID := parts[1]

		versions := make([]string, 0, len(pkg.Versions))
		var latest, release string

		for _, ver := range pkg.Versions {
			versions = append(versions, ver.Version)

			if latest == "" || compareVersions(ver.Version, latest) > 0 {
				latest = ver.Version
			}

			if isRelease(ver.Version) {
				if release == "" || compareVersions(ver.Version, release) > 0 {
					release = ver.Version
				}
			}
		}

		index.Packages = append(index.Packages, MavenPackageSummary{
			GroupID:    groupID,
			ArtifactID: artifactID,
			Versions:   versions,
			Latest:     latest,
			Release:    release,
		})
	}

	return index
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): add generateIndex method for repository indexing"
```

---

## 任务 7：添加索引请求处理方法

**文件：**
- 修改：`internal/adapter/maven_adapter.go`（在generateIndex方法后添加）

- [ ] **步骤 1：添加handleIndexRequest方法**

添加索引请求处理方法：

```go
func (a *MavenAdapter) handleIndexRequest(c *gin.Context) {
	repoName := c.Param("repoName")

	var repo *model.Repository
	if r, ok := c.Get("repo"); ok {
		repo = r.(*model.Repository)
	}

	if repo == nil {
		response.NotFound(c, "repository not found")
		return
	}

	var packages []model.Package
	err := a.pkgRepo.DB().
		Preload("Versions").
		Where("repository_id = ?", repo.ID).
		Find(&packages).
		Error

	if err != nil {
		response.InternalError(c, "failed to query packages")
		return
	}

	index := a.generateIndex(packages, repo.Name)

	accept := c.GetHeader("Accept")
	if strings.Contains(accept, "application/xml") {
		c.XML(200, index)
	} else {
		c.JSON(200, index)
	}
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): add handleIndexRequest method for index endpoint"
```

---

## 任务 8：注册索引路由

**文件：**
- 修改：`internal/adapter/maven_adapter.go:105-115`

- [ ] **步骤 1：在RegisterRoutes方法中添加索引路由**

修改RegisterRoutes方法，添加索引路由：

```go
func (a *MavenAdapter) RegisterRoutes(r *gin.RouterGroup, authMw gin.HandlerFunc, permMw func(resource, action string) gin.HandlerFunc) {
	repo := r.Group("/repo/:repoName")
	repo.GET("/*path", a.handleRequest)
	repo.PUT("/*path", authMw, permMw("repositories", "write"), a.HandleRepoPublish)
	repo.DELETE("/*path", authMw, permMw("repositories", "write"), a.HandleRepoDelete)
	
	// 添加索引端点
	repo.GET("/index", a.handleIndexRequest)
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/maven_adapter.go
git commit -m "feat(maven): register index endpoint in RegisterRoutes"
```

---

## 任务 9：测试校验文件功能

**文件：**
- 无需修改文件，进行集成测试

- [ ] **步骤 1：启动后端服务**

运行：`go run cmd/registry/main.go`
预期：服务启动成功，监听9081端口

- [ ] **步骤 2：上传测试文件**

运行：
```bash
curl -X PUT "http://localhost:9081/repo/maven-local/com/test/lib/1.0.0/lib-1.0.0.jar" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/octet-stream" \
  --data-binary "test content"
```
预期：返回200状态码

- [ ] **步骤 3：测试SHA1校验和**

运行：
```bash
curl "http://localhost:9081/repo/maven-local/com/test/lib/1.0.0/lib-1.0.0.jar.sha1"
```
预期：返回SHA1校验和，格式为 `<sha1-hash>  lib-1.0.0.jar`

- [ ] **步骤 4：测试MD5校验和**

运行：
```bash
curl "http://localhost:9081/repo/maven-local/com/test/lib/1.0.0/lib-1.0.0.jar.md5"
```
预期：返回MD5校验和，格式为 `<md5-hash>  lib-1.0.0.jar`

- [ ] **步骤 5：验证校验和正确性**

手动计算文件的SHA1和MD5，与返回值对比，确认一致。

---

## 任务 10：测试仓库索引功能

**文件：**
- 无需修改文件，进行集成测试

- [ ] **步骤 1：测试JSON格式索引**

运行：
```bash
curl "http://localhost:9081/repo/maven-local/index" \
  -H "Accept: application/json"
```
预期：返回JSON格式的索引数据，包含包列表

- [ ] **步骤 2：测试XML格式索引**

运行：
```bash
curl "http://localhost:9081/repo/maven-local/index" \
  -H "Accept: application/xml"
```
预期：返回XML格式的索引数据，包含包列表

- [ ] **步骤 3：验证索引数据正确性**

检查返回的索引数据是否包含正确的：
- groupId
- artifactId
- versions列表
- latest版本
- release版本

---

## 任务 11：最终验证和文档更新

**文件：**
- 无需修改代码文件

- [ ] **步骤 1：运行完整测试流程**

执行完整的测试流程：
1. 上传文件
2. 下载文件
3. 获取校验和
4. 获取索引
5. 验证所有功能正常

- [ ] **步骤 2：验证代理仓库的校验和功能**

测试代理仓库的校验和获取：
```bash
curl "http://localhost:9081/repo/maven-proxy-aliyun/commons-lang/commons-lang/2.6/commons-lang-2.6.jar.sha1"
```
预期：返回正确的SHA1校验和

- [ ] **步骤 3：最终Commit**

```bash
git add -A
git commit -m "feat(maven): complete checksum and index implementation with tests"
```

---

## 注意事项

1. **性能考虑**：
   - 校验和动态计算，对于大文件可能需要优化
   - 索引查询可能需要添加缓存机制

2. **错误处理**：
   - 所有错误都有明确的返回码和消息
   - 代理失败时返回404

3. **兼容性**：
   - 校验文件格式符合Maven标准
   - 索引格式支持JSON和XML

4. **测试覆盖**：
   - 测试本地仓库和代理仓库
   - 测试不同格式的索引
   - 测试错误场景
