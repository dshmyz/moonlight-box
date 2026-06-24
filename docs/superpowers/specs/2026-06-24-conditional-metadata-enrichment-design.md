# 条件阻断元数据增强设计

## 目标

让代理仓库在下载制品前尽可能基于 `license`、`published_at` 等协议语义元数据执行条件阻断；当元数据不可获得时，按可用性优先策略放行并留下可检索的审计记录。

本设计的首批支持范围为 npm、PyPI、Maven。Go、YUM、APT、Generic 等协议保持现有下载行为；若存在条件规则但不能提供所需属性，记录未评估放行审计。

## 约束

- 严格遵守 `docs/new3.md` 的边界：Runtime 只决定何时评估和回源；Plugin 执行协议 HTTP 访问、解析并归一化元数据。
- 不改变 ProtocolPlugin 的 `Handle` 调用路径，不在 Handler 中增加上游 HTTP。
- 不改变无条件阻断、下载 API、前端页面和未支持协议的正常下载路径。
- 元数据失败、超时和协议不支持都不能使下载失败。
- 只有本地规则缓存显示存在潜在命中的条件规则时，才允许额外的元数据回源。

## 方案选择

采用可选的通用制品元数据能力，而不是复用 `FetchRemote`：制品文件路径在不同协议中的语义不同，复用现有 metadata projection 回源容易错配。

不使用“直接放行且不取属性”的方案，因为它会让条件阻断在首次直链下载中几乎失效。也不要求所有插件同步实现，避免接口变更破坏现有协议。

## 架构

```text
Plugin.Handle
  -> Runtime.GetArtifact(key)
  -> BlockRuleService.RequiredAttributes(format, name, version)
  -> 无候选条件规则：原有 GetArtifact 流程
  -> 有候选条件规则：
       1. 复用 Artifact.Attributes
       2. 缺失时，由 Runtime 回调 Plugin 的 ArtifactMetadataFetcher
       3. 属性完整：IsBlockedWithAttrs
       4. 属性不可得：Audit condition_unverified，放行
       5. 命中规则：返回 ErrBlocked，不下载 Blob
```

### 可选插件接口

在 `internal/core/runtime/interface.go` 新增：

```go
type ArtifactMetadata struct {
	Attributes map[string]string
}

type ArtifactMetadataFetcher interface {
	FetchArtifactMetadata(
		ctx context.Context,
		remoteBaseURL string,
		key ArtifactKey,
	) (*ArtifactMetadata, error)
}
```

该接口是可选能力，`RemoteFetcher` 保持不变。Runtime 仅在 Fetcher 同时实现此接口时调用它。插件不感知条件规则，仅返回标准化 Artifact 属性。

错误必须可区分：`ErrMetadataUnsupported`、`ErrMetadataUnavailable` 和其他错误。三者都放行，但审计原因分别为 `unsupported`、`unavailable`、`fetch_failed`。

### 规则预判

`BlockRuleService` 增加基于现有规则缓存的查询：

```go
type ConditionalRuleRequirement struct {
	RuleID    uint
	Attribute string
}

func (s *BlockRuleService) RequiredAttributes(
	pkgType, pkgName, version string,
) []ConditionalRuleRequirement
```

它仅按包类型、包名和版本筛选潜在命中的条件规则，包含 `PackageType=all` 规则。没有候选规则时不触发属性回源；条件评估仍由现有 `IsBlockedWithArtifact` 完成。

启动装配层的 `blockRuleBlocker` 将 service 的 `ConditionalRuleRequirement` 转换为 runtime 自己定义的 `ConditionRequirement`；Runtime 不 import service，Service 也不 import Runtime。Runtime 通过可选 `ConditionalBlocker` 类型断言使用此能力，不扩展已有 `PackageBlocker`，以维持所有既有 mock 与实现的兼容性。

### 风险控制

- 仅存在候选条件规则时发起属性请求。
- 已保存的 `Artifact.Attributes` 优先复用；获取成功后写回 MetadataStore。
- 用 ProxyRuntime 已有的 `singleflight.Group`，以 `attrs:<repo>:<format>:<name>:<version>` 聚合并发属性请求。
- 属性请求使用独立的、默认 2 秒的 context deadline；超时审计放行，不阻塞文件下载的完整上游超时。
- 增加短 TTL（默认 5 分钟）的“属性不可得”缓存，键与 singleflight 键一致。命中缓存时不重复请求上游，直接审计放行。
- 条件命中检查发生在 `ensureArtifactBlob` 和 `openArtifactContent` 之前，避免先下载 Blob 再拒绝。

### 审计

新增运行时可选 `ConditionAuditLogger`。其在启动装配层适配到 `AuditService`，记录动作 `condition_unverified`。

审计详情为 JSON，至少包含：repository、format、name、version、remote_path、候选 rule_ids、missing_attributes、reason。响应仍为成功下载；日志不包含上游响应体或用户敏感数据。

首版不做审计去重。只有条件规则候选匹配且属性无法评估时才写入，因此不会影响普通下载。上线后以审计量与属性请求指标决定是否增加聚合。

## 协议接入

### npm

复用 package metadata 中已解析的版本属性。根据 ArtifactKey 中的包名与版本请求 npm package metadata，返回对应版本的 `license` 与 `published_at`。

### PyPI

复用 JSON API 的 package/version 信息，返回对应 release 的 license 与最早上传时间。无法从文件名可靠映射包名或版本时返回 `ErrMetadataUnavailable`。

### Maven

读取并解析对应版本的 POM，返回 license；若可获得发布时间则一并返回。POM 缺失或无法解析时返回 `ErrMetadataUnavailable`。

## 测试

1. BlockRuleService：精确、通配、`all` 条件规则正确返回所需属性；无匹配规则返回空。
2. ProxyRuntime：无候选规则不调用属性 Fetcher；属性命中阻断且不拉 Blob；属性不支持、缺失、超时、请求失败时放行并记录审计。
3. ProxyRuntime：并发相同制品只执行一次属性请求；不可得缓存 TTL 内不重复请求；已保存 Attributes 不请求上游。
4. npm、PyPI、Maven：各自返回规范化属性，并覆盖上游不可得情形。
5. 回归：无条件阻断、无条件下载、未实现元数据能力的协议保持既有行为。

## 非目标

- 不保证所有协议都能执行条件阻断。
- 不修改 Hosted/Group 的既有阻断语义。
- 不增加前端配置项或审计去重 UI。
- 不以元数据不可得为理由拒绝下载。
