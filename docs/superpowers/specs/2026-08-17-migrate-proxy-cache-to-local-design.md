# Proxy 缓存迁移到本地仓库设计

- **日期**: 2026-08-17
- **主题**: 为 proxy 仓库提供"迁移缓存到本地仓库"能力，解决"下线 proxy 但保留缓存内容"的场景
- **状态**: 设计待批准

## 背景与动机

生产环境存在一个 `maven-snapshots` **proxy** 仓库指向 Nexus，累计缓存了 2000+ 包（artifact 行数万）。场景诉求：

1. 想下线（删除）这个 proxy 仓库——依赖方不再需要从 Nexus 拉新构建，或上游将下线
2. 但已缓存的内容**不能丢**——历史构建、已解析版本要保留继续可用
3. 直接 `DELETE /repositories/{name}` 只删 `repositories` 行 + 清内存缓存（[repository_service.go:344](file:///Users/gracegaoya/work/project/moonlight-box/internal/service/repository_service.go#L344)），**不删** artifacts/packages/blob——结果是 2000+ 变孤儿数据：占空间、搜索可能仍显示、永远无法下载（已知问题：仓库删除不级联删 artifacts）

**目标**：提供显式的"迁移缓存到本地仓库"操作，把 proxy 的缓存内容整体搬进一个 local（hosted）仓库；之后用户再手动删除 proxy——数据保留、可下载、可被现有快照清理策略管理。

**非目标**：
- 不做"删除时二选一"的销毁流程改造（DELETE 保持简单、可预期）
- 不做跨 format 迁移（仅 maven → maven）
- 不做 proxy 回源数据的持续同步/镜像

## 架构前提：为什么迁移是零拷贝

底层存储是**全局共享的 CAS Blob Store**：

- blob 按 sha256 内容寻址存储在 `blobs/{alg}/{d[:2]}/{d[2:4]}/{d}`（[cas_blob_store.go:163](file:///Users/gracegaoya/work/project/moonlight-box/internal/storage/cas_blob_store.go#L163)），跨仓库去重共享
- artifact 通过 `artifact_blobs` 关联表引用 blob（[metadata_store.go:508](file:///Users/gracegaoya/work/project/moonlight-box/internal/storage/metadata_store.go#L508)）

因此"迁移"= **改三张表的 `repository_id`**，blob 文件原样不动：零拷贝、零网络、秒级完成。

唯一键均含 `repository_id`（[package.go:11,34](file:///Users/gracegaoya/work/project/moonlight-box/internal/model/package.go#L11)：`packages(rid,format,name)`、`package_versions(rid,format,package_name,version)`；artifact 的 `idx_artifact_identity` 同理）——目标仓库 ID 不同，跨仓库 UPDATE 天然不冲突（前提：目标仓库无重叠包）。

## 决策

**独立动作 + 手动删除（方案 A）**：

```
proxy 仓库详情页 → "迁移缓存到本地仓库" → 选目标 local 仓库
  → API 执行（事务 UPDATE repository_id）
  → 返回迁移数量 → 用户在 UI 验证 local 可下载 → 手动删除 proxy
```

不塞进 DELETE 流程。可逆、可验证、DELETE 保持简单。

## 接口设计

### REST API

```
POST /api/v1/repositories/{name}/migrate-cache
Authorization: Bearer <token>   （挂在 reposWrite 组，`repositories:write` 权限）

Request:
{
  "target_repository": "maven-snapshots-copy"   // 已存在的 local 仓库名
}

Response 200:
{
  "source_repository": "maven-snapshots",
  "target_repository": "maven-snapshots-copy",
  "moved_artifacts": 2083,
  "moved_packages": 657,
  "moved_versions": 26809,
  "target_had_conflicts": false
}

错误：
400  目标仓库不存在 / 不是 local / format 不一致 / source==target / source 不是 proxy
409  目标仓库存在重叠包（format+name 或 format+name+version 已存在）
404  source 仓库不存在
```

### 服务层

编排放 `RepositoryService`（已有 repoRepo + 缓存失效 + 运行时管理，仓库生命周期归它管），行迁移放 `ArtifactService`（持有 db 与投影逻辑）：

1. `RepositoryService.MigrateCacheToRepo(ctx, sourceName, targetName string)`：
   - 加载并校验 source（proxy、enabled）、target（local、enabled）、`PackageType` 一致、`source != target`
   - 调用 `ArtifactService.MigrateArtifactsToRepo(ctx, sourceRepoID, targetRepoID)` 执行行迁移
   - 成功后 `invalidateCache(source)`、`invalidateCache(target)`（复用现有缓存失效 + 运行时重载）

2. `ArtifactService.MigrateArtifactsToRepo(ctx, sourceRepoID, targetRepoID uint) (*MigrateResult, error)`：
   - **预检查冲突**：查询目标仓库中与源仓库重叠的 `(format, name)` 与 `(format, name, version)`。若有 → 409（列出前 N 个重叠项）
   - **单事务**：
     - `UPDATE artifacts SET repository_id = target WHERE repository_id = source` → moved_artifacts
     - `UPDATE packages SET repository_id = target WHERE repository_id = source` → moved_packages
     - `UPDATE package_versions SET repository_id = target WHERE repository_id = source` → moved_versions
   - 返回结果

> 投影行整体搬移后与 artifacts 保持一致，**无需 Rebuild**（数据未变，仅换仓库归属）。Maven SNAPSHOT 元数据由插件从 artifacts 表实时渲染（`CurrentSnapshotFileDisplays`），迁入 local 后照常解析、自动指向最新构建。

## 与快照清理的互动（重要）

目标 local 仓库是 maven 仓库，**天然受现有快照清理任务管理**（`maven_snapshot` 扫所有 `type=local + format=maven`，[snapshot_cleanup_service.go:95](file:///Users/gracegaoya/work/project/moonlight-box/internal/service/snapshot_cleanup_service.go#L95)）。

- 迁移后，local 仓库里老构建（文件名时间戳超出 `max_age_days`，默认 90 天）会在下一次清理执行时被删除——**这正是"2000+ 瘦身"的机制**，无需手动逐条删
- 想多保留 → 调 target 仓库级配置 `snapshot_keep_last` / `snapshot_max_age_days`（[repository.go:188-189](file:///Users/gracegaoya/work/project/moonlight-box/internal/model/repository.go#L188)）
- 想尽快清空历史 → 调小 `max_age_days`（如 7）或手动 `POST /download-logs/snapshot-cleanup/now`
- ⚠️ 迁移执行期间避免并发触发 `snapshot-cleanup/now`（事务保证单次一致性，但推荐串行执行）

## 前端设计

proxy 仓库详情页增加"迁移缓存到本地仓库"操作：

1. 按钮仅对 **proxy** 仓库显示（hosted/virtual 不显示）
2. 点击弹对话框：下拉选择目标仓库（过滤 `local` + 同 format，排除自身）
3. 确认 → `POST migrate-cache` → 成功展示迁移数量
4. 成功提示："已迁移 N 个 artifact 到 {target}。请验证 {target} 可正常下载后，再删除代理仓库 {source}。"
5. 409 冲突 → 展示冲突提示

## 边界情况与风险

| 情况 | 行为 |
|------|------|
| source 缓存为空 | 返回 moved=0，成功（幂等） |
| target 是空仓库 | 最典型用法，无冲突 |
| target 有重叠包 | 409 拒绝，提示先清理 target 或改用空仓库 |
| 迁移中途失败（DB 错误） | 事务回滚，保持原状 |
| 迁移后 source 仍被请求 | source 缓存已空 → 回源重拉（上游在则可用；已删则 404） |
| 目标仓库快照清理并发 | 推荐串行；文档注明 |
| 大仓库性能 | 纯 UPDATE，无文件 IO，数万行秒级 |

## 验证方案

1. **单元测试**（service 层 `MigrateCacheToRepo`）：
   - proxy→local 成功：三表 repository_id 全量更新、blob 引用不变
   - source 非 proxy / target 非 local / format 不一致 / source==target → 400
   - target 有重叠包 → 409
   - source 缓存为空 → moved=0
   - 事务回滚：注入 DB 错误后数据无变化
2. **集成验证**（本地起服务）：
   - 建 proxy maven 仓库（指向本地 mock/Nexus）→ 缓存若干包 → 建空 local maven 仓库 → migrate-cache → 验证 local 可下载、maven-metadata 渲染正常 → 删除 proxy → 确认无孤儿数据
3. **生产演练**：先在测试环境对 `maven-snapshots` 完整演练一次（含快照清理对迁移后数据的裁剪效果），确认后上线

## 影响范围

| 文件 | 改动 |
|------|------|
| `internal/service/artifact_service.go` | 新增 `MigrateArtifactsToRepo`（预检查 + 事务 UPDATE 三表） |
| `internal/service/repository_service.go` | 新增 `MigrateCacheToRepo`（校验编排 + 缓存失效） |
| `internal/api/http/repository_handler.go` | 新增 `MigrateCache` handler |
| `cmd/registry/router.go` | 在 `reposWrite` 组注册 `POST /repositories/:name/migrate-cache` |
| `cmd/registry/main.go` | DI 注入 `ArtifactService` 到 `RepositoryService`（如未注入） |
| `web/src/api/repository.ts` | 新增 API 方法 |
| `web/src/views/`（仓库详情页） | proxy 迁移按钮 + 目标选择对话框 |
| 测试 | service 单元测试 + 前端组件测试 |
