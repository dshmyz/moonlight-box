# 条件阻断元数据增强 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 npm、PyPI、Maven 的代理直链下载补充按需元数据评估，使条件阻断在可获得属性时生效、不可获得时审计放行。

**Architecture:** `ProxyRuntime` 只在本地规则缓存显示存在候选条件规则时，调用插件提供的可选通用元数据接口。属性优先复用已保存的 Artifact；缺失时通过单飞、短超时与不可得 TTL 缓存回源。插件只进行协议 HTTP 与解析，Runtime 决定阻断或审计放行。

**Tech Stack:** Go 1.24、GORM、Gin、`golang.org/x/sync/singleflight`、现有 Runtime/Plugin 架构。

## Global Constraints

- Plugin `Handle` 中不得直接新增上游 HTTP；Runtime 通过可选插件接口回调。
- 不修改现有 `RemoteFetcher` 或 `PackageBlocker` 的方法集，使用可选扩展接口保持兼容。
- 条件属性不可得、插件不支持、超时及元数据请求失败都必须放行并记录审计。
- 无候选条件规则的请求不得新增上游请求、等待或审计写入。
- 条件命中必须发生在 `ensureArtifactBlob` 和 `openArtifactContent` 前。
- 首批仅实现 npm、PyPI、Maven；其他插件保持现有行为。

---

### Task 1: 定义可选能力与审计动作

**Files:**
- Modify: `internal/core/runtime/interface.go`
- Modify: `internal/model/audit.go`
- Test: `internal/core/runtime/interface_test.go`

**Consumes:** 现有 `RemoteFetcher`、`PackageBlocker` 与 `AuditLogger`。
**Produces:** 不破坏旧实现的 `ArtifactMetadataFetcher`、`ConditionalBlocker`、`ConditionAuditLogger`；新增 `ActionConditionUnverified`。

- [ ] **Step 1: Write the failing test**

```go
func TestOptionalConditionalInterfaces(t *testing.T) {
	var _ ArtifactMetadataFetcher = &metadataFetcherMock{}
	var _ ConditionalBlocker = &conditionalBlockerMock{}
	var _ ConditionAuditLogger = &conditionAuditMock{}
	if model.ActionConditionUnverified != "condition_unverified" {
		t.Fatalf("unexpected audit action: %q", model.ActionConditionUnverified)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/runtime -run TestOptionalConditionalInterfaces -count=1`

Expected: FAIL，缺少新接口或审计动作。

- [ ] **Step 3: Write minimal implementation**

```go
var (
	ErrMetadataUnsupported = errors.New("artifact metadata unsupported")
	ErrMetadataUnavailable = errors.New("artifact metadata unavailable")
)

type ArtifactMetadata struct { Attributes map[string]string }

type ArtifactMetadataFetcher interface {
	FetchArtifactMetadata(context.Context, string, ArtifactKey) (*ArtifactMetadata, error)
}

type ConditionalBlocker interface {
	RequiredAttributes(packageType, packageName, version string) []ConditionRequirement
}

type ConditionAuditLogger interface {
	LogConditionUnverified(context.Context, ConditionUnverifiedEntry)
}
```

定义 `ConditionRequirement`、`ConditionUnverifiedEntry` 的明确字段：仓库、格式、包名、版本、远端路径、规则 ID、缺失属性与原因。给 `AuditAction` 增加 `ActionConditionUnverified`。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/runtime -run TestOptionalConditionalInterfaces -count=1`

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/core/runtime/interface.go internal/core/runtime/interface_test.go internal/model/audit.go
git commit -m "feat: add conditional metadata runtime contracts"
```

### Task 2: 为阻断服务提供无回源的属性需求预判

**Files:**
- Modify: `internal/service/block_rule_service.go`
- Modify: `internal/service/block_rule_service_test.go`
- Modify: `cmd/registry/runtime_init.go`

**Consumes:** 现有 `conditionalRules` 缓存及 `matchPkgNameVersion`。
**Produces:** `BlockRuleService.RequiredAttributes` 与 `blockRuleBlocker.RequiredAttributes`。

- [ ] **Step 1: Write the failing test**

```go
func TestRequiredAttributesOnlyReturnsPotentialConditionalRules(t *testing.T) {
	svc, _ := setupBlockRuleService(t)
	// 创建 npm lodash@4.* 的 license 条件规则，以及 all 的 publish_time 条件规则。
	requirements := svc.RequiredAttributes("npm", "lodash", "4.17.21")
	assertRequirementAttributes(t, requirements, "license", "published_at")
	if got := svc.RequiredAttributes("npm", "other", "1.0.0"); len(got) != 1 {
		t.Fatalf("unmatched package must retain only all rule, got %#v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service -run TestRequiredAttributesOnlyReturnsPotentialConditionalRules -count=1`

Expected: FAIL，方法不存在。

