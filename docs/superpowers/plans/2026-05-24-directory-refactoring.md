# 目录结构重构实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 将项目目录结构完全重构为符合 `new3.md` 文档要求的架构，包括创建 core 层、迁移 plugins 层、重组 API 层等。

**架构：** 采用分层架构，将核心抽象（runtime、graph、blob）放入 core/ 层，协议实现放入 plugins/ 层，HTTP API 放入 api/http/ 层，后台任务放入 worker/ 层。通过拆分 types/v2.go 实现职责分离。

**技术栈：** Go 1.21+, Git, sed (批量替换), gofmt

---

## 文件结构总览

### 新建文件（核心层）
- `internal/core/runtime/interface.go` - RepositoryRuntime 接口定义
- `internal/core/runtime/hosted.go` - HostedRuntime 实现
- `internal/core/runtime/proxy.go` - ProxyRuntime 实现
- `internal/core/runtime/group.go` - GroupRuntime 实现
- `internal/core/graph/artifact.go` - Artifact 数据结构
- `internal/core/graph/relation.go` - ArtifactRelation 关系
- `internal/core/blob/interface.go` - BlobStore 接口
- `internal/core/blob/cas.go` - CAS 实现（从 storage/ 迁移）
- `internal/core/projection/interface.go` - Projection 接口
- `internal/core/transaction/upload_session.go` - UploadSession 事务
- `internal/core/repository/router.go` - RepositoryRouter 路由
- `internal/core/repository/manager.go` - RepositoryManager 管理器

### 新建文件（插件层）
- `internal/plugins/maven/plugin.go` - Maven 插件（从 adapter/ 迁移）
- `internal/plugins/npm/plugin.go` - npm 插件
- `internal/plugins/pypi/plugin.go` - PyPI 插件
- `internal/plugins/apt/plugin.go` - APT 插件
- `internal/plugins/yum/plugin.go` - YUM 插件
- `internal/plugins/go/plugin.go` - Go 插件
- `internal/plugins/raw/plugin.go` - Raw 插件

### 迁移文件（API 层）
- `internal/api/http/*.go` - 所有 handler 文件（从 handler/ 迁移）

### 迁移文件（缓存层）
- `internal/core/cache/*.go` - 所有缓存文件（从 cache/ 迁移）

### 修改文件
- `cmd/registry/main.go` - 更新 import 路径
- `cmd/registry/router.go` - 更新 import 路径
- `go.mod` - 无需修改（模块路径不变）

### 删除文件
- `internal/types/v2.go` - 拆分后删除
- `internal/adapter/` - 迁移后删除整个目录
- `internal/handler/` - 迁移后删除整个目录
- `internal/cache/` - 迁移后删除整个目录

---

## 任务 1：创建备份和新目录结构

**文件：**
- 无文件修改，仅创建目录和 Git 分支

- [ ] **步骤 1：创建 Git 备份分支**

```bash
git checkout -b backup-before-refactor
git add -A
git commit -m "backup: 保存重构前状态"
git checkout main
git checkout -b refactor-directory-structure
```

- [ ] **步骤 2：创建 core 层目录结构**

```bash
mkdir -p internal/core/runtime
mkdir -p internal/core/graph
mkdir -p internal/core/blob
mkdir -p internal/core/projection
mkdir -p internal/core/transaction
mkdir -p internal/core/repository
mkdir -p internal/core/cache
mkdir -p internal/core/events
```

- [ ] **步骤 3：创建 plugins 层目录结构**

```bash
mkdir -p internal/plugins/maven
mkdir -p internal/plugins/npm
mkdir -p internal/plugins/pypi
mkdir -p internal/plugins/oci
mkdir -p internal/plugins/apt
mkdir -p internal/plugins/yum
mkdir -p internal/plugins/go
mkdir -p internal/plugins/raw
```

- [ ] **步骤 4：创建 api 和 worker 层目录结构**

```bash
mkdir -p internal/api/http
mkdir -p internal/worker/projection
mkdir -p internal/worker/gc
```

- [ ] **步骤 5：验证目录创建成功**

