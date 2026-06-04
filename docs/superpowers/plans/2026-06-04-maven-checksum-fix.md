# Maven Checksum 校验失败修复计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 修复 Maven 客户端在 `mvn compile` 时出现 "Checksum validation failed" 警告的问题

**架构：** 在 Maven 插件的 `Handle` 方法中拦截 checksum 文件请求（`.sha1`、`.md5`、`.sha256`），提取对应的原始 artifact 文件名，通过 Runtime 获取原始 artifact 内容，动态计算 checksum 并返回。这避免了上游 checksum 文件与本地缓存 artifact 内容不同步的问题。

**技术栈：** Go 1.24, crypto/sha1, crypto/md5, crypto/sha256

---

## 问题根因

Maven 客户端下载 `.jar`/`.pom` 等文件后，会同时请求对应的 `.sha1` 文件来校验完整性。当前 Maven 插件对 `.sha1`/`.md5` 文件没有任何特殊处理——它们和普通 artifact 走完全相同的代理路径。

在 `ProxyRuntime.GetArtifact()` 中，artifact 和其 `.sha1` 是分别独立获取和缓存的。当上游更新了某个版本的文件，可能出现 `.jar` 和 `.sha1` 缓存不同步的情况，导致 Maven 客户端拿到的 `.jar` 的实际 SHA1 与 `.sha1` 文件中的值不匹配。

## 文件结构

| 文件 | 职责 | 操作 |
|------|------|------|
| `internal/plugins/maven/plugin.go` | Maven 协议插件 — 添加 checksum 请求拦截和动态计算 | **修改** |
| `internal/plugins/maven/checksum.go` | Maven checksum 计算和格式化逻辑（独立文件，职责清晰） | **新建** |
| `internal/plugins/maven/checksum_test.go` | Checksum 计算的单元测试 | **新建** |

## 任务列表

### 任务 1：新建 checksum 计算辅助文件

**文件：**
- 创建：`internal/plugins/maven/checksum.go`

- [ ] **步骤 1：创建 checksum.go，包含检测和计算逻辑**

```go
package maven

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

// checksumAlgo 表示 Maven checksum 算法类型
type checksumAlgo string

const (
	checksumSHA1   checksumAlgo = "sha1"
	checksumMD5    checksumAlgo = "md5"
	checksumSHA256 checksumAlgo = "sha256"
)

// parseChecksumRequest 检测文件名是否为 checksum 文件请求。
// 如果是，返回原始文件名、算法类型和 true；否则返回 false。
//
// Maven checksum 文件命名规则：
//   - my-lib-1.0.0.jar.sha1   → 原始文件: my-lib-1.0.0.jar, 算法: sha1
//   - my-lib-1.0.0.jar.md5    → 原始文件: my-lib-1.0.0.jar, 算法: md5
//   - my-lib-1.0.0.jar.sha256 → 原始文件: my-lib-1.0.0.jar, 算法: sha256
func parseChecksumRequest(filename string) (originalFile string, algo checksumAlgo, ok bool) {
	if strings.HasSuffix(filename, ".sha256") {
		return strings.TrimSuffix(filename, ".sha256"), checksumSHA256, true
	}
	if strings.HasSuffix(filename, ".sha1") {
		return strings.TrimSuffix(filename, ".sha1"), checksumSHA1, true
	}
	if strings.HasSuffix(filename, ".md5") {
		return strings.TrimSuffix(filename, ".md5"), checksumMD5, true
	}
	return "", "", false
}

// computeChecksum 计算 reader 内容的指定算法 checksum，返回小写十六进制字符串。
func computeChecksum(reader io.Reader, algo checksumAlgo) (string, error) {
	var h io.Writer
	switch algo {
	case checksumSHA1:
		h = sha1.New()
	case checksumMD5:
		h = md5.New()
	case checksumSHA256:
		h = sha256.New()
	default:
		sha := sha1.New()
		h = sha
	}

	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}

	hasher := h.(interface{ Sum(b []byte) []byte })
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// formatMavenChecksum 格式化为 Maven 标准 checksum 格式。
// 格式: "<hex_digest>  <filename>\n"
// 两个空格分隔 digest 和 filename，这是 Maven 的标准约定。
func formatMavenChecksum(digest, filename string) string {
	return digest + "  " + filename + "\n"
}
```