- [ ] **Step 3: Write minimal implementation**

在 service 包内定义 `ConditionalRuleRequirement{RuleID uint, Attribute string}`，并让 `RequiredAttributes` 返回该类型。先按 `IsBlocked` 同样的 TTL 规则确保缓存已刷新；读取 `conditionalRules[pkgType]` 与 `conditionalRules[model.PackageTypeAll]`；复用 `matchPkgNameVersion`；按 Rule ID 和属性名去重。license 映射为 `license`，publish_time 映射为 `published_at`。不得调用网络或产生审计。

`blockRuleBlocker.RequiredAttributes` 将 service 的需求类型映射为 runtime 的 `ConditionRequirement`；service 出错返回空切片并写 warning，保证下载可用性。这样 Service 与 Runtime 之间不会产生反向依赖。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/service -run 'TestRequiredAttributes|TestIsBlockedWithArtifact' -count=1`

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/service/block_rule_service.go internal/service/block_rule_service_test.go cmd/registry/runtime_init.go
git commit -m "feat: expose conditional block metadata requirements"
```

### Task 3: 在 ProxyRuntime 中按需评估属性并审计放行

**Files:**
- Modify: `internal/core/runtime/proxy.go`
- Modify: `internal/core/runtime/proxy_test.go`
- Modify: `cmd/registry/runtime_init.go`

**Consumes:** Task 1 的可选接口和 Task 2 的 `ConditionalBlocker`。
**Produces:** 带单飞、短超时、不可得 TTL 的 `evaluateConditionalAccess`；AuditService 适配器。

- [ ] **Step 1: Write the failing test**

```go
func TestGetArtifactBlocksFromFetchedConditionalMetadataBeforeBlobFetch(t *testing.T) {
	rt, remote, fetcher := newConditionalProxyRuntime(t)
	remote.metadata.Exists = true
	fetcher.metadata = &ArtifactMetadata{Attributes: map[string]string{"license": "GPL-3.0"}}
	_, err := rt.GetArtifact(context.Background(), npmArtifactKey())
	if !errors.Is(err, ErrBlocked) { t.Fatalf("got %v, want ErrBlocked", err) }
	if remote.blobFetches != 0 { t.Fatalf("blob fetched before conditional block") }
}

func TestGetArtifactAllowsAndAuditsWhenConditionalMetadataUnavailable(t *testing.T) {
	rt, _, fetcher := newConditionalProxyRuntime(t)
	fetcher.err = ErrMetadataUnavailable
	if _, err := rt.GetArtifact(context.Background(), npmArtifactKey()); err != nil { t.Fatal(err) }
	if got := rt.ConditionAudit.(*conditionAuditMock).entries; len(got) != 1 {
		t.Fatalf("audit entries=%d", len(got))
	}
}
```

另加：无候选规则不调用 fetcher；已缓存 Attributes 不调用 fetcher；并发十次同一 key 只调用一次；不可得 TTL 内第二次不调用；超时返回成功下载且审计原因为 `fetch_failed`。

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/runtime -run 'TestGetArtifact.*Conditional' -count=1`

Expected: FAIL，`ProxyRuntime` 尚未执行元数据评估。

- [ ] **Step 3: Write minimal implementation**

给 `ProxyRuntime` 增加：

```go
ConditionAudit       ConditionAuditLogger
MetadataFetchTimeout time.Duration // default 2s
MetadataFailureTTL   time.Duration // default 5m
```

使用已有 `fetchGroup`，key 为 `attrs:` 加上 repository、format、name、version。增加受 mutex 保护的不可得缓存；只缓存 `ErrMetadataUnsupported`、`ErrMetadataUnavailable` 和请求错误，不缓存成功结果。成功属性合并到 `artifact.Attributes` 并 `MetadataStore.Put`。

`evaluateConditionalAccess` 按顺序：

1. 类型断言 `ConditionalBlocker`，无候选立即返回；
2. 使用 Artifact 中已有属性补齐需求；
3. 若仍缺失，类型断言 `ArtifactMetadataFetcher`，以 `context.WithTimeout(ctx, timeout)` 获取属性；
4. 属性完整时调用已有 `IsBlockedWithAttrs`；命中即返回 `ErrBlocked`；
5. 任何不可得路径调用 `ConditionAudit.LogConditionUnverified` 并返回 nil。

在内存命中、MetadataStore 命中和首次创建 artifact 的路径都在 `openArtifactContent` / `ensureArtifactBlob` 前调用该方法。保留原有无条件 `checkBlocked`。

在 `runtime_init.go` 中实现 `conditionAuditLoggerAdapter`，将 entry details 编码为 JSON 后调用 `AuditService.LogWithRequestAndStatus`，动作为 `model.ActionConditionUnverified`。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/runtime -count=1`

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/core/runtime/proxy.go internal/core/runtime/proxy_test.go cmd/registry/runtime_init.go
git commit -m "feat: evaluate conditional metadata before proxy downloads"
```

### Task 4: 接入 npm、PyPI 与 Maven 元数据获取器

**Files:**
- Modify: `internal/plugins/npm/plugin.go`
- Modify: `internal/plugins/npm/plugin_test.go`
- Modify: `internal/plugins/pypi/plugin.go`
- Modify: `internal/plugins/pypi/plugin_test.go`
- Modify: `internal/plugins/maven/plugin.go`
- Modify: `internal/plugins/maven/plugin_test.go`

**Consumes:** Task 1 的 `ArtifactMetadataFetcher`。
**Produces:** 三个插件的 `FetchArtifactMetadata` 实现。

- [ ] **Step 1: Write npm、PyPI 与 Maven failing tests**

```go
func TestNPMFetchArtifactMetadataReturnsVersionLicenseAndPublishTime(t *testing.T) {
	p := NewNPMPlugin(testHTTPClient(npmVersionMetadataFixture()))
	meta, err := p.FetchArtifactMetadata(context.Background(), "https://registry.example",
		runtime.ArtifactKey{Name: "lodash", Version: "4.17.21"})
	if err != nil { t.Fatal(err) }
	if meta.Attributes["license"] != "MIT" { t.Fatal("license missing") }
	if meta.Attributes["published_at"] == "" { t.Fatal("published_at missing") }
}
```

PyPI fixture 必须模拟 JSON API 的指定 release 并断言 license 与最早上传时间。Maven fixture 必须模拟 version POM，断言解析到 license；没有 POM 或 POM 无 license 时断言 `ErrMetadataUnavailable`。

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/plugins/npm ./internal/plugins/pypi ./internal/plugins/maven -run 'Test.*FetchArtifactMetadata' -count=1`

