# Plugin 包协议实现 Bug 修复方案

> 审查范围：`internal/plugins/` 下 6 个协议插件（Maven / NPM / PyPI / Go / YUM / APT / Raw）
> 审查日期：2026-06-03
> 状态：待批准

---

## 一、Go Plugin（4 个问题）

### BUG-GO-1 [必须修复] 缺少 `decodeGoPath`，大写模块路径无法正确解析

**文件**: `internal/plugins/go/plugin.go`

**问题**: Go Module Proxy 协议规定，客户端请求中大写字母会被编码为 `!a`-`!z`（如 `github.com/Azure/...` → `github.com/!azure/...`）。当前只有 `encodeGoPath`（向上游请求时编码），缺少解码函数处理客户端发来的编码路径。

**影响**: 所有包含大写字母的 Go 模块（`github.com/Azure/...`、`github.com/BurntSushi/toml`、`github.com/GoogleCloudPlatform/...`）全部 404。

**复现**:
```bash
# 客户端请求
GET /repository/go-proxy/github.com/!azure/azure-sdk-go/@v/list

# 当前 modulePath = "github.com/!azure/azure-sdk-go"（未解码）
# encodeGoPath 再次编码 → "github.com/!!azure/azure-sdk-go" → 上游 404
```

**方案**: 新增 `decodeGoPath` 函数，在 `Handle` 入口处对 path 中的 `!a`-`!z` 解码回大写字母：

```go
func decodeGoPath(path string) string {
    var b strings.Builder
    b.Grow(len(path))
    i := 0
    for i < len(path) {
        if path[i] == '!' && i+1 < len(path) && path[i+1] >= 'a' && path[i+1] <= 'z' {
            b.WriteByte(path[i+1] - 32) // lowercase → uppercase
            i += 2
        } else {
            b.WriteByte(path[i])
            i++
        }
    }
    return b.String()
}
```

在 `Handle` 中，解析 modulePath 后调用 `decodeGoPath`：

```go
// handleLatest
modulePath := decodeGoPath(strings.TrimSuffix(path, "/@latest"))

// handleVersionList
modulePath := decodeGoPath(strings.TrimSuffix(path, "/@v/list"))

// handleModuleDownload
modulePath := decodeGoPath(parts[0])
```

---

### BUG-GO-2 [必须修复] `.info` 端点返回 `time.Now()` 而非版本实际发布时间

**文件**: `internal/plugins/go/plugin.go` 第 441-446 行

**问题**: Go Module Proxy 协议规定 `.info` 的 `Time` 字段必须是该版本的实际提交/发布时间。当前直接用 `time.Now()` 生成，导致每次请求返回不同时间。

**影响**:
- Go CLI 本地缓存反复失效，每次都重新下载
- 版本排序错误（所有版本看起来都是"刚刚发布"）
- 构建不可复现

**方案**: 不再动态生成 `.info`，改为通过 `GetArtifact` 从缓存/上游获取原始 `.info` 内容。在 `FetchRemote` 中，对 `.info` 文件也返回 artifact 引用，让 ProxyRuntime 缓存上游的原始 `.info` 响应：

```go
// handleModuleDownload 中 .info 的处理改为：
if fileType == "info" {
    key := runtime.ArtifactKey{
        RepositoryID: ctx.Repository.ID,
        Format:       "go",
        Coordinates: map[string]string{
            "name":     modulePath,
            "module":   modulePath,
            "version":  cleanVersion,
            "path":     modulePath + "/@v",
            "ext":      "info",
            "filename": filename,
        },
        Filename: filename,
    }
    artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key)
    if err != nil {
        if errors.Is(err, runtime.ErrNotFound) {
            http.Error(ctx.Writer, "Not found", http.StatusNotFound)
        } else {
            http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
        }
        return nil
    }
    if artifact.Content == nil {
        http.Error(ctx.Writer, "Not found", http.StatusNotFound)
        return nil
    }
    defer artifact.Content.Close()
    ctx.Writer.Header().Set("Content-Type", "application/json")
    ctx.Writer.WriteHeader(http.StatusOK)
    io.Copy(ctx.Writer, artifact.Content)
    return nil
}
```