- [ ] **步骤 2：Commit**

```bash
git add internal/plugins/maven/checksum.go
git commit -m "feat(maven): add checksum computation utilities"
```

---

### 任务 2：编写 checksum 单元测试

**文件：**
- 创建：`internal/plugins/maven/checksum_test.go`

- [ ] **步骤 1：编写测试**

```go
package maven

import (
	"strings"
	"testing"
)

func TestParseChecksumRequest(t *testing.T) {
	tests := []struct {
		filename       string
		wantOriginal   string
		wantAlgo       checksumAlgo
		wantOK         bool
	}{
		{"my-lib-1.0.0.jar.sha1", "my-lib-1.0.0.jar", checksumSHA1, true},
		{"my-lib-1.0.0.jar.md5", "my-lib-1.0.0.jar", checksumMD5, true},
		{"my-lib-1.0.0.jar.sha256", "my-lib-1.0.0.jar", checksumSHA256, true},
		{"my-lib-1.0.0.pom.sha1", "my-lib-1.0.0.pom", checksumSHA1, true},
		{"maven-metadata.xml.sha1", "maven-metadata.xml", checksumSHA1, true},
		{"my-lib-1.0.0.jar", "", "", false},
		{"my-lib-1.0.0.pom", "", "", false},
		{"maven-metadata.xml", "", "", false},
		{"sources.jar.sha1", "sources.jar", checksumSHA1, true},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			original, algo, ok := parseChecksumRequest(tt.filename)
			if ok != tt.wantOK {
				t.Fatalf("parseChecksumRequest(%q) ok = %v, want %v", tt.filename, ok, tt.wantOK)
			}
			if original != tt.wantOriginal {
				t.Errorf("original = %q, want %q", original, tt.wantOriginal)
			}
			if algo != tt.wantAlgo {
				t.Errorf("algo = %q, want %q", algo, tt.wantAlgo)
			}
		})
	}
}

func TestComputeChecksum(t *testing.T) {
	// "hello world" 的已知 checksum 值
	content := "hello world"

	sha1Val, err := computeChecksum(strings.NewReader(content), checksumSHA1)
	if err != nil {
		t.Fatalf("computeChecksum sha1 error: %v", err)
	}
	// SHA1("hello world") = 2aae6c35c94fcfb415dbe95f408b9ce91ee846ed
	if sha1Val != "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed" {
		t.Errorf("sha1 = %q, want %q", sha1Val, "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed")
	}

	md5Val, err := computeChecksum(strings.NewReader(content), checksumMD5)
	if err != nil {
		t.Fatalf("computeChecksum md5 error: %v", err)
	}
	// MD5("hello world") = 5eb63bbbe01eeed093cb22bb8f5acdc3
	if md5Val != "5eb63bbbe01eeed093cb22bb8f5acdc3" {
		t.Errorf("md5 = %q, want %q", md5Val, "5eb63bbbe01eeed093cb22bb8f5acdc3")
	}

	sha256Val, err := computeChecksum(strings.NewReader(content), checksumSHA256)
	if err != nil {
		t.Fatalf("computeChecksum sha256 error: %v", err)
	}
	// SHA256("hello world") = b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9
	if sha256Val != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Errorf("sha256 = %q, want %q", sha256Val, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9")
	}
}

func TestFormatMavenChecksum(t *testing.T) {
	result := formatMavenChecksum("abc123", "my-lib-1.0.0.jar")
	want := "abc123  my-lib-1.0.0.jar\n"
	if result != want {
		t.Errorf("formatMavenChecksum = %q, want %q", result, want)
	}
}
```

- [ ] **步骤 2：运行测试验证通过**

运行：`go test ./internal/plugins/maven/ -run TestParseChecksum -v && go test ./internal/plugins/maven/ -run TestComputeChecksum -v && go test ./internal/plugins/maven/ -run TestFormatMaven -v`
预期：全部 PASS

- [ ] **步骤 3：Commit**