```bash
ls -la internal/core/
ls -la internal/plugins/
ls -la internal/api/
ls -la internal/worker/
```

预期：所有目录都已创建

- [ ] **步骤 6：Commit 目录结构**

```bash
git add internal/core internal/plugins internal/api internal/worker
git commit -m "refactor: 创建新的目录结构"
```

---

## 任务 2：拆分 types/v2.go - Runtime 接口和实现

**文件：**
- 创建：`internal/core/runtime/interface.go`
- 创建：`internal/core/runtime/hosted.go`
- 创建：`internal/core/runtime/proxy.go`
- 创建：`internal/core/runtime/group.go`
- 修改：`internal/types/v2.go`（后续删除）

- [ ] **步骤 1：创建 runtime/interface.go**

```go
package runtime

import (
	"context"
)

type ArtifactKey struct {
	RepositoryID string
	Format       string
	Coordinates  map[string]string
	Filename     string
	Extension    string
}

func (k *ArtifactKey) String() string {
	return k.RepositoryID + "/" + k.Format + "/" + k.Filename
}

type ArtifactQuery struct {
	RepositoryID string
	Format       string
	Coordinates  map[string]string
	Limit        int
	Offset       int
}

type ProjectionQuery struct {
	RepositoryID string
	Format       string
	Kind         string
	Coordinates  map[string]string
}

type UploadRequest struct {
	RepositoryID string
	Format       string
	Filename     string
	Size         int64
}

type RepositoryRuntime interface {
	GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
	QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
	BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
}
```

- [ ] **步骤 2：创建 runtime/hosted.go**

```go
package runtime

import (
	"context"
)

type HostedRuntime struct {
	MetadataStore MetadataStore
	BlobStore     BlobStore
	RepositoryID  string
}

func (n *HostedRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	return n.MetadataStore.Get(ctx, key)
}

func (n *HostedRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return n.MetadataStore.Query(ctx, query)
}

func (n *HostedRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	artifacts, err := n.MetadataStore.Query(ctx, ArtifactQuery{
		RepositoryID: query.RepositoryID,
		Format:       query.Format,
		Coordinates:  query.Coordinates,
	})
	if err != nil {
		return nil, err
	}
	return &ProjectionResult{
		Dynamic:  true,
		Artifact: artifacts[0],
	}, nil
}

func (n *HostedRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrNotImplemented
}
```

- [ ] **步骤 3：创建 runtime/proxy.go**

```go
package runtime

import (
	"context"
)

type ProxyRuntime struct {
	MetadataStore MetadataStore
	BlobStore     BlobStore
	RemoteClient  RemoteClient
	RepositoryID  string
	CachePolicy   CachePolicy
}

func (n *ProxyRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	artifact, err := n.MetadataStore.Get(ctx, key)
	if err == nil {
		return artifact, nil
	}

	metadata, err := n.RemoteClient.FetchMetadata(ctx, key)
	if err != nil {
		return nil, err
	}
	if !metadata.Exists {
		return nil, ErrNotFound
	}

	artifact = &Artifact{
		RepositoryID: n.RepositoryID,
		Format:       key.Format,
		Coordinates:  key.Coordinates,
	}

	if err := n.MetadataStore.Put(ctx, artifact); err != nil {
		return nil, err
	}

	return artifact, nil
}

func (n *ProxyRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	return n.MetadataStore.Query(ctx, query)
}

func (n *ProxyRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	artifacts, err := n.MetadataStore.Query(ctx, ArtifactQuery{
		RepositoryID: query.RepositoryID,
		Format:       query.Format,
		Coordinates:  query.Coordinates,
	})
	if err != nil {
		return nil, err
	}
	return &ProjectionResult{
		Dynamic:  true,
		Artifact: artifacts[0],
	}, nil
}

func (n *ProxyRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	return nil, ErrReadOnly
}
```

- [ ] **步骤 4：创建 runtime/group.go**