同时在 `FetchRemote` 中，对 `.info` 路径返回 artifact 引用（与 `.mod`、`.zip` 一致），确保 ProxyRuntime 会缓存上游的 `.info` 原始响应。

---

### BUG-GO-3 [建议修改] `handleModuleDownload` 未检查 `artifact.Content == nil`

**文件**: `internal/plugins/go/plugin.go` 第 472 行

**问题**: 直接 `defer artifact.Content.Close()`，如果 Content 为 nil 会 panic。其他插件（Maven、PyPI、Raw）都有 nil 检查。

**方案**: 在 `defer artifact.Content.Close()` 前添加 nil 检查：

```go
if artifact.Content == nil {
    http.Error(ctx.Writer, "Not found", http.StatusNotFound)
    return nil
}
defer artifact.Content.Close()
```

---

### BUG-GO-4 [建议修改] `fetchVersionList` 并发获取 info 无并发控制

**文件**: `internal/plugins/go/plugin.go` 第 136-146 行

**问题**: 虽然限制了最多获取 10 个版本的 info，但 10 个 goroutine 同时发起 HTTP 请求，在上游响应慢时可能耗尽连接池。

**方案**: 使用带缓冲的 channel 作为 semaphore 限制并发数：

```go
const maxInfoFetches = 10
const maxConcurrent = 3
sem := make(chan struct{}, maxConcurrent)

var wg sync.WaitGroup
for _, a := range artifacts[infoStart:] {
    wg.Add(1)
    go func(art *runtime.Artifact) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        info, err := p.fetchVersionInfo(ctx, remoteURL, art.Coordinates["module"], art.Coordinates["version"])
        if err == nil && info.Time != "" {
            art.Properties = map[string]string{"published_at": info.Time}
        }
    }(a)
}
wg.Wait()
```

---

## 二、NPM Plugin（3 个问题）

### BUG-NPM-1 [必须修复] Scoped 包的版本提取逻辑错误

**文件**: `internal/plugins/npm/plugin.go` 第 268 行

**问题**: npm 规范中 scoped 包的 tarball 文件名不含 scope 前缀（`@babel/core` 的 tarball 是 `core-7.22.0.tgz`，不是 `@babel/core-7.22.0.tgz`）。当前用 `packageName+"-"` 做 TrimPrefix，对 scoped 包不匹配。

**影响**: 所有 scoped 包（`@babel/core`、`@vue/cli`、`@types/node` 等）下载返回 404。

**复现**:
```bash
npm install @babel/core@7.22.0
# 请求: GET /@babel/core/-/core-7.22.0.tgz
# TrimPrefix("core-7.22.0.tgz", "@babel/core-") → 无效
# version = "core-7.22.0"（错误）→ GetArtifact 404
```

**方案**: 对 scoped 包，提取 `/` 后的短名称做 TrimPrefix：

```go
func extractNpmVersionFromTarball(packageName, filename string) string {
    nameForTrim := packageName
    if idx := strings.LastIndex(packageName, "/"); idx >= 0 {
        nameForTrim = packageName[idx+1:] // "@babel/core" → "core"
    }
    return strings.TrimSuffix(strings.TrimPrefix(filename, nameForTrim+"-"), ".tgz")
}
```

在 `handleTarballDownload` 和 `handleTarballDelete` 中使用此函数替换原有逻辑。

---

### BUG-NPM-2 [建议修改] `handlePackagePut` 将整个 base64 tarball 解码到内存

**文件**: `internal/plugins/npm/plugin.go` 第 511 行

**问题**: 虽然有 100MB 限制，但 base64 字符串 + 解码后字节数组同时存在于内存，峰值约为 tarball 的 2.5-3 倍。

**方案**: 使用 `base64.NewDecoder` 流式解码，避免同时持有完整字符串和字节数组：

```go
// 替换 base64.StdEncoding.DecodeString 为流式解码
decodedReader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(data))
tarballBlob, err := session.PutBlob(ctx.Request.Context(), decodedReader)
```

