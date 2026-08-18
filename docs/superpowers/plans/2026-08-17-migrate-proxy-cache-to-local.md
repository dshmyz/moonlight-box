# Proxy 缓存迁移到本地仓库 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 proxy 仓库提供"迁移缓存到本地仓库"能力：把 proxy 缓存的内容（artifacts + packages + package_versions 三表行）整体搬进一个已存在的 local 仓库，之后用户手动删除 proxy——数据保留、可下载、由现有快照清理策略管理。零拷贝（CAS 共享 blob），仅改 `repository_id`。

**Architecture:** 编排放 `RepositoryService.MigrateCacheToRepo`（已有 repoRepo + 缓存失效 + repoMgr，负责仓库生命周期），行迁移放 `ArtifactService.MigrateArtifactsToRepo`（持有 db + 投影逻辑，负责预检查冲突 + 单事务 UPDATE 三表）。两者通过 main.go 注入的 setter 关联（与现有 `SetRepoCache`/`SetRepoManager`/`SetHealthCheckService` 模式一致）。API 挂 `reposWrite` 组（`repositories:write` 权限）。

**Tech Stack:** Go 1.24, GORM, Gin, Vue 3 + TS + Element Plus。

## Global Constraints

- 迁移只允许 **proxy → local**，`PackageType` 必须一致，`source != target`。
- **禁止复制/移动 blob**——CAS 内容寻址共享，改 `repository_id` 即可；任何涉及文件 IO 的实现都是错误方向。
- 冲突必须预检查并 **409 拒绝**，不能在 UPDATE 时靠唯一索引报错兜底（会留下半迁移状态）。
- 三表 UPDATE 必须在**同一个事务**内，失败整体回滚。
- 迁移后**不做**全量 Rebuild（数据未变，仅换归属）；只做缓存失效。
- 前端入口只对 **proxy** 仓库显示。
- 不修改 `DELETE /repositories/{name}` 行为（DELETE 保持简单）。

---

### Task 1: ArtifactService.MigrateArtifactsToRepo（事务行迁移）

**Files:**
- Modify: `internal/service/artifact_service.go`
- Test: `internal/service/artifact_service_test.go`（或新建 `migrate_test.go`）

**Interfaces:**

```go
type MigrateResult struct {
    MovedArtifacts int64
    MovedPackages  int64
    MovedVersions  int64
    Conflicts      []ConflictItem // 预检查发现的冲突（无则为 nil）
}

type ConflictItem struct {
    Kind  string // "package" | "version"
    Name  string
    Version string
}

// 预检查冲突 + 单事务 UPDATE 三表。无冲突才迁移。
func (s *ArtifactService) MigrateArtifactsToRepo(ctx context.Context, sourceRepoID, targetRepoID uint) (*MigrateResult, error)
```

**行为:**
1. 预检查冲突：查询 target 中与 source 重叠的 `(format, name)`（packages）和 `(format, package_name, version)`（package_versions）。有冲突 → 返回 `MigrateResult{Conflicts: ...}` + 不迁移（由上层转 409）。
2. 无冲突 → 单事务内：
   - `UPDATE artifacts SET repository_id = target WHERE repository_id = source` → MovedArtifacts
   - `UPDATE packages SET repository_id = target WHERE repository_id = source` → MovedPackages
   - `UPDATE package_versions SET repository_id = target WHERE repository_id = source` → MovedVersions
   - （`artifact_blobs` 按 artifact_id 关联，不涉及 repository_id，无需改动）
3. 返回结果。

**注意:** 不要触发 `recalcPackageVersions` / `syncPackageAfterSave`（行已存在，无增删）。迁移是纯归属变更。

---

### Task 2: RepositoryService.MigrateCacheToRepo（校验编排 + 缓存失效）

**Files:**
- Modify: `internal/service/repository_service.go`
- Modify: `internal/service/repository_service_test.go`（如存在）

**Interfaces:**

```go
// 注入（与 SetRepoCache 同模式）
func (s *RepositoryService) SetArtifactService(as *ArtifactService)

func (s *RepositoryService) MigrateCacheToRepo(ctx context.Context, sourceName, targetName string) (*MigrateResult, error)
```

**行为:**
1. 加载 source、target；校验：
   - source 存在且 `Type == proxy`，否则返回 400（明确错误信息）
   - target 存在且 `Type == local`，否则返回 400
   - `source.PackageType == target.PackageType`，否则返回 400
   - `source.ID != target.ID`，否则返回 400