```go
package runtime

import (
	"context"
)

type RepositoryNode interface {
	GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error)
	QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error)
	RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error)
	BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error)
}

type GroupRuntime struct {
	Members  []RepositoryNode
	Writable RepositoryNode
}

func (g *GroupRuntime) GetArtifact(ctx context.Context, key ArtifactKey) (*Artifact, error) {
	for _, node := range g.Members {
		artifact, err := node.GetArtifact(ctx, key)
		if err == nil {
			return artifact, nil
		}
	}
	return nil, ErrNotFound
}

func (g *GroupRuntime) QueryArtifacts(ctx context.Context, query ArtifactQuery) ([]*Artifact, error) {
	allArtifacts := make([]*Artifact, 0)
	seen := make(map[string]bool)

	for _, node := range g.Members {
		artifacts, err := node.QueryArtifacts(ctx, query)
		if err != nil {
			continue
		}
		for _, a := range artifacts {
			if !seen[a.ID] {
				seen[a.ID] = true
				allArtifacts = append(allArtifacts, a)
			}
		}
	}
	return allArtifacts, nil
}

func (g *GroupRuntime) RenderProjection(ctx context.Context, query ProjectionQuery) (*ProjectionResult, error) {
	for _, node := range g.Members {
		result, err := node.RenderProjection(ctx, query)
		if err == nil {
			return result, nil
		}
	}
	return nil, ErrNotFound
}

func (g *GroupRuntime) BeginUpload(ctx context.Context, request UploadRequest) (UploadSession, error) {
	if g.Writable == nil {
		return nil, ErrReadOnly
	}
	return g.Writable.BeginUpload(ctx, request)
}
```

- [ ] **步骤 5：创建 runtime/errors.go**

```go
package runtime

import "errors"

var (
	ErrNotFound       = errors.New("not found")
	ErrNotImplemented = errors.New("not implemented")
	ErrReadOnly       = errors.New("read only")
)
```

- [ ] **步骤 6：Commit runtime 层**

```bash
git add internal/core/runtime/
git commit -m "refactor: 创建 runtime 核心层"
```

---

## 任务 3：拆分 types/v2.go - Graph 和 Blob

**文件：**
- 创建：`internal/core/graph/artifact.go`
- 创建：`internal/core/graph/relation.go`
- 创建：`internal/core/blob/interface.go`

- [ ] **步骤 1：创建 graph/artifact.go**

```go
package graph

import (
	"time"
)

type Artifact struct {
	ID           string
	RepositoryID string
	Format       string
	Kind         string
	Coordinates  map[string]string
	Properties   map[string]string
	Relations    []ArtifactRelation
	BlobRefs     []BlobRef
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type BlobRef struct {
	BlobID    uint
	Algorithm string
	Digest    string
	Size      int64
}

type BlobMetadata struct {
	Algorithm   string
	Digest      string
	Size        int64
	StoragePath string
	CreatedAt   time.Time
}
```

- [ ] **步骤 2：创建 graph/relation.go**

```go
package graph

type ArtifactRelation struct {
	Type     string
	TargetID string
}
```

- [ ] **步骤 3：创建 blob/interface.go**

```go
package blob

import (
	"context"
	"io"
)

type BlobStore interface {
	Put(reader io.Reader) (BlobRef, error)
	Open(ref BlobRef) (io.ReadCloser, error)
	Stat(ref BlobRef) (*BlobMetadata, error)
	Delete(ref BlobRef) error
}

type BlobRef struct {
	BlobID    uint
	Algorithm string
	Digest    string
	Size      int64
}

type BlobMetadata struct {
	Algorithm   string
	Digest      string
	Size        int64
	StoragePath string
	CreatedAt   time.Time
}
```

- [ ] **步骤 4：Commit graph 和 blob 层**

```bash
git add internal/core/graph/ internal/core/blob/
git commit -m "refactor: 创建 graph 和 blob 核心层"
```

---

## 任务 4：迁移 CAS BlobStore 实现

**文件：**
- 创建：`internal/core/blob/cas.go`
- 删除：`internal/storage/cas_blob_store.go`（后续）

- [ ] **步骤 1：读取原始文件**

```bash
cat internal/storage/cas_blob_store.go
```

- [ ] **步骤 2：创建 blob/cas.go（更新 package 声明）**

