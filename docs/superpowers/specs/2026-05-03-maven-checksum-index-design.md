# Maven校验文件和仓库索引功能设计

## 概述

为Maven适配器添加校验文件（.sha1/.md5）和仓库索引功能，提升Maven仓库的完整性和可用性。

## 背景

当前Maven适配器已经实现了核心功能：
- 构件上传、下载、删除
- maven-metadata.xml支持
- SNAPSHOT版本管理
- 代理和缓存机制

但缺少以下重要功能：
1. **校验文件**：Maven客户端在下载文件时会验证校验和，缺失会导致警告
2. **仓库索引**：无法快速查看仓库中的所有包，影响用户体验

## 目标

### 第一阶段：校验文件支持
- 动态生成 .sha1 和 .md5 校验文件
- 支持本地仓库和代理仓库
- 符合Maven标准格式

### 第二阶段：仓库索引
- 提供基础索引功能
- 支持JSON和XML格式
- 列出仓库中所有包和版本

## 非目标

- GPG签名文件支持（暂不实现）
- Lucene索引（暂不实现）
- 复杂的搜索功能（暂不实现）

## 设计细节

### 第一阶段：校验文件支持

#### 架构设计

**核心思路**：在 `handleDownloadArtifact` 方法中检测校验文件请求，动态计算并返回校验和。

**处理流程**：
```
请求文件
   ↓
检查文件名
   ↓
┌──┴──┐
│     │
.sha1  普通
.md5   文件
│     │
↓     ↓
计算   正常
校验和 下载
   ↓
返回
校验和
```

#### 实现细节

**1. 修改 `handleDownloadArtifact` 方法**

在方法开头添加校验文件检测：

```go
func (a *MavenAdapter) handleDownloadArtifact(c *gin.Context, fullPath string) {
    // 检查是否是校验文件请求
    if strings.HasSuffix(fullPath, ".sha1") || strings.HasSuffix(fullPath, ".md5") {
        a.handleChecksumRequest(c, fullPath)
        return
    }
    
    // 原有的下载逻辑...
}
```

**2. 添加 `handleChecksumRequest` 方法**

```go
func (a *MavenAdapter) handleChecksumRequest(c *gin.Context, fullPath string) {
    // 解析路径
    parts := strings.Split(fullPath, "/")
    if len(parts) < 4 {
        response.NotFound(c, "checksum not found")
        return
    }

    filename := parts[len(parts)-1]
    
    // 确定校验类型和实际文件名
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

    // 构建实际文件路径
    groupArtifact := strings.Join(parts[:len(parts)-2], "/")
    version := parts[len(parts)-2]
    storageVersion := version + "/" + actualFilename
    
    // 获取文件内容
    content, _, err := a.storageSvc.GetPackage(c.Request.Context(), "maven", groupArtifact, storageVersion)
    
    if err != nil {
        // 尝试从代理获取
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
                    // 计算并返回校验和
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

    // 计算校验和
    body, _ := io.ReadAll(content)
    checksum := calculateChecksum(body, checksumType)

    // 返回校验和（Maven标准格式）
    c.String(200, "%s  %s", checksum, actualFilename)
}
```

**3. 添加 `calculateChecksum` 辅助方法**

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

**4. 需要添加的导入**

```go
import (
    "crypto/md5"
    "crypto/sha1"
    "encoding/hex"
    // ... 其他导入
)
```

#### 错误处理

- 文件不存在：返回 404
- 计算失败：返回 500
- 代理失败：返回 404

#### 测试场景

```bash
# 1. 上传文件
PUT /repo/maven-local/com/example/lib/1.0/lib-1.0.jar

Content: <jar-file-content>

# 2. 请求SHA1校验和
GET /repo/maven-local/com/example/lib/1.0/lib-1.0.jar.sha1

返回: <sha1-hash>  lib-1.0.jar

# 3. 请求MD5校验和
GET /repo/maven-local/com/example/lib/1.0/lib-1.0.jar.md5

返回: <md5-hash>  lib-1.0.jar

# 4. 代理仓库的校验和
GET /repo/maven-proxy-aliyun/commons-lang/commons-lang/2.6/commons-lang-2.6.jar.sha1

返回: <sha1-hash>  commons-lang-2.6.jar
```

### 第二阶段：仓库索引

#### 架构设计

**核心思路**：提供一个简单的索引端点，返回仓库中所有包的列表。

**处理流程**：
```
请求索引
   ↓
查询数据库
   ↓
生成索引数据
   ↓
返回JSON/XML
```

#### 实现细节

**1. 添加索引端点**

在 `RegisterRoutes` 方法中添加索引路由：

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