Expected: FAIL，方法不存在。

- [ ] **Step 3: Write minimal implementations**

- npm：请求 package metadata，复用 `extractNpmVersionAttributes`，按 key.Version 选择版本；无该版本返回 `ErrMetadataUnavailable`。
- PyPI：复用 JSON API 的 release 数据及 `selectPyPILicense`；按 key.Version 选择 release，汇总最早上传时间；无法从 key 确定 name/version 返回 `ErrMetadataUnavailable`。
- Maven：按 key 的 group/artifact/version 组合 POM 路径，复用 POM license 解析；无法定位 POM 或缺少 license 返回 `ErrMetadataUnavailable`。

每个实现必须使用插件已有 HTTP client 与传入 context，不得在 `Handle` 中调用。上游 404 映射 `ErrMetadataUnavailable`；协议不能表达所需属性也返回该错误。

- [ ] **Step 4: Run plugin tests**

Run: `go test ./internal/plugins/npm ./internal/plugins/pypi ./internal/plugins/maven -count=1`

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/plugins/npm internal/plugins/pypi internal/plugins/maven
git commit -m "feat: fetch conditional metadata for npm pypi maven"
```

### Task 5: 端到端回归与运行时验证

**Files:**
- Modify: `cmd/registry/runtime_init_test.go`
- Test: `internal/core/runtime/proxy_test.go`

**Consumes:** Tasks 1–4。
**Produces:** 启动装配覆盖与完整回归证据。

- [ ] **Step 1: Write the failing wiring test**

```go
func TestCreateRuntimeForRepoWiresConditionAudit(t *testing.T) {
	rt := createProxyRuntimeForTest(t)
	proxy := rt.(*runtime.ProxyRuntime)
	if proxy.ConditionAudit == nil { t.Fatal("condition audit is not wired") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/registry -run TestCreateRuntimeForRepoWiresConditionAudit -count=1`

Expected: FAIL，直到装配完成。

- [ ] **Step 3: Complete defaults and wiring**

确认所有 `ProxyRuntime` 构造位置都设置 2 秒属性超时、5 分钟不可得 TTL、audit adapter；nil fetcher 与 nil blocker 不 panic。若已有 metrics 约定，新增属性请求 `success` / `unavailable` / `error` 计数；没有合适现有指标则不新增 Prometheus 指标，依赖审计日志。

- [ ] **Step 4: Run verification**

Run: `go test ./internal/service ./internal/core/runtime ./internal/plugins/npm ./internal/plugins/pypi ./internal/plugins/maven ./cmd/registry -count=1`

Expected: PASS。

Run: `go test ./... -count=1`

Expected: PASS。

- [ ] **Step 5: Check cleanup and commit**

Run: `rg -n '\[DEBUG-[^]]+\]' internal cmd || true`

Expected: no output。

```bash
git add cmd/registry/runtime_init_test.go internal/core/runtime/proxy_test.go
git commit -m "test: cover conditional metadata runtime wiring"
```