---

### BUG-NPM-3 [建议修改] `dist-tags` 只计算了 `latest`，丢失自定义 tag

**文件**: `internal/plugins/npm/plugin.go` 第 407-413 行

**问题**: npm 支持自定义 dist-tags（`next`、`beta`、`canary`），但当前只根据 semver 排序生成 `latest`。

**方案**: 在上传时将 dist-tags 信息存储到 artifact 的 Properties 中（如 `dist-tag: next`），在 `handlePackageGet` 时从 artifacts 中提取并聚合：

```go
distTags := map[string]string{}
for _, artifact := range artifacts {
    if tag := artifact.Properties["dist-tag"]; tag != "" {
        v := artifact.Coordinates["version"]
        if v != "" {
            distTags[tag] = v
        }
    }
}
if len(distTags) == 0 && len(versionList) > 0 {
    // fallback: 自动计算 latest
    distTags["latest"] = versionList[0]
}
```

---

## 三、PyPI Plugin（4 个问题）

### BUG-PYPI-1 [必须修复] HTML 包文件链接缺少 `#sha256=` hash fragment

**文件**: `internal/plugins/pypi/plugin.go` 第 286-290 行

**问题**: PEP 503 规定 Simple API 的包文件链接必须包含 `#sha256=<digest>` hash fragment。pip 依赖此做完整性校验。

**影响**:
- `--require-hashes` 模式直接报错
- 普通安装产生 `WARNING: The hashes of the source archive are not verified`
- 存在中间人攻击风险

**方案**: 在生成 HTML 链接时追加 hash fragment：

```go
for _, artifact := range artifacts {
    // ... 现有过滤逻辑 ...
    hashFragment := ""
    for _, blobRef := range artifact.BlobRefs {
        if blobRef.Algorithm == "sha256" {
            hashFragment = "#sha256=" + blobRef.Digest
            break
        }
    }
    sb.WriteString(`<a href="../../packages/`)
    sb.WriteString(html.EscapeString(remotePath))
    sb.WriteString(html.EscapeString(hashFragment))
    sb.WriteString(`">`)
    sb.WriteString(html.EscapeString(filename))
    sb.WriteString(`</a><br>` + "\n")
}
```

---

### BUG-PYPI-2 [建议修改] `normalizePackageName` 不完全符合 PEP 503

**文件**: `internal/plugins/pypi/plugin.go` 第 662-664 行

**问题**: PEP 503 规定包名规范化应将 `[-_.]+`（连续的横线、点、下划线）统一替换为单个 `-`。当前只替换了 `_`，未处理 `.` 和连续字符。

**影响**: `My..Pkg___Name` 应规范化为 `my-pkg-name`，当前变成 `my..pkg---name`，可能导致包名匹配失败。

**方案**:

```go
import "regexp"

var normalizeRe = regexp.MustCompile(`[-_.]+`)