将 `cas_blob_store.go` 的内容复制到 `blob/cas.go`，修改：
- package 声明从 `storage` 改为 `blob`
- import 路径更新：
  - `github.com/dshmyz/moonlight-box/internal/model` 保持不变
  - `github.com/dshmyz/moonlight-box/internal/types` 改为 `github.com/dshmyz/moonlight-box/internal/core/blob`

- [ ] **步骤 3：Commit CAS 实现**

```bash
git add internal/core/blob/cas.go
git commit -m "refactor: 迁移 CAS BlobStore 到 core/blob"
```

---

## 任务 5：创建其他核心组件

**文件：**
- 创建：`internal/core/projection/interface.go`
- 创建：`internal/core/transaction/upload_session.go`
- 创建：`internal/core/repository/router.go`
- 创建：`internal/core/repository/manager.go`

- [ ] **步骤 1：创建 projection/interface.go**

```go
package projection

type ProjectionResult struct {
	Dynamic  bool
	Content  []byte
	Artifact *Artifact
}
```

- [ ] **步骤 2：创建 transaction/upload_session.go**

```go
package transaction

import (
	"context"
	"io"
)

type UploadSession interface {
	PutBlob(ctx context.Context, blob io.Reader) (BlobRef, error)
	PutArtifact(ctx context.Context, artifact *Artifact) error
	Commit(ctx context.Context) error
	Abort(ctx context.Context) error
}
```

- [ ] **步骤 3：创建 repository/router.go（从 types/v2.go 提取）**

提取 `types/v2.go` 中的 `RepositoryRouter` 相关代码，更新 package 为 `repository`

- [ ] **步骤 4：创建 repository/manager.go（从 types/v2.go 提取）**

提取 `types/v2.go` 中的 `RepositoryManager` 相关代码，更新 package 为 `repository`

- [ ] **步骤 5：Commit 其他核心组件**

```bash
git add internal/core/projection/ internal/core/transaction/ internal/core/repository/
git commit -m "refactor: 创建 projection、transaction、repository 核心层"
```

---

## 任务 6：迁移 cache 层

**文件：**
- 迁移：`internal/cache/` → `internal/core/cache/`

- [ ] **步骤 1：复制 cache 目录**

```bash
cp -r internal/cache/* internal/core/cache/
```

- [ ] **步骤 2：更新所有文件的 package 声明**

```bash
find internal/core/cache -name "*.go" -exec sed -i '' 's/^package cache$/package cache/' {} +
```

- [ ] **步骤 3：Commit cache 层**

```bash
git add internal/core/cache/
git commit -m "refactor: 迁移 cache 到 core/cache"
```

---

## 任务 7：迁移 adapter 到 plugins

**文件：**
- 迁移：`internal/adapter/maven_v2.go` → `internal/plugins/maven/plugin.go`
- 迁移：`internal/adapter/npm_v2.go` → `internal/plugins/npm/plugin.go`
- 迁移：`internal/adapter/pypi_v2.go` → `internal/plugins/pypi/plugin.go`
- 迁移：`internal/adapter/apt_v2.go` → `internal/plugins/apt/plugin.go`
- 迁移：`internal/adapter/yum_v2.go` → `internal/plugins/yum/plugin.go`
- 迁移：`internal/adapter/go_v2.go` → `internal/plugins/go/plugin.go`
- 迁移：`internal/adapter/generic_v2.go` → `internal/plugins/raw/plugin.go`

- [ ] **步骤 1：迁移 Maven 插件**

```bash
cp internal/adapter/maven_v2.go internal/plugins/maven/plugin.go
```

更新 `internal/plugins/maven/plugin.go`：
- package 声明改为 `maven`
- import 路径更新：`internal/types` → `internal/core/runtime`

- [ ] **步骤 2：迁移 npm 插件**

```bash
cp internal/adapter/npm_v2.go internal/plugins/npm/plugin.go
```

更新 package 和 import

- [ ] **步骤 3：迁移 PyPI 插件**

```bash
cp internal/adapter/pypi_v2.go internal/plugins/pypi/plugin.go
```