```bash
git add internal/plugins/maven/checksum_test.go
git commit -m "test(maven): add checksum computation unit tests"
```

---

### 任务 3：在 Maven 插件 Handle 中拦截 checksum 请求

**文件：**
- 修改：`internal/plugins/maven/plugin.go:305-333`（Handle 方法）
- 修改：`internal/plugins/maven/plugin.go:785-813`（handleDownload 方法）

- [ ] **步骤 1：修改 Handle 方法，在 parseMavenPath 之前拦截 checksum 请求**

在 `Handle` 方法中，`maven-metadata.xml` 检查之后、`parseMavenPath` 之前，添加 checksum 文件检测：

```go
func (p *MavenPlugin) Handle(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	path := ctx.RepositoryPath
	path = strings.TrimPrefix(path, "/")

	if strings.HasSuffix(path, "maven-metadata.xml") && ctx.Request.Method == http.MethodGet {
		return p.handleMetadata(ctx, repoRuntime, path)
	}

	key, err := p.parseMavenPath(path)
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusBadRequest)
		return nil
	}
	key.RepositoryID = ctx.Repository.ID

	ctx.PackageName = key.Coordinates["name"]
	ctx.Version = key.Coordinates["version"]
	ctx.Filename = key.Filename

	switch ctx.Request.Method {
	case http.MethodGet:
		// 拦截 checksum 文件请求，动态计算并返回
		if originalFile, algo, ok := parseChecksumRequest(key.Filename); ok {
			return p.handleChecksumDownload(ctx, repoRuntime, key, originalFile, algo)
		}
		return p.handleDownload(ctx, repoRuntime, key)
	case http.MethodPut:
		return p.handleUpload(ctx, repoRuntime, key)
	case http.MethodDelete:
		return p.handleDelete(ctx, repoRuntime, key)
	}
	return errors.New("method not allowed")
}
```

- [ ] **步骤 2：添加 handleChecksumDownload 方法**

在 `handleDownload` 方法之前添加新方法：

```go
func (p *MavenPlugin) handleChecksumDownload(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, key runtime.ArtifactKey, originalFile string, algo checksumAlgo) error {
	// 构建原始 artifact 的 key（用原始文件名替换 checksum 文件名）
	originalKey := key
	originalKey.Filename = originalFile
	originalKey.Extension = filepath.Ext(originalFile)

	artifact, err := repoRuntime.GetArtifact(ctx.Request.Context(), originalKey)
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

	digest, err := computeChecksum(artifact.Content, algo)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"filename": key.Filename,
			"algo":     string(algo),
		}).Error("maven: compute checksum failed")
		http.Error(ctx.Writer, "internal error", http.StatusInternalServerError)
		return nil
	}

	ctx.FromCache = artifact.FromCache
	ctx.RemoteURL = artifact.RemoteURL

	ctx.Writer.Header().Set("Content-Type", "text/plain")
	ctx.Writer.WriteHeader(http.StatusOK)
	_, _ = ctx.Writer.Write([]byte(formatMavenChecksum(digest, originalFile)))
	return nil
}
```

- [ ] **步骤 3：运行全量测试验证没有回归**

运行：`go test ./internal/plugins/maven/ -v`
预期：全部 PASS

- [ ] **步骤 4：Commit**

```bash
git add internal/plugins/maven/plugin.go
git commit -m "feat(maven): intercept checksum requests and compute dynamically from artifact content"
```

---

### 任务 4：全量构建验证

- [ ] **步骤 1：编译项目**

运行：`make build`
预期：编译成功，无错误

- [ ] **步骤 2：运行全部测试**

运行：`make test`
预期：全部 PASS

- [ ] **步骤 3：运行 lint**

运行：`make lint`
预期：无 lint 错误

---

## 验证清单

修复完成后，验证以下场景：
- [ ] Proxy 仓库：`mvn compile` 不再出现 "Checksum validation failed" 警告
- [ ] Proxy 仓库：`.sha1`、`.md5`、`.sha256` 文件均能正确返回
- [ ] Hosted 仓库：上传的 artifact 的 checksum 能正确计算返回
- [ ] 不存在的 artifact 请求 checksum 返回 404
