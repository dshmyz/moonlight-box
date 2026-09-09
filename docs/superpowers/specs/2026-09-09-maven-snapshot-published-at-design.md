# Maven SNAPSHOT 版本发布时间 — 设计文档

日期：2026-09-09
状态：待评审

## 背景问题

包详情页版本列表的「发布时间」对 Maven SNAPSHOT 不真实：

1. 代理同步来源是 `maven-metadata.xml` 的 `<lastUpdated>`——它是 **metadata 文件的更新时间**，随上游重新部署变化，不是快照构建时间。
2. 本地（local）仓库上传的制品从不写入 `published_at`，界面显示 `-`。
3. 现有 `preservePublishedAt`（first-write-wins）解决了「时间反复跳变」，但没解决「时间本身不对」。

## 目标

对 `1.0-SNAPSHOT` 这类版本，发布时间显示**最新一次快照构建的真实时间**：

- 时间来源：快照文件名中的 timestamp（如 `lib-1.0-20260604.090000-2.jar`）或 `<snapshotVersion><updated>`。
- 文件不变 → 时间不变（下载、重复同步都不动它）。
- 出现新构建 → 时间前进到新构建时间。
- 无可解析时间 → 回退 metadata 时间；再无 → 上传/首次入库时间。
- local 与 proxy 走同一套规则，界面零改动。

## 非目标

- 不做启动时全量历史重算；旧记录在下一次该版本发生聚合（同步/新文件/AttachBlob）时自然修正。
- 不在版本列表展开每个历史构建的时间；只显示最新构建时间。
- 不新增数据库字段、不新增数据库查询。

## 设计

### 通用性约束（硬性）

**服务层与任何共享代码严禁出现 `format == "maven"` 类格式特判。** 协议语义只存在于插件内部。服务层只使用两个通用概念：

- `artifact.Attributes["published_at"]`（RFC3339 字符串）——任何插件都可以按自己的协议语义写入；
- `runtime.Kind`（通用枚举：artifact/version/metadata/checksum/directory）——`IsCountableFileKind` 已有的口径，表示「真实文件行」。

### 插件侧（internal/plugins/maven，协议解析内聚于此）

新增一个 helper，复用已有的 `parseSnapshotFileInfo`：

```go
// snapshotPublishedAt 从 SNAPSHOT 时间戳文件名解析构建时间（UTC，RFC3339）。
// 非时间戳文件名 / 非 SNAPSHOT 版本返回 ""。
func snapshotPublishedAt(artifact, version, filename string) string
```

三个制品构造点调用它，把结果写入 `Attributes["published_at"]`（仅解析成功时覆盖，解析失败保留已有值）：

| 构造点 | 覆盖场景 |
|---|---|
| `handleUpload` | local `mvn deploy` / 手动上传 |
| `NormalizeAsset` | proxy 回源下载单个快照文件 |
| `FetchRemote` 的 `<snapshotVersions>` 循环 | proxy 元数据同步；`sv.Updated`（14 位）经已有的 `parseMavenLastUpdated` 转换 |

版本级行（KindVersion）从 `<lastUpdated>` 写 `published_at` 的现有逻辑**保持不变**——它是回退来源，不是主来源。

### 服务侧（recalcPackageVersionSummary，通用逻辑）

现有循环里「取第一个非空 published_at」改为按行类别收集：

```text
文件行（IsCountableFileKind）→ filePublished = max(各行的 published_at)
版本行（其余）              → metaPublished = 首个 published_at（≈现状）
```

优先级：

```text
summary.PublishedAt = filePublished        ← 真实构建时间，可随新构建前进
                    || metaPublished       ← metadata 时间（proxy 无文件行时）
                    || summary.CreatedAt   ← 上传/首次入库时间（local 无时间戳文件名时）
```

`preservePublishedAt` 的 first-write-wins **保持不变**：它作用于「同一行」的重复写入。新构建是**新行**（identity_key 含时间戳文件名），新行自带新的 `published_at`，参与 max 即可让时间前进——无需任何特判。

### 行为推演

| 场景 | 结果 |
|---|---|
| 反复 GET 下载 / 重复同步同一批文件 | 行不变 → max 不变 → 时间不变 |
| 上游 metadata `lastUpdated` 变化后重新同步 | 版本行被 first-write-wins 保留；文件行未变 → 时间不变 |
| 上游出现新构建 `-20260605.090000-3.jar` | 新文件行 → max 前进 → 时间 = 新构建时间 |
| local deploy 时间戳文件名 | = 文件名构建时间（UTC） |
| local 上传 `lib-1.0-SNAPSHOT.jar`（无时间戳） | 解析失败 → 回退 `created_at`（= 上传时间） |
| proxy 无任何可解析时间 | 回退 metadata 时间 |
| release（非 SNAPSHOT）版本 | 只有版本行 → 行为与现状完全一致 |
| KindVersion（lastUpdated）晚于最新构建 | 文件行优先于版本行 → 不被 metadata 时间污染 |

## 性能

- `recalcPackageVersionSummary` 本就一次性加载该版本全部制品并遍历；新增的只是同一次遍历中的字符串解析与比较，**O(n) 内常数开销，0 新增 DB 查询**。
- 无启动全量扫描；历史数据随该版本的下一次聚合自然修正。

## 测试计划（全部自动化）

插件级（internal/plugins/maven）：
1. 时间戳文件名 → `published_at == "2026-06-04T09:00:00Z"`（UTC）。
2. `lib-1.0-SNAPSHOT.jar` / release 文件名 → 不写 `published_at`。
3. `snapshotVersions` 路径 → 文件行携带 `sv.Updated` 转换的时间。

服务级（recalcPackageVersionSummary）：
4. 两个构建（旧+新）→ 取新构建时间。
5. 版本行 metadata 时间晚于最新构建 → 仍取文件行时间。
6. 仅版本行 → 取 metadata 时间（release 行为不变）。
7. 均无 → 取 `created_at`。
8. 重复 recalc / 重复同步 → 时间稳定不变。
9. 追加新构建行 → 时间前进。

grep 检查：`internal/service` 与非 maven 插件目录中不出现本次新增的任何 maven 字样特判。