**2. 添加 `handleIndexRequest` 方法**

```go
func (a *MavenAdapter) handleIndexRequest(c *gin.Context) {
    repoName := c.Param("repoName")
    
    // 获取仓库信息
    var repo *model.Repository
    if r, ok := c.Get("repo"); ok {
        repo = r.(*model.Repository)
    }
    
    if repo == nil {
        response.NotFound(c, "repository not found")
        return
    }
    
    // 查询该仓库的所有包
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
    
    // 生成索引数据
    index := a.generateIndex(packages, repo.Name)
    
    // 根据Accept头返回不同格式
    accept := c.GetHeader("Accept")
    if strings.Contains(accept, "application/xml") {
        c.XML(200, index)
    } else {
        c.JSON(200, index)
    }
}
```

**3. 定义索引数据结构**

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

**4. 添加 `generateIndex` 方法**

```go
func (a *MavenAdapter) generateIndex(packages []model.Package, repoName string) *MavenPackageIndex {
    index := &MavenPackageIndex{
        Repository:  repoName,
        GeneratedAt: time.Now().Format(time.RFC3339),
        Packages:    make([]MavenPackageSummary, 0),
    }
    
    for _, pkg := range packages {
        // 解析 groupId 和 artifactId
        parts := strings.Split(pkg.Name, "/")
        if len(parts) < 2 {
            continue
        }
        
        groupID := parts[0]
        artifactID := parts[1]
        
        // 收集版本列表
        versions := make([]string, 0, len(pkg.Versions))
        var latest, release string
        
        for _, ver := range pkg.Versions {
            versions = append(versions, ver.Version)
            
            // 找出最新版本
            if latest == "" || compareVersions(ver.Version, latest) > 0 {
                latest = ver.Version
            }
            
            // 找出最新的release版本
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

#### API端点

```bash
# 获取仓库索引（JSON格式）
GET /repo/maven-local/index
Accept: application/json

# 获取仓库索引（XML格式）
GET /repo/maven-local/index
Accept: application/xml
```

#### 返回示例

**JSON格式**：
```json
{
  "repository": "maven-local",
  "generatedAt": "2026-05-03T12:00:00Z",
  "packages": [
    {
      "groupId": "com.example",
      "artifactId": "lib",
      "versions": ["1.0.0", "2.0.0"],
      "latest": "2.0.0",
      "release": "2.0.0"
    }
  ]
}
```

**XML格式**：
```xml
<?xml version="1.0"?>
<index>
  <repository>maven-local</repository>
  <generatedAt>2026-05-03T12:00:00Z</generatedAt>
  <packages>
    <package>
      <groupId>com.example</groupId>
      <artifactId>lib</artifactId>
      <versions>
        <version>1.0.0</version>
        <version>2.0.0</version>
      </versions>
      <latest>2.0.0</latest>
      <release>2.0.0</release>
    </package>
  </packages>
</index>
```

## 性能考虑

### 校验文件
- 动态计算，不占用存储空间
- 对于大文件，计算时间可接受（通常<100ms）
- 可以考虑添加缓存机制（未来优化）

### 仓库索引
- 对于大型仓库，可以考虑添加分页
- 可以添加缓存机制
- 可以添加过滤参数（如 groupId、artifactId）

## 安全考虑

- 校验文件只读，不涉及写操作
- 索引端点不暴露敏感信息
- 遵循现有的权限控制机制

## 向后兼容性

- 新增功能，不影响现有功能
- 校验文件请求返回404不影响Maven客户端（会显示警告）
- 索引端点为新增端点，不影响现有API

## 测试计划

### 单元测试
- `calculateChecksum` 方法测试
- `generateIndex` 方法测试
- 路径解析测试

### 集成测试
- 上传文件后获取校验和
- 代理仓库的校验和获取
- 索引端点的JSON/XML格式测试

### 性能测试
- 大文件的校验和计算时间
- 大型仓库的索引生成时间

## 实现计划

1. 添加校验文件支持
   - 修改 `handleDownloadArtifact` 方法
   - 添加 `handleChecksumRequest` 方法
   - 添加 `calculateChecksum` 方法
   - 添加必要的导入

2. 添加仓库索引支持
   - 定义索引数据结构
   - 添加 `handleIndexRequest` 方法
   - 添加 `generateIndex` 方法
   - 注册索引路由

3. 测试和验证
   - 编写单元测试
   - 进行集成测试
   - 验证Maven客户端兼容性

## 未来扩展

- 添加校验和缓存机制
- 支持GPG签名文件
- 实现Lucene索引
- 添加索引搜索功能
- 添加分页和过滤功能