2. 调 `s.artifactSvc.MigrateArtifactsToRepo(ctx, source.ID, target.ID)`。
3. 成功（或返回冲突结果）后 `s.invalidateCache(sourceName)`、`s.invalidateCache(targetName)`——`invalidateCache` 已存在，含 `repoMgr` 运行时删除。冲突时也应失效缓存（无实际变更，无害）。
4. 返回结果。

---

### Task 3: RepositoryHandler.MigrateCache + 路由注册

**Files:**
- Modify: `internal/api/http/repository_handler.go`
- Modify: `cmd/registry/router.go`
- Modify: `cmd/registry/main.go`

**Handler:**

```go
func (h *RepositoryHandler) MigrateCache(c *gin.Context) {
    // name := c.Param("name")
    // body: {"target_repository": "..."} binding required
    // result, err := h.svc.MigrateCacheToRepo(ctx, name, req.TargetRepository)
    // if len(result.Conflicts) > 0 → 409，附冲突列表（截断到前 20 条）
    // 否则 response.Success(c, gin.H{source, target, moved_artifacts, moved_packages, moved_versions})
}
```

**Router:** 在 `cmd/registry/router.go` 的 `reposWrite` 组（约 198-205 行）内新增：
```go
reposWrite.POST("/:name/migrate-cache", ctx.Handlers.Repository.MigrateCache)
```
（确认 `ctx.Handlers.Repository` 字段名——当前 HandlerSet 中仓库 handler 的字段名）

**DI:** main.go 在 `repoSvc` 与 `artifactSvc` 都已创建后调用 `repoSvc.SetArtifactService(artifactSvc)`（注意两者创建顺序；若 artifactSvc 在 repoSvc 之后创建，则在最后统一注入，或提前创建 artifactSvc）。

---

### Task 4: 后端单元测试

**Files:**
- Test: `internal/service/`（迁移逻辑）
- Test: `internal/api/http/`（handler 层，如仓库 handler 已有测试模式）

**用例（对应 spec 验证方案）：**
1. proxy→local 成功：三表 repository_id 全量更新、`artifact_blobs` 不变
2. source 非 proxy / target 非 local / format 不一致 / source==target → 对应 400 错误
3. target 有重叠 package/version → 返回 Conflicts，不迁移
4. source 缓存为空 → Moved*=0，无冲突
5. 事务回滚：在 UPDATE 之间注入错误 → 三表均无变化
6. handler：非法 body（缺 target_repository）→ 400；冲突 → 409 + 冲突列表

**命令：** 在仓库根目录运行 `go test ./internal/service/... ./internal/api/http/...`

---

### Task 5: 前端 API + 迁移对话框

**Files:**
- Modify: `web/src/api/repository.ts`
- Modify: `web/src/views/RepositoryDetail.vue`
- Test: `web/src/views/RepositoryDetail.spec.ts`（或仓库组件现有测试）

**API 方法（web/src/api/repository.ts）：**
```ts
export function migrateRepositoryCache(name: string, targetRepository: string) {
  return request.post(`/repositories/${name}/migrate-cache`, { target_repository: targetRepository })
}
```

**UI（RepositoryDetail.vue）：**
1. 仅当当前仓库 `type === 'proxy'` 时显示"迁移缓存到本地仓库"按钮（放在操作区，与禁用/删除相邻）。
2. 点击 → `el-dialog`：`el-select` 目标仓库（数据源：仓库列表过滤 `type==='local'` 且 `package_type===当前仓库` 且排除自身），确认按钮带 loading。
3. 成功 → `ElMessage.success`：`已迁移 {moved_artifacts} 个 artifact 到 {target}。请验证 {target} 可正常下载后，再删除代理仓库 {source}。`
4. 409 → 展示冲突提示（"目标仓库存在重叠包，请先清理或改用空仓库"，可展示冲突列表）。
5. 按钮按现有权限控制方式处理（仓库管理权限用户可见）。

---

### Task 6: 验证

- 后端：`make build`（或 `go build ./...`）、`go vet ./...`、`go test ./internal/service/... ./internal/api/http/...` 全绿。
- 前端：`cd web && npm run build`（TS 编译）+ 相关组件测试通过。
- 集成（本地 preview）：建 proxy maven 仓库 → 触发缓存若干包 → 建空 local maven 仓库 → 调 `migrate-cache` → 验证 local 可下载、maven-metadata 渲染正常 → 删除 proxy → 无孤儿数据。
- 权限：无 `repositories:write` 的用户调接口返回 403。