func normalizePackageName(name string) string {
    return normalizeRe.ReplaceAllString(strings.ToLower(name), "-")
}
```

---

### BUG-PYPI-3 [建议修改] `handleJsonAPI` 中 `url` 字段路径构造错误

**文件**: `internal/plugins/pypi/plugin.go` 第 477 行

**问题**: 使用 `"../../packages/" + filename` 构造 URL，但实际下载路径需要包含 hash 目录前缀（如 `packages/62/35/.../filename`）。

**方案**: 使用 `artifact.Properties["remote_path"]` 构造完整路径：

```go
remotePath := artifact.Properties["remote_path"]
if remotePath == "" {
    remotePath = filename
}
file := map[string]interface{}{
    "filename": filename,
    "url":      "../../packages/" + remotePath,
}
```

---

### BUG-PYPI-4 [建议修改] `handlePackagesDownload` 忽略了 `io.Copy` 的错误返回

**文件**: `internal/plugins/pypi/plugin.go` 第 392 行

**问题**: `io.Copy(ctx.Writer, artifact.Content)` 未检查错误返回，其他插件都检查了。

**方案**:

```go
if _, err := io.Copy(ctx.Writer, artifact.Content); err != nil {
    return err
}
```

---

## 四、Maven Plugin（3 个问题）

### BUG-MAVEN-1 [建议修改] SNAPSHOT timestamp 生成逻辑有误

**文件**: `internal/plugins/maven/plugin.go` 第 431 行

**问题**: `strings.TrimSuffix(lastUpdated, lastUpdated[len(lastUpdated)-2:])` 意图是去掉最后 2 个字符，但写法晦涩且结果格式不正确。Maven SNAPSHOT timestamp 格式应为 `YYYYMMDD.HHmmss`。

**方案**:

```go
if len(lastUpdated) >= 14 {
    ts = lastUpdated[:8] + "." + lastUpdated[8:14]
} else {
    ts = lastUpdated
}
```

---

### BUG-MAVEN-2 [建议修改] SNAPSHOT 版本过滤逻辑遗漏时间戳版本

**文件**: `internal/plugins/maven/plugin.go` 第 395-397 行

**问题**: Maven SNAPSHOT 的实际版本号格式为 `1.0.0-20231201.120000-1`，而 `version` 是 `1.0.0-SNAPSHOT`。`strings.HasPrefix("1.0.0-20231201.120000-1", "1.0.0-SNAPSHOT-")` 为 false，时间戳版本被过滤掉。

**方案**: 用 SNAPSHOT 前缀匹配替代：

```go
if version != "" && strings.Contains(version, "-SNAPSHOT") {
    baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
    if v != version && !strings.HasPrefix(v, baseVersion+"-") {
        continue
    }
} else if version != "" && v != version {
    continue
}
```

---

### BUG-MAVEN-3 [建议修改] `parseMavenPath` 不处理 classifier

**文件**: `internal/plugins/maven/plugin.go` 第 507-537 行

**问题**: Maven artifact 文件名格式为 `{artifactId}-{version}[-classifier].{ext}`，当前没有从文件名中提取 classifier，导致 `sources`、`javadoc` 等 classifier artifact 无法正确区分。

**方案**: 从文件名中提取 classifier：

```go
// 在 parseMavenPath 中，解析 filename 提取 classifier
baseName := strings.TrimSuffix(filename, ext) // e.g. "mylib-1.0.0-sources"
prefix := artifact + "-" + version + "-"       // e.g. "mylib-1.0.0-"
if strings.HasPrefix(baseName, prefix) {
    classifier = strings.TrimPrefix(baseName, prefix) // "sources"
}
```

---

## 五、YUM Plugin（2 个问题）

### BUG-YUM-1 [必须修复] 动态生成的 repomd.xml 结构不完整

**文件**: `internal/plugins/yum/plugin.go` 第 283-307 行

**问题**: 动态生成的 repomd.xml 只包含 `type` 和 `location`，缺少 `checksum`、`timestamp`、`size` 等必要字段。yum/dnf 客户端依赖这些字段做完整性校验和缓存判断。

**影响**: yum/dnf 大概率报错 `Cannot retrieve repository metadata (repomd.xml)`，无法使用该仓库。

**方案**: 优先使用 `GetArtifact` 获取上游缓存的原始 repomd.xml，仅在完全无法获取时才动态生成，且动态生成时补充必要字段：

```go
// 优先走 GetArtifact 获取原始 repomd.xml
key := runtime.ArtifactKey{
    RepositoryID: ctx.Repository.ID,
    Format:       "yum",
    Coordinates:  map[string]string{"file": "repomd.xml"},
    Filename:     "repomd.xml",
}
if artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), key); err == nil && artifact.Content != nil {
    // 直接返回上游的原始 repomd.xml
    defer artifact.Content.Close()
    ctx.Writer.Header().Set("Content-Type", "application/xml")
    ctx.Writer.WriteHeader(http.StatusOK)
    io.Copy(ctx.Writer, artifact.Content)
    return nil
}
// fallback: 动态生成（补充 checksum、timestamp 等字段）
```

---

### BUG-YUM-2 [建议修改] 动态生成 repomd.xml 时 `type` 被硬编码为 `"primary"`

**文件**: `internal/plugins/yum/plugin.go` 第 300 行

**问题**: `d := data{Type: "primary"}` 硬编码，但 artifacts 可能包含 `filelists`、`other`、`updateinfo` 等类型。

**方案**: 从 artifact 的 Properties 或 Coordinates 中读取原始 type：

```go
d := data{Type: a.Coordinates["type"]}
if d.Type == "" {
    d.Type = "primary"
}
```

---

## 六、APT Plugin（1 个问题）

### BUG-APT-1 [建议修改] 不支持压缩的 Packages 索引文件

**文件**: `internal/plugins/apt/plugin.go` 第 235-237 行

**问题**: APT 客户端通常请求 `Packages.gz` 或 `Packages.xz`（压缩格式），但 `isPackagesRequest` 匹配到后，`handlePackages` 返回未压缩内容，apt 客户端解析失败。

**方案**: 与 YUM repomd.xml 同理，优先通过 `GetArtifact` 获取上游缓存的原始压缩文件。对 `.gz`/`.xz` 后缀的请求，直接透传上游响应：

```go
func (p *AptPlugin) isPackagesRequest(path string) bool {
    return strings.Contains(path, "Packages") &&
        (strings.HasSuffix(path, "Packages") ||
         strings.HasSuffix(path, "Packages.gz") ||
         strings.HasSuffix(path, "Packages.xz") ||
         strings.HasSuffix(path, "Packages.bz2"))
}
```

在 `handlePackages` 中，对压缩格式直接 `GetArtifact` 透传，不做动态生成。

---

## 七、Raw/Generic Plugin（1 个问题）

### BUG-RAW-1 [建议修改] `handleModuleDownload` 中 `artifact.Content` nil 检查

> 此问题已在 BUG-GO-3 中统一描述，Raw 插件本身已有 nil 检查，无需额外修改。

---

## 修复优先级排序

| 优先级 | Bug ID | 概述 | 影响范围 |
|--------|--------|------|----------|
| P0 | BUG-GO-1 | 缺少 decodeGoPath | 所有含大写字母的 Go 模块不可用 |
| P0 | BUG-NPM-1 | Scoped 包版本提取错误 | 所有 @scope/pkg 形式的 NPM 包下载 404 |
| P0 | BUG-GO-2 | .info 返回 time.Now() | Go 模块缓存失效、版本排序错误 |
| P0 | BUG-PYPI-1 | 缺少 #sha256= hash fragment | pip --require-hashes 失败、安全风险 |
| P0 | BUG-YUM-1 | repomd.xml 结构不完整 | yum/dnf 无法使用该仓库 |
| P1 | BUG-MAVEN-2 | SNAPSHOT 时间戳版本被过滤 | Maven SNAPSHOT 版本列表不完整 |
| P1 | BUG-PYPI-2 | normalizePackageName 不完整 | 部分包名匹配失败 |
| P1 | BUG-PYPI-3 | JSON API url 路径错误 | PyPI JSON API 下载链接 404 |
| P1 | BUG-YUM-2 | repomd.xml type 硬编码 | 非 primary 类型的 metadata 丢失 |
| P1 | BUG-APT-1 | 不支持压缩 Packages 索引 | apt 客户端解析失败 |
| P2 | BUG-GO-3 | Content nil 检查缺失 | 潜在 panic |
| P2 | BUG-GO-4 | 并发获取 info 无控制 | 连接池耗尽风险 |
| P2 | BUG-MAVEN-1 | SNAPSHOT timestamp 格式错误 | SNAPSHOT 元数据格式不合规 |
| P2 | BUG-MAVEN-3 | 不处理 classifier | sources/javadoc 等 artifact 无法区分 |
| P2 | BUG-NPM-2 | base64 全量解码到内存 | 大包内存压力 |
| P2 | BUG-NPM-3 | dist-tags 丢失 | 自定义 tag 不可用 |
| P2 | BUG-PYPI-4 | io.Copy 错误未检查 | 错误被静默吞掉 |