更新 package 和 import

- [ ] **步骤 4：迁移 APT 插件**

```bash
cp internal/adapter/apt_v2.go internal/plugins/apt/plugin.go
```

更新 package 和 import

- [ ] **步骤 5：迁移 YUM 插件**

```bash
cp internal/adapter/yum_v2.go internal/plugins/yum/plugin.go
```

更新 package 和 import

- [ ] **步骤 6：迁移 Go 插件**

```bash
cp internal/adapter/go_v2.go internal/plugins/go/plugin.go
```

更新 package 和 import

- [ ] **步骤 7：迁移 Raw 插件**

```bash
cp internal/adapter/generic_v2.go internal/plugins/raw/plugin.go
```

更新 package 和 import

- [ ] **步骤 8：Commit plugins 层**

```bash
git add internal/plugins/
git commit -m "refactor: 迁移 adapter 到 plugins"
```

---

## 任务 8：迁移 handler 到 api/http

**文件：**
- 迁移：`internal/handler/` → `internal/api/http/`

- [ ] **步骤 1：复制所有 handler 文件**

```bash
cp internal/handler/*.go internal/api/http/
```

- [ ] **步骤 2：更新所有文件的 package 声明**

```bash
find internal/api/http -name "*.go" -exec sed -i '' 's/^package handler$/package http/' {} +
```

- [ ] **步骤 3：Commit api/http 层**

```bash
git add internal/api/http/
git commit -m "refactor: 迁移 handler 到 api/http"
```

---

## 任务 9：批量更新 import 路径

**文件：**
- 修改：所有 `.go` 文件的 import 路径

- [ ] **步骤 1：更新 types 引用**

```bash
find . -name "*.go" -type f -exec sed -i '' 's|github.com/dshmyz/moonlight-box/internal/types|github.com/dshmyz/moonlight-box/internal/core/runtime|g' {} +
```

- [ ] **步骤 2：更新 adapter 引用**

```bash
find . -name "*.go" -type f -exec sed -i '' 's|github.com/dshmyz/moonlight-box/internal/adapter|github.com/dshmyz/moonlight-box/internal/plugins|g' {} +
```

- [ ] **步骤 3：更新 handler 引用**

```bash
find . -name "*.go" -type f -exec sed -i '' 's|github.com/dshmyz/moonlight-box/internal/handler|github.com/dshmyz/moonlight-box/internal/api/http|g' {} +
```

- [ ] **步骤 4：更新 cache 引用**

```bash
find . -name "*.go" -type f -exec sed -i '' 's|github.com/dshmyz/moonlight-box/internal/cache|github.com/dshmyz/moonlight-box/internal/core/cache|g' {} +
```

- [ ] **步骤 5：Commit import 更新**

```bash
git add -A
git commit -m "refactor: 批量更新 import 路径"
```

---

## 任务 10：修复编译错误

**文件：**
- 修改：所有有编译错误的文件

- [ ] **步骤 1：尝试编译**

```bash
go build ./...
```

记录所有编译错误

- [ ] **步骤 2：逐个修复编译错误**

根据编译错误提示，逐个修复：
- 缺失的 import
- 类型不匹配
- 未定义的符号

- [ ] **步骤 3：再次编译验证**

```bash
go build ./...
```

预期：无编译错误

- [ ] **步骤 4：Commit 编译修复**

```bash
git add -A
git commit -m "fix: 修复编译错误"
```

---

## 任务 11：修复测试

**文件：**
- 修改：所有测试文件

- [ ] **步骤 1：运行所有测试**

```bash
go test ./...
```

记录所有失败的测试

- [ ] **步骤 2：修复测试 import 路径**

测试文件的 import 路径也需要更新

- [ ] **步骤 3：修复测试逻辑**

如果有测试逻辑因重构而失败，修复测试代码

- [ ] **步骤 4：再次运行测试**

```bash
go test ./...
```

预期：所有测试通过

- [ ] **步骤 5：Commit 测试修复**

```bash
git add -A
git commit -m "fix: 修复测试"
```

---

## 任务 12：清理旧文件

