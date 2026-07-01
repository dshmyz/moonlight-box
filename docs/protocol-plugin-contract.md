# Protocol Plugin Contract

本文档定义新增或重构协议插件时必须遵守的工程契约。它是 `docs/new3.md`
的落地版，重点回答三个问题：

1. Artifact 里什么是事实，什么不是事实。
2. Protocol Projection 和 Admin Projection 分别放在哪里。
3. 新协议如何在不牺牲性能的前提下接入 hosted/proxy/group。

## 1. Core Principles

Moonlight Box 的 source of truth 是：

```text
Artifact Graph + CAS Blob
```

不是：

```text
HTTP 文件树
协议 metadata 文件
管理后台页面状态
```

因此所有协议实现必须遵守：

```text
Artifact 保存事实。
Runtime 管仓库行为。
Plugin 管协议语义。
Projection 从事实生成视图。
Admin API 聚合和展示视图，但不制造新的持久化事实。
```

## 2. Layer Ownership

| Layer | Owns | Must Not Own |
| --- | --- | --- |
| ProtocolPlugin | 路径语法、上传下载语义、协议 metadata parse/render、协议投影、RemoteFetcher | hosted/proxy/group 策略、缓存策略、stale 策略、Runtime 类型判断 |
| RepositoryRuntime | hosted/proxy/group、cache、stale、remote fetch orchestration、merge | XML/JSON/HTML 等协议格式、Maven SNAPSHOT、npm dist-tags 等协议语义 |
| Artifact Store | Artifact/Blob/Relation 持久化事实 | UI 展示结论、临时计算结果 |
| Protocol Projection | 包管理客户端看到的协议视图 | 管理后台表格状态 |
| Admin Projection | 管理后台统一视图和响应装饰 | Artifact source of truth |

## 3. Artifact Field Contract

Artifact 字段必须按下面规则使用。

| Field | Meaning | Examples |
| --- | --- | --- |
| `Format` | 协议格式 | `maven`, `npm`, `pypi`, `go` |
| `Kind` | Artifact 在协议内的逻辑角色 | `artifact`, `version`, `metadata`, `manifest`, `index` |
| `Name` | 包级身份，必须稳定可查询 | Maven `group:artifact`, npm package name, PyPI normalized package |
| `Version` | 逻辑版本 | `1.0.0`, `1.0-SNAPSHOT`, npm version |
| `Path` / `RemotePath` | 仓库路径事实。`RemotePath` 是 proxy/group 回源和下载路由的关键字段 | `com/acme/lib/1.0/lib-1.0.jar` |
| `Filename` | 文件名事实 | `lib-1.0.jar`, `foo-1.0.tar.gz` |
| `Qualifiers` | 协议身份字段，用于定位和区分 artifact | Maven `group`, `artifact`, `classifier`; PyPI `package`; OCI `platform` |
| `Properties` | 协议事实元数据，可用于重建协议投影 | `extension`, `filename`, `license`, `python_requires` |
| `Attributes` | 系统业务状态，必须是真实状态，不是页面显示结论 | `deprecated`, `yanked`, `visibility`, `retention` |
| `Metadata` | 运行时、来源、审计上下文 | `trigger_ip`, `remote_url`, `fetched_at`, `migrated_from` |

### 3.1 Red Lines

以下内容不得写入 Artifact：

```text
UI 展示字段
临时排序字段
默认展开/默认可见字段
可以从协议事实稳定推导的字段
一次 API 响应里的计算结果
```

示例：

```text
default_visible      不落库，只能在 Admin Projection/response decoration 中计算
display_group        不落库，只能在 Admin Projection/response decoration 中计算
files_downloaded     不落库，由 blob refs / storage 状态计算
```

## 4. Projection Types

Moonlight Box 使用两类 projection。

### 4.1 Protocol Projection

Protocol Projection 是包管理客户端看到的协议视图。

例子：

```text
Maven: maven-metadata.xml, snapshotVersions
npm: package metadata JSON, dist-tags
PyPI: /simple/{name}/ HTML 或 JSON
Go: @v/list, @v/{version}.info, @v/{version}.mod
YUM/APT: repodata, Packages, Release
```

位置：

```text
internal/plugins/<format>/
```

规则：

```text
Protocol Projection 必须靠近协议插件。
Runtime 不得理解协议 projection 的内部格式。
Protocol Projection 可以动态生成，也可以物化缓存，但缓存必须可从 Artifact facts 重建。
```

### 4.2 Admin Projection

Admin Projection 是管理后台看到的统一视图。

例子：

```text
包列表
版本列表
文件默认展示
缓存状态
迁移来源展示
```

规则：

```text
Admin Projection 可以调用协议插件提供的纯函数 helper。
Admin Projection 不得把展示结论回写 Artifact。
Admin Projection 默认必须纯内存计算，不得额外查 DB。
```

当前允许的务实做法：

```text
如果只有一个协议需要特殊展示，可以在 Admin API 中有一个小的 format 分支，
但协议规则必须放在对应 plugin package 中。
```

未来当两个以上协议需要特殊 Admin Projection 时，再引入 registry：

```go
type VersionFilesProjector func(version, name string, qualifiers model.JSONB, files []gin.H)
```

不要为了单个协议提前抽一套大型 view model。

## 5. Runtime Contract

ProtocolPlugin 的 `Handle` 必须遵守唯一合法路径：

```text
Plugin.Handle()
  -> parse protocol path
  -> identify protocol meaning
  -> runtime.QueryArtifacts(query)    // 需要回源时必须带 RemotePath
  -> runtime.GetArtifact(key)
  -> runtime.BeginUpload(...)
  -> render protocol response
```

