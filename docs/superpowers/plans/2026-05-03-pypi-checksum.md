# PyPI校验和文件实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为PyPI适配器添加SHA256校验和文件支持，在上传时计算并存储校验和，在请求时返回校验和。

**架构：** 在PyPI适配器中添加校验和计算和存储功能，利用现有PackageFile模型的ChecksumSHA256字段，支持本地仓库和代理仓库。

**技术栈：** Go、Gin、GORM、crypto/sha256、encoding/hex

---

## 文件结构

**修改文件：**
- `internal/adapter/pypi_adapter.go` - PyPI适配器主文件
  - 添加crypto导入
  - 修改UploadPackage方法
  - 添加handleChecksumRequest方法
  - 修改DownloadPackage方法

- `internal/repository/package_repo.go` - 包仓库文件
  - 添加FindFilesByFilename方法

---

## 任务 1：添加必要的导入

**文件：**
- 修改：`internal/adapter/pypi_adapter.go:1-23`

- [ ] **步骤 1：添加crypto和encoding导入**

在文件开头的import块中添加必要的导入：

```go
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
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/proxy"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/response"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/util"

	"github.com/gin-gonic/gin"
)
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/pypi_adapter.go
git commit -m "feat(pypi): add crypto imports for SHA256 checksum"
```

---

## 任务 2：添加FindFilesByFilename方法

**文件：**
- 修改：`internal/repository/package_repo.go`（在文件末尾添加）

- [ ] **步骤 1：添加FindFilesByFilename方法**

在文件末尾添加新方法：

```go
func (r *PackageRepository) FindFilesByFilename(filename string) ([]model.PackageFile, error) {
	var files []model.PackageFile
	err := r.db.Where("filename = ?", filename).Find(&files).Error
	if err != nil {
		return nil, err
	}
	return files, nil
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/repository/package_repo.go
git commit -m "feat(pypi): add FindFilesByFilename method for checksum lookup"
```

---

## 任务 3：修改UploadPackage方法计算并存储SHA256

**文件：**
- 修改：`internal/adapter/pypi_adapter.go`（UploadPackage方法）

- [ ] **步骤 1：在UploadPackage方法中添加SHA256计算**

找到UploadPackage方法，在读取文件内容后添加SHA256计算：

```go
func (a *PyPIAdapter) UploadPackage(c *gin.Context) {
	// ... 现有的解析逻辑 ...

	// 读取文件内容
	content, err := io.ReadAll(file)
	if err != nil {
		response.InternalError(c, "failed to read file")
		return
	}

	// 计算SHA256
	hash := sha256.Sum256(content)
	checksum := hex.EncodeToString(hash[:])

	// 存储文件
	storagePath, err := a.storageSvc.StorePackage(c.Request.Context(), "pypi", pkgName, version, bytes.NewReader(content), int64(len(content)))
	if err != nil {
		response.InternalError(c, "failed to store package")
		return
	}

	// 创建 PackageFile 记录，包含校验和
	_, _, err = a.pkgRepo.StorePackageFile(c.Request.Context(), &model.Package{
		Name:         pkgName,
		Type:         model.PackageTypePyPI,
		RepositoryID: repo.ID,
	}, &model.PackageVersion{
		Version: version,
		Status:  model.StatusPublished,
	}, &model.PackageFile{
		Filename:       filename,
		FileType:       getPyPIFileType(filename),
		StoragePath:    storagePath,
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: checksum,
	})

	// ... 其余逻辑 ...
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/pypi_adapter.go
git commit -m "feat(pypi): calculate and store SHA256 checksum on upload"
```

---

## 任务 4：添加handleChecksumRequest方法

**文件：**
- 修改：`internal/adapter/pypi_adapter.go`（在DownloadPackage方法后添加）

- [ ] **步骤 1：添加handleChecksumRequest方法**

在DownloadPackage方法后添加新方法：

