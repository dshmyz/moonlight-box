# PyPI校验和文件功能设计

## 概述

为PyPI适配器添加SHA256校验和文件支持，提升PyPI仓库的完整性和安全性。

## 背景

当前PyPI适配器已经实现了核心功能：
- 包上传（POST）
- 包下载
- Simple API（HTML）
- JSON API
- 包列表

但缺少以下重要功能：
1. **校验和文件**：pip安装时会验证SHA256校验和，缺失会导致警告
2. **安全性**：无法验证下载文件的完整性

## 目标

- 动态生成 .sha256 校验文件
- 支持本地仓库和代理仓库
- 符合PyPI标准格式
- 利用现有模型存储校验和

## 非目标

- 其他校验和算法（MD5、SHA384、SHA512）
- 元数据同步功能
- 搜索API功能

## 设计细节

### 架构设计

**核心思路**：在PyPI适配器中添加校验和文件处理功能，利用现有的 `PackageFile` 模型存储SHA256校验和。

**处理流程**：
```
上传文件
   ↓
计算SHA256
   ↓
存储到数据库
   ↓
请求.sha256文件
   ↓
┌──┴──┐
│     │
有缓存 无缓存
│     │
↓     ↓
返回  实时计算
校验和 并返回
```

### 混合模式实现

**上传时：**
1. 读取文件内容
2. 计算SHA256校验和
3. 存储到 `PackageFile.ChecksumSHA256` 字段

**请求时：**
1. 先查询数据库中的校验和
2. 如果有缓存，直接返回
3. 如果没有缓存，从存储获取文件并实时计算

### 实现细节

#### 1. 添加必要的导入

在 `internal/adapter/pypi_adapter.go` 文件开头添加：

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

#### 2. 修改 `UploadPackage` 方法

在上传文件时计算SHA256并存储：

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
		Name: pkgName,
		Type: model.PackageTypePyPI,
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

#### 3. 添加 `handleChecksumRequest` 方法

```go
func (a *PyPIAdapter) handleChecksumRequest(c *gin.Context, filename string) {
	// 解析文件名，获取实际文件名
	actualFilename := strings.TrimSuffix(filename, ".sha256")

	// 从路径中提取包名和版本
	// 路径格式: /packages/{hash}/{package}-{version}.{ext}.sha256
	// 或简单格式: /packages/{package}-{version}.{ext}.sha256

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
	// 尝试从本地存储获取
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
		// 构建远程URL
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

#### 4. 修改 `DownloadPackage` 方法

在方法开头添加校验文件检测：

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

#### 5. 辅助方法

添加辅助方法来查找文件：

```go
// FindFilesByFilename 根据文件名查找文件
func (r *PackageRepository) FindFilesByFilename(filename string) ([]model.PackageFile, error) {
	var files []model.PackageFile
	err := r.db.Where("filename = ?", filename).Find(&files).Error
	if err != nil {
		return nil, err
	}
	return files, nil
}
```

### API端点

```bash
# 上传文件时自动计算SHA256
POST /pypi/simple/
Content-Type: multipart/form-data
file: package.whl

# 请求SHA256校验和
GET /pypi/packages/{hash}/package-1.0.0.whl.sha256
返回: <sha256-hash>

# 或简单格式
GET /pypi/packages/package-1.0.0.whl.sha256
返回: <sha256-hash>
```

### 数据模型

利用现有的 `PackageFile` 模型：

```go
type PackageFile struct {
	BaseModel
	VersionID     uint              `gorm:"not null;index" json:"version_id"`
	Filename      string            `gorm:"not null;size:255;index" json:"filename"`
	FileType      PackageFileType   `gorm:"not null;size:50" json:"file_type"`
	StoragePath   string            `gorm:"not null;size:500" json:"storage_path"`
	SizeBytes     int64             `gorm:"not null" json:"size_bytes"`
	ChecksumMD5   string            `gorm:"size:32" json:"checksum_md5,omitempty"`
	ChecksumSHA256 string           `gorm:"size:64" json:"checksum_sha256,omitempty"` // ✅ 已有字段
	// ... 其他字段 ...
}
```

### 错误处理

- 文件不存在：返回 404
- 计算失败：返回 500
- 代理失败：返回 404

### 测试场景

```bash
# 1. 上传文件
POST /pypi/simple/
file: test-package-1.0.0.whl

# 2. 请求SHA256校验和
GET /pypi/packages/test-package-1.0.0.whl.sha256
预期: 返回SHA256校验和（64位十六进制字符串）

# 3. 验证校验和正确性
# 手动计算文件的SHA256
sha256sum test-package-1.0.0.whl
# 对比返回值是否一致

# 4. 测试代理仓库的校验和
GET /pypi-proxy/packages/requests-2.28.0.whl.sha256
预期: 返回正确的SHA256校验和
```

## 性能考虑

### 校验和计算
- 上传时计算并缓存，避免重复计算
- 对于大文件，计算时间可接受（通常<100ms）
- 数据库查询快速高效

### 存储优化
- 利用现有字段，无需额外存储
- 校验和字段大小固定（64字节）
- 不影响现有性能

## 安全考虑

- 校验文件只读，不涉及写操作
- SHA256算法安全性高
- 遵循现有的权限控制机制
- 防止校验和篡改

## 向后兼容性

- 新增功能，不影响现有功能
- 校验文件请求返回404不影响pip（会显示警告）
- 现有API保持不变

## 测试计划

### 单元测试
- SHA256计算测试
- 文件名解析测试
- 数据库查询测试

### 集成测试
- 上传文件后获取校验和
- 代理仓库的校验和获取
- 缓存机制测试

### 性能测试
- 大文件的校验和计算时间
- 数据库查询性能
- 并发请求测试

## 实现计划

1. **添加导入**
   - 添加 crypto/sha256 和 encoding/hex 导入

2. **修改上传方法**
   - 在 UploadPackage 中计算SHA256
   - 存储到 ChecksumSHA256 字段

3. **添加校验和处理**
   - 添加 handleChecksumRequest 方法
   - 支持缓存和实时计算

4. **修改下载方法**
   - 在 DownloadPackage 中添加校验文件检测

5. **测试验证**
   - 上传文件测试
   - 校验和获取测试
   - 代理仓库测试

## 未来扩展

- 支持其他校验和算法（MD5、SHA384、SHA512）
- 添加校验和验证API
- 实现元数据同步功能
- 添加搜索API功能
- 支持GPG签名文件

## 参考资料

- [PEP 503 - Simple Repository API](https://www.python.org/dev/peps/pep-0503/)
- [PyPI File Formats](https://packaging.python.org/specifications/)
- [pip Documentation - Secure Installs](https://pip.pypa.io/en/stable/topics/secure-installs/)