禁止：

```text
Handle 中直接 http.Get/http.Post
Handle 中判断 ctx.Repository.Type == "proxy"/"group"/"hosted"
Handle 中对 *ProxyRuntime/*GroupRuntime/*HostedRuntime 做类型断言
Runtime 中解析协议 metadata 格式
```

唯一允许的远程 HTTP 调用位置：

```text
RemoteFetcher.FetchRemote(ctx, remoteURL, path)
```

## 6. Performance Contract

新增协议必须先满足性能契约，再追求抽象优雅。

### 6.1 Query Budget

| Path | DB Query Rule |
| --- | --- |
| Download path | 不得出现 N+1 查询；命中 artifact 后应直接流式返回 blob |
| Metadata/protocol index path | 允许 1 次 artifact query + 必要 blob/checksum 查询；不得按文件逐条查 DB |
| Admin list/detail path | 可以使用批量查询和内存聚合；projection 必须基于已加载数据 |
| Upload path | 不得为 UI 展示状态额外批量更新旧 artifact |
| Remote fetch path | FetchRemote 可以请求上游，但 Runtime 负责触发时机和缓存策略 |

### 6.2 Projection Budget

默认规则：

```text
Projection 只能遍历当前请求已经加载的 artifacts/files。
Projection 不允许自己访问 DB。
Projection 不允许请求远端。
Projection 时间复杂度应为 O(n) 或 O(n log n)，n 为当前版本/包的 artifact 数量。
```

允许的例外：

```text
协议索引非常大，动态生成会超过性能预算时，可以引入 Materialized Projection。
```

Materialized Projection 的要求：

```text
必须标记为缓存/派生数据，不是 source of truth。
必须有明确失效规则。
必须能从 Artifact Graph 重建。
必须避免写入路径同步做大规模重算；优先异步或增量更新。
```

### 6.3 Memory Budget

Admin API 和 Protocol Projection 不应一次性加载整个仓库。

要求：

```text
按 package/version/path 范围查询。
大索引必须分页、流式或物化。
JSON/XML/HTML 响应生成不得把超大 blob 读入内存。
下载必须走 streaming。
```

### 6.4 Cache Rules

缓存只能缓存派生结果或远端响应，不改变事实层。

```text
Cache key 必须包含 repository、format、path/package/version 等关键维度。
Cache TTL 必须可配置或来源清楚。
Cache miss 后必须能通过 Runtime/Artifact facts 重建。
Cache invalidation 必须和 upload/delete/migration 事件关联。
```

## 7. New Protocol Layout

新增协议建议使用以下目录结构：

```text
internal/plugins/<format>/
  plugin.go          // Handle 编排，只做协议路由和 Runtime 调用
  path.go            // path grammar parse/build
  metadata.go        // protocol metadata parse/render
  projection.go      // protocol projection
  remote.go          // FetchRemote
  model.go           // 协议内部小结构，不外泄到 Runtime
  *_test.go
```

简单协议可以合并文件，但职责边界不变。

## 8. New Protocol Checklist

新增协议前必须回答：

```text
1. Format 名称是什么？是否和前端/DB/路由一致？
2. Name 如何规范化？
3. Version 如何定义？
4. Artifact.Kind 有哪些？
5. RemotePath 是否对所有可下载 artifact 必填？
6. Qualifiers 放哪些协议身份字段？
7. Properties 放哪些协议事实字段？
8. Attributes 是否只包含系统业务状态？
9. Metadata 是否只包含运行时/来源/审计上下文？
10. Protocol Projection 如何从 Artifact facts 重建？
11. FetchRemote 返回哪些 Artifact？是否带 RemotePath？
12. Handle 是否完全通过 Runtime，不直接回源？
13. Admin Projection 是否纯内存？
14. 是否有防 N+1 查询测试或批量查询设计？
15. 是否有 hosted/proxy/group 的最小测试？
```

## 9. Maven SNAPSHOT Example

错误设计：

```text
上传 SNAPSHOT 文件时写 attributes.default_visible = true
上传后查询旧 artifact 并批量删除 default_visible
回源 metadata 时持久化 display_group
```

问题：

```text
迁移数据漏标会影响 UI
上传路径多一次 QueryArtifacts + BeginUpload + PutArtifact + Commit
清理失败会导致多个文件同时 visible
UI 展示结论污染 Artifact facts
```

正确设计：

```text
Maven artifact 保存文件名、版本、classifier、extension 等事实。
Maven snapshot projection 根据文件名解析 timestamp/buildNumber。
Admin API 返回版本文件时，基于已加载 files 动态计算 default_visible/display_group。
default_visible/display_group 只存在于响应中，不写 DB。
```

性能特征：

```text
上传路径少一次旧 artifact 批量更新。
Admin API 不增加 DB 查询。
计算复杂度 O(files in version)。
迁移数据天然兼容。
```

## 10. Design Review Questions

每次新增协议或重构协议时，评审必须问：

```text
1. 这里保存的是事实，还是投影？
2. 如果删除所有 projection/cache，能否从 Artifact Graph 重建？
3. Runtime 是否知道了协议格式？
4. Plugin 是否绕过了 Runtime？
5. Admin API 是否把展示结论写回 DB？
6. 是否引入了 N+1 查询？
7. 大包/大版本/大索引下是否会一次性加载过多数据？
8. 回源、上传、删除后缓存如何失效？
```

如果无法回答，先补设计再写代码。