**文件：**
- 删除：`internal/types/v2.go`
- 删除：`internal/adapter/`
- 删除：`internal/handler/`
- 删除：`internal/cache/`

- [ ] **步骤 1：删除 types/v2.go**

```bash
git rm internal/types/v2.go
```

- [ ] **步骤 2：删除 adapter 目录**

```bash
git rm -r internal/adapter/
```

- [ ] **步骤 3：删除 handler 目录**

```bash
git rm -r internal/handler/
```

- [ ] **步骤 4：删除 cache 目录**

```bash
git rm -r internal/cache/
```

- [ ] **步骤 5：Commit 清理**

```bash
git commit -m "refactor: 删除旧文件"
```

---

## 任务 13：最终验证

**文件：**
- 无文件修改，仅验证

- [ ] **步骤 1：编译验证**

```bash
go build ./...
```

预期：无错误

- [ ] **步骤 2：测试验证**

```bash
go test ./...
```

预期：所有测试通过

- [ ] **步骤 3：代码检查**

```bash
go vet ./...
```

预期：无警告

- [ ] **步骤 4：目录结构验证**

```bash
ls -la internal/core/
ls -la internal/plugins/
ls -la internal/api/
ls -la internal/worker/
```

预期：目录结构符合设计

- [ ] **步骤 5：功能验证**

启动服务，验证核心功能：
- Maven 仓库上传/下载
- npm 仓库上传/下载
- PyPI 仓库上传/下载
- 代理功能

- [ ] **步骤 6：创建最终 commit**

```bash
git add -A
git commit -m "refactor: 完成目录结构重构

- 创建 core/ 层，包含 runtime、graph、blob 等核心抽象
- 迁移 adapter/ 到 plugins/，按协议组织
- 迁移 handler/ 到 api/http/
- 迁移 cache/ 到 core/cache/
- 拆分 types/v2.go 到各核心模块
- 更新所有 import 路径
- 修复所有编译错误和测试

符合 new3.md 文档要求的架构设计"
```

---

## 任务 14：更新文档

**文件：**
- 修改：`README.md`
- 修改：`CLAUDE.md`

- [ ] **步骤 1：更新 README.md**

更新项目结构说明，反映新的目录组织

- [ ] **步骤 2：更新 CLAUDE.md**

更新开发指南，说明新的目录结构和职责划分

- [ ] **步骤 3：Commit 文档更新**

```bash
git add README.md CLAUDE.md
git commit -m "docs: 更新项目结构文档"
```

---

## 任务 15：合并到主分支

**文件：**
- 无文件修改，仅 Git 操作

- [ ] **步骤 1：切换到主分支**

```bash
git checkout main
```

- [ ] **步骤 2：合并重构分支**

```bash
git merge refactor-directory-structure
```

- [ ] **步骤 3：推送到远程**

```bash
git push origin main
```

- [ ] **步骤 4：删除重构分支**

```bash
git branch -d refactor-directory-structure
```

---

## 验证检查清单

### 编译验证
- [ ] `go build ./...` 无错误
- [ ] `go vet ./...` 无警告
- [ ] `golint ./...` 无严重问题

### 测试验证
- [ ] 所有单元测试通过
- [ ] 所有集成测试通过
- [ ] 覆盖率保持不变或提升

### 功能验证
- [ ] Maven 仓库功能正常
- [ ] npm 仓库功能正常
- [ ] PyPI 仓库功能正常
- [ ] 代理功能正常
- [ ] 上传功能正常

### 文档验证
- [ ] README 更新
- [ ] 架构图更新
- [ ] API 文档更新

### 目录结构验证
- [ ] core/ 层包含所有核心抽象
- [ ] plugins/ 层按协议组织
- [ ] api/http/ 包含所有 handler
- [ ] worker/ 层已创建
- [ ] 旧目录已删除

---

## 回滚策略

如果重构失败，执行以下回滚：

```bash
# 方式 1：切回备份分支
git checkout backup-before-refactor
git checkout -b main-restored
git branch -D main
git branch -m main-restored main

# 方式 2：重置到备份 commit
git reset --hard backup-before-refactor
```