```go
func (a *PyPIAdapter) handleChecksumRequest(c *gin.Context, filename string) {
	// 解析文件名，获取实际文件名
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	// 尝试从数据库查询文件
	files, err := a.pkgRepo.FindFilesByFilename(actualFilename)
	if err == nil && len(files) > 0 {
		// 检查是否有缓存的校验和
		for _, file := range files {
			if file.ChecksumSHA256 != "" {
				c.String(200, file.ChecksumSHA256)
				return
			}
		}
	}

	// 如果没有缓存，从存储获取文件并计算
	content, _, err := a.storageSvc.GetPackage(c.Request.Context(), "pypi", actualFilename, "")
	if err == nil {
		defer content.Close()
		body, _ := io.ReadAll(content)
		hash := sha256.Sum256(body)
		checksum := hex.EncodeToString(hash[:])
		c.String(200, checksum)
		return
	}

	// 尝试从代理获取
	if a.proxyRouter != nil {
		var repo *model.Repository
		if r, ok := c.Get("repo"); ok {
			repo = r.(*model.Repository)
		}

		if repo != nil {
			urlBuilder := func(repo *model.Repository, pkgName, pkgVersion string) string {
				baseURL := strings.TrimSuffix(repo.RemoteURL, "/")
				return fmt.Sprintf("%s/packages/%s", baseURL, actualFilename)
			}

			result, resolveErr := a.proxyRouter.ResolveSmart(c.Request.Context(), repo, "pypi", actualFilename, "", urlBuilder)
			if resolveErr == nil && result != nil {
				defer result.Content.Close()
				body, readErr := io.ReadAll(result.Content)
				if readErr == nil {
					hash := sha256.Sum256(body)
					checksum := hex.EncodeToString(hash[:])
					c.String(200, checksum)
					return
				}
			}
		}
	}

	response.NotFound(c, "file not found")
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/pypi_adapter.go
git commit -m "feat(pypi): add handleChecksumRequest method for checksum files"
```

---

## 任务 5：修改DownloadPackage方法支持校验文件

**文件：**
- 修改：`internal/adapter/pypi_adapter.go`（DownloadPackage方法）

- [ ] **步骤 1：在DownloadPackage方法开头添加校验文件检测**

找到DownloadPackage方法，在开头添加校验文件检测：

```go
func (a *PyPIAdapter) DownloadPackage(c *gin.Context) {
	filename := c.Param("filename")

	// 检查是否是校验文件请求
	if strings.HasSuffix(filename, ".sha256") {
		a.handleChecksumRequest(c, filename)
		return
	}

	// 原有的下载逻辑...
	// ... 其余代码保持不变 ...
}
```

- [ ] **步骤 2：验证编译通过**

运行：`go build ./cmd/registry`
预期：编译成功，无错误

- [ ] **步骤 3：Commit**

```bash
git add internal/adapter/pypi_adapter.go
git commit -m "feat(pypi): integrate checksum handling in DownloadPackage"
```

---

## 任务 6：测试校验和功能

**文件：**
- 无需修改文件，进行集成测试

- [ ] **步骤 1：启动后端服务**

运行：`go run cmd/registry/main.go`
预期：服务启动成功，监听9081端口

- [ ] **步骤 2：上传测试文件**

运行：
```bash
# 先登录获取token
curl -X POST "http://localhost:9081/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}'

# 创建测试包文件
echo "test package content" > test-package-1.0.0.whl

# 上传测试文件
curl -X POST "http://localhost:9081/pypi/simple/" \
  -H "Authorization: Bearer <token>" \
  -F "file=@test-package-1.0.0.whl"
```
预期：返回200状态码

- [ ] **步骤 3：测试SHA256校验和**

运行：
```bash
curl "http://localhost:9081/pypi/packages/test-package-1.0.0.whl.sha256"
```
预期：返回SHA256校验和（64位十六进制字符串）

- [ ] **步骤 4：验证校验和正确性**

手动计算文件的SHA256：
```bash
sha256sum test-package-1.0.0.whl
```
对比返回值是否一致。

- [ ] **步骤 5：测试代理仓库的校验和**

运行：
```bash
curl "http://localhost:9081/pypi-proxy/packages/requests-2.28.0.whl.sha256"
```
预期：返回正确的SHA256校验和

---

## 任务 7：最终验证和提交

**文件：**
- 无需修改代码文件

- [ ] **步骤 1：运行完整测试流程**

执行完整的测试流程：
1. 上传文件
2. 下载文件
3. 获取校验和
4. 验证所有功能正常

- [ ] **步骤 2：验证数据库存储**

检查数据库中的校验和是否正确存储：
```bash
sqlite3 ./data/registry.db "SELECT filename, checksum_sha256 FROM package_files WHERE filename LIKE '%.whl' LIMIT 5;"
```
预期：看到校验和已存储

- [ ] **步骤 3：最终Commit**

```bash
git add -A
git commit -m "feat(pypi): complete SHA256 checksum implementation with tests"
```

---

## 注意事项

1. **性能考虑**：
   - 上传时计算并缓存，避免重复计算
   - 数据库查询快速高效
   - 对于大文件，计算时间可接受

2. **错误处理**：
   - 所有错误都有明确的返回码和消息
   - 代理失败时返回404
   - 文件不存在时返回404

3. **兼容性**：
   - 校验文件格式符合PyPI标准
   - 支持本地仓库和代理仓库
   - 不影响现有功能

4. **测试覆盖**：
   - 测试本地仓库和代理仓库
   - 测试缓存机制
   - 测试错误场景
