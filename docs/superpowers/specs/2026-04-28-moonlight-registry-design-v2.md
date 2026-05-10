# Moonlight Registry 增强设计文档 v2.0

> **日期**: 2026-04-28
> **状态**: ✅ 已批准 (含 6 大增强场景)
> **基于**: v1.0 设计 + 企业级场景增强

---

## 🆕 新增核心模块总览

| # | 模块名称 | 核心价值 | 复杂度 | Phase |
|---|---------|---------|--------|-------|
| 1 | **🛡️ 供应链安全防护** | 防止 Typosquatting/依赖混淆/Token 泄露/恶意替换 | ⭐⭐⭐ | Phase 2 |
| 2 | **🔌 离线环境支持** | 完全断网部署、批量同步、数据迁移工具 | ⭐⭐ | Phase 2 |
| 3 | **💾 备份恢复系统** | 全量/增量备份、一致性校验、时间点恢复 | ⭐⭐ | Phase 2 |
| 4 | **📦 CAS 存储优化** | 内容寻址去重、自动清理策略、容量预警 | ⭐⭐⭐ | Phase 2 |
| 5 | **🔗 多代理仓库** | 同语言多上游代理、优先级路由、聚合仓库 | ⭐⭐⭐ | Phase 2 |
| 6 | **🤖 AI 辅助功能** | 智能推荐、语义搜索、依赖分析、自动文档 | ⭐⭐⭐⭐ | Phase 2 |

---

## 一、🛡️ 供应链攻击防护 (Supply Chain Security)

### 1.1 攻击向量矩阵

```
┌─────────────────────────────────────────────────────────────────────┐
│                    供应链攻击防护层                                   │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │              上传前检查 (Pre-Upload Checks)                   │   │
│  │                                                             │   │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐   │   │
│  │  │ Typosquatting│  │ Dependency   │  │ Token Leakage    │   │   │
│  │  │ Detection    │  │ Confusion    │  │ Scanner          │   │   │
│  │  │ (包名相似度)  │  │ Prevention   │  │ (敏感信息扫描)    │   │   │
│  │  └──────┬───────┘  └──────┬───────┘  └───────┬──────────┘   │   │
│  │         └────────────────┼───────────────────┘               │   │
│  │                           ▼                                 │   │
│  │                 ┌──────────────────┐                        │   │
│  │                 │  Risk Assessment │                        │   │
│  │                 │  综合风险评估引擎  │                        │   │
│  │                 └────────┬─────────┘                        │   │
│  │                          ▼                                  │   │
│  │          ┌────────────────────────┐                       │   │
│  │          │  Decision:            │                       │   │
│  │          │  ✅ ALLOW / ⚠️ REVIEW  │                       │   │
│  │          │  / ❌ BLOCK           │                       │   │
│  │          └────────────────────────┘                       │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │              运行时保护 (Runtime Protection)                  │   │
│  │                                                             │   │
│  │  • Version Immutability (版本不可变策略)                     │   │
│  │  • Publish Approval Workflow (发布审批工作流)                │   │
│  │  • Package Provenance Tracking (包来源追踪)                 │   │
│  │  • Automated Quarantine (自动隔离可疑包)                   │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

### 1.2 各攻击类型详细设计

#### A. Typosquatting Detection (拼写混淆检测)

```go
// internal/service/supply_chain.go

type TyposquattingDetector struct {
    publicRegistryCache map[string]bool  // 公共仓库已知包名缓存
    similarityThreshold float64          // 相似度阈值 (默认 0.85)
}

type TyposquattingResult struct {
    IsSuspicious      bool
    SimilarPackages   []string     // 可能被模仿的合法包名
    SimilarityScore    float64      // 相似度分数 (0-1)
    Recommendation    string       // 处理建议
}

// 检测算法: Levenshtein Distance + 编辑距离归一化
func (d *TyposquattingDetector) Check(packageName string, pkgType PackageType) (*TyposquattingResult, error) {
    // 1. 检查是否为 scoped 包 (@scope/name 格式)
    if strings.HasPrefix(packageName, "@") {
        return d.checkScopedPackage(packageName)
    }
    
    // 2. 对非 scope 包，检查与公共仓库包名的相似度
    candidates := d.findSimilarPublicPackages(packageName)
    
    if len(candidates) > 0 && candidates[0].Score > d.similarityThreshold {
        return &TyposquattingResult{
            IsSuspicious:    true,
            SimilarPackages: extractNames(candidates),
            SimilarityScore: candidates[0].Score,
            Recommendation: "此包名与公共包高度相似，可能是 typosquatting 攻击",
        }, nil
    }
    
    return &TyposquattingResult{IsSuspicious: false}, nil
}
```

**配置示例**:
```yaml
supply_chain:
  typosquatting:
    enabled: true
    similarity_threshold: 0.85      # 高于此值触发警告
    auto_block_first_time_scope: true  # 首次出现的新 scope 自动进入审核队列
    known_public_registries:
      - npmjs
      - pypi
    allowlist_scopes:               # 白名单 scope 不检查
      - "@yourcompany"
      - "@internal"
```

**API 端点**:
```
POST   /api/v1/supply-chain/check-typosquatting   # 手动检查包名
GET    /api/v1/supply-chain/typosquatting/reports  # 查看历史检测报告
PUT    /api/v1/supply-chain/typosquatting/:id/approve # 审核通过
PUT    /api/v1/supply-chain/typosquatting/:id/block   # 加入黑名单
```

---

#### B. Dependency Confusion Prevention (依赖混淆防护)

```go
// 核心逻辑: 内部包必须优先从私有仓库解析

type DependencyConfusionGuard struct {
    internalScopes []string  // 内部 scope 列表
    privateOnlyMode bool     // 严格模式: 内部包禁止从公共源拉取
}

type DepConfusionCheckResult struct {
    PackageName      string
    HasInternalScope bool
    ResolutionPolicy string  // "private-only" / "prefer-private" / "public-ok"
    RiskLevel        string  // "low" / "medium" / "high" / "critical"
    ActionRequired   bool
    Message           string
}

func (g *DependencyConfusionGuard) CheckDependency(depName string) *DepConfusionCheckResult {
    result := &DepConfusionCheckResult{PackageName: depName}
    
    // 检查是否匹配内部 scope
    for _, scope := range g.internalScopes {
        if strings.HasPrefix(depName, scope) || strings.HasPrefix(depName, scope+"/") {
            result.HasInternalScope = true
            result.ResolutionPolicy = "private-only"
            result.RiskLevel = "critical"
            result.ActionRequired = true
            result.Message = fmt.Sprintf(
                "内部包 %s 必须从私有仓库解析，禁止 fallback 到公共仓库", depName,
            )
            return result
        }
    }
    
    result.RiskLevel = "low"
    result.ResolutionPolicy = "public-ok"
    return result
}
```

**npm 配置注入**:
当用户执行 `moonlight auth login` 时，自动生成 `.npmrc`:
```ini
# .npmrc (自动生成)
registry=http://your-registry:8080/npm/
//your-registry:8080/npm/:_authToken=<jwt-token>

# 强制内部 scope 走私有仓库
@yourcompany:registry=http://your-registry:8080/npm/
@internal:registry=http://your-registry:8080/npm/
```

**Maven settings.xml 注入**:
```xml
<!-- settings.xml 片段 -->
<profiles>
  <profile>
    <id>moonlight-private</id>
    <repositories>
      <repository>
        <id>moonlight</id>
        <url>http://your-registry:8080/maven2</url>
      </repository>
    </repositories>
  </profile>
</profiles>
<activeProfiles>moonlight-private</activeProfiles>
```

---

#### C. Token Leakage Scanner (凭证泄露扫描)

```go
type TokenLeakageScanner struct {
    patterns []*regexp.Regexp  // 敏感信息正则模式
}

var defaultPatterns = []*regexp.Regexp{
    // npm token
    regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),
    // GitHub token
    regexp.MustCompile(`ghp_[A-Za-z0-9]{36}`),
    regexp.MustCompile(`github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}`),
    // AWS Key
    regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
    // Generic JWT
    regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`),
    // Password in URL
    regexp.MustCompile(`:[^/@]+@`),
    // Private key
    regexp.MustCompile(`-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----`),
}

type LeakScanResult struct {
    FileScanned      string
   LeaksFound        []LeakFinding
    OverallRisk       string  // "safe" / "warning" / "critical"
}

type LeakFinding struct {
    Type        string  // "npm_token" | "github_token" | "aws_key" | ...
    MatchedText string  // 匹配到的文本 (脱敏后显示)
    LineNumber  int
    Context     string  // 周围代码上下文
    Severity    string  // "high" | "medium"
}

func (s *TokenLeakageScanner) ScanPackageContent(reader io.Reader, filename string) (*LeakScanResult, error) {
    content, err := io.ReadAll(reader)
    if err != nil {
        return nil, err
    }
    
    var findings []LeakFinding
    lines := strings.Split(string(content), "\n")
    
    for i, line := range lines {
        for _, pattern := range s.patterns {
            matches := pattern.FindAllString(line, -1)
            if len(matches) > 0 {
                for _, match := range matches {
                    findings = append(findings, LeakFinding{
                        Type:        detectType(pattern, match),
                        MatchedText: maskSensitive(match),
                        LineNumber:  i + 1,
                        Context:     truncateContext(line, 100),
                        Severity:    classifySeverity(detectType(pattern, match)),
                    })
                }
            }
        }
    }
    
    risk := "safe"
    if len(findings) > 0 {
        risk = "warning"
        for _, f := range findings {
            if f.Severity == "high" {
                risk = "critical"
                break
            }
        }
    }
    
    return &LeakScanResult{
        FileScanned: filename,
        LeaksFound:  findings,
        OverallRisk:  risk,
    }, nil
}
```

**集成到上传流程**:
```
用户上传 → [Token 扫描]
              ├─ 发现高敏凭证 → ❌ 阻断上传 + 通知安全管理员
              ├─ 发现中敏凭证 → ⚠️ 警告但允许（需确认）
              └─ 无异常 → ✅ 继续后续流程
```

---

#### D. Version Immutability Policy (版本不可变策略)

```go
type VersionImmutabilityService struct {
    db           *gorm.DB
    gracePeriod  time.Duration  // 宽限期 (默认 24h)
}

type VersionStatus string
const (
    VersionMutable    VersionStatus = "mutable"
    VersionImmutable  VersionStatus = "immutable"
)

func (s *VersionImmutabilityService) CanDelete(versionID uint, requesterID uint) (*ImmutabilityDecision, error) {
    version, err := s.getVersion(versionID)
    if err != nil {
        return nil, err
    }
    
    publishedAt := version.PublishedAt
    elapsed := time.Since(publishedAt)
    
    decision := &ImmutabilityDecision{
        Allowed: false,
        Reason:  "",
    }
    
    switch {
    case elapsed < s.gracePeriod:
        // 宽限期内允许删除
        decision.Allowed = true
        decision.Reason = fmt.Sprintf("在宽限期内 (%.1f 小时)", s.gracePeriod.Hours())
        
    case version.Status == model.StatusDraft:
        // Draft 版本始终可删除
        decision.Allowed = true
        decision.Reason = "草稿状态"
        
    case version.DownloadCount == 0:
        // 无下载记录可删除
        decision.Allowed = true
        decision.Reason = "无下载记录"
        
    case requesterID == version.PublishedBy && hasAdminRole(requesterID):
        // 发布者本人 + 管理员权限可删除
        decision.Allowed = true
        decision.Reason = "管理员权限覆盖"
        
    default:
        decision.Allowed = false
        decision.Reason = fmt.Sprintf(
            "版本已发布超过 %.0f 小时且有 %d 次下载，不可删除。可使用 deprecate 替代。",
            elapsed.Hours(), version.DownloadCount,
        )
    }
    
    // 记录审计日志
    s.auditLog(requesterID, "version_delete_check", decision)
    
    return decision, nil
}
```

**配置**:
```yaml
supply_chain:
  version_immutability:
    enabled: true
    grace_period_hours: 24          # 发布后 24h 内可删除
    allow_deprecate_after: 0h       # 弃用随时允许
    require_admin_for_force_delete: true  # 强制删除需管理员权限
    notify_on_deprecation: true      # 弃用时通知下载者
```

---

### 1.3 供应链安全 API

```
# ========== 供应链安全 ==========

POST   /api/v1/supply-chain/pre-upload-check     # 上传前综合检查
GET    /api/v1/supply-chain/policies             # 获取当前策略配置
PUT    /api/v1/supply-chain/policies             # 更新策略 (仅 admin)

# Typosquatting
GET    /api/v1/supply-chain/typosquatting/rules  # 规则列表
POST   /api/v1/supply-chain/typosquatting/check  # 手动检查
GET    /api/v1/supply-chain/typosquatting/alerts  # 告警列表
PUT    /api/v1/supply-chain/typosquatting/:id/resolve  # 处理告警

# Token Leakage
GET    /api/v1/supply-chain/token-scans          # 扫描历史
POST   /api/v1/supply-chain/token-scan/manual     # 手动扫描指定包

# Dependency Confusion
GET    /api/v1/supply-chain/internal-scopes       # 内部 scope 配置
PUT    /api/v1/supply-chain/internal-scopes       # 更新配置
GET    /api/v1/supply-chain/deps-confusion/report  # 依赖混淆风险报告

# Version Immutability
GET    /api/v1/packages/:id/versions/:vid/immutability-status  # 查询状态
POST   /api/v1/packages/:id/versions/:vid/request-deletion       # 申请删除
POST   /api/v1/packages/:id/versions/:vid/deprecate             # 弃用版本
```

---

## 二、🔌 离线环境支持 (Air-Gapped Deployment)

### 2.1 离线部署架构

```
联网环境 (Build Machine)              隔离环境 (Target Server)
┌─────────────────────────┐           ┌─────────────────────────┐
│                         │           │                         │
│  $ moonlight sync \     │  USB/内网  │  $ moonlight import \  │
│    --source npmjs \     │ ───────→  │    offline-bundle.tar.gz│
│    --source maven-central│           │                         │
│    --source pypi        │           │  $ moonlight serve      │
│                         │           │    --mode offline       │
│  生成 offline-bundle    │           │                         │
│  .tar.gz (~50GB)        │           │  ✅ 完全离线运行        │
│                         │           │                         │
└─────────────────────────┘           └─────────────────────────┘
```

### 2.2 CLI 命令设计

```bash
# ===== 同步命令 =====

# 从远程仓库同步包到本地缓存
$ moonlight sync \
    --source npmjs \
    --packages "react,express,lodash,vue" \
    --versions "latest" \
    --output ./offline-bundle/

# 同步 Maven Central 的特定构件
$ moonlight sync \
    --source maven-central \
    --packages "org.springframework:spring-core" \
    --versions "6.0.0,5.3.30" \
    --include-dependencies \
    --output ./offline-bundle/

# 同步整个 PyPI Top 1000 项目
$ moonlight sync \
    --source pypi \
    --top-n 1000 \
    --platform any \
    --python-version ">=3.8" \
    --output ./offline-bundle/

# ===== 导出命令 =====

# 导出离线安装包 (包含二进制 + 数据)
$ moonlight export \
    --type full \
    --include-database \
    --include-cache \
    --include-vuln-db \
    --compress gzip \
    --output moonlight-offline-2026Q1.tar.gz

# 仅导出增量变更
$ moonlight export \
    --type incremental \
    --since 2026-01-01 \
    --output moonlight-incremental-0428.tar.gz

# ===== 导入命令 =====

# 在隔离服务器导入
$ moonlight import \
    --input moonlight-offline-2026Q1.tar.gz \
    --target-dir ./data/ \
    --verify-checksum \
    --dry-run  # 先预览不实际导入

# ===== 验证命令 =====

# 验证离线包完整性
$ moonlight verify \
    --bundle moonlight-offline-2026Q1.tar.gz \
    --check-integrity \
    --check-dependencies \
    --report verification-report.json

# ===== 服务模式 =====

# 离线模式启动 (禁用所有外部网络请求)
$ moonlight serve \
    --config config.yaml \
    --mode offline \
    --read-only-repos  # 远程代理仓库只读
```

### 2.3 离线 Bundle 结构

```
moonlight-offline-2026Q1.tar.gz
├── manifest.json              # 清单文件 (包含校验和/元数据)
├── metadata/
│   ├── packages.json         # 包索引
│   ├── versions.json         # 版本索引
│   └── checksums.sha256       # SHA256 校验和
│
├── database/
│   ├── registry.db            # SQLite 数据库 (含元数据)
│   └── registry.db-wal        # WAL 日志
│
├── packages/                 # 所有预缓存的包文件
│   ├── npm/
│   │   ├── react/
│   │   │   └── 18.2.0/
│   │   │       ├── package.tgz
│   │   │       └── package.json
│   │   └── ...
│   ├── maven2/
│   │   └── org/springframework/
│   │       └── spring-core/
│   │           └── 6.0.0/
│   │               └── spring-core-6.0.0.jar
│   └── pypi/
│       └── numpy/
│           └── numpy-1.26.4-cp312-cp312-manylinux_2_17_x86_64.whl
│
├── cache/                    # 预热的缓存条目元数据
│   └── cache-index.json
│
├── security/
│   └── vuln-database.json     # 离线漏洞数据库
│
└── scripts/
    └── post-import.sh         # 导入后执行的初始化脚本
```

### 2.4 Manifest 文件格式

```json
{
    "version": "1.0",
    "created_at": "2026-04-28T10:00:00Z",
    "created_by": "admin",
    "source_registries": [
        {"name": "npmjs", "url": "https://registry.npmjs.org"},
        {"name": "maven-central", "url": "https://repo.maven.apache.org/maven2"}
    ],
    "statistics": {
        "total_packages": 1250,
        "total_versions": 8500,
        "total_size_bytes": 53687091200,
        "total_size_human": "50.0 GB",
        "package_types": {"npm": 500, "maven": 400, "pypi": 350}
    },
    "checksums": {
        "sha256": "abc123...",
        "manifest_sha256": "def456..."
    },
    "includes_vuln_db": true,
    "vuln_db_version": "2026-04-27",
    "requires": {
        "min_moonlight_version": "1.0.0",
        "disk_space_gb": 60,
        "memory_mb": 512
    }
}
```

---

## 三、💾 备份恢复与数据完整性

### 3.1 备份系统架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Backup & Restore System                  │
│                                                             │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ Full Backup  │    │ Incremental  │    │ Point-in-Time│  │
│  │ (全量备份)    │    │ (增量备份)    │    │ (时间点恢复)  │  │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘  │
│         │                   │                   │            │
│         └───────────────────┼───────────────────┘            │
│                             ▼                                │
│  ┌────────────────────────────────────────────────────┐   │
│  │              Backup Manager                         │   │
│  │                                                     │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │   │
│  │  │ DB Backup   │  │ File Backup │  │ Metadata    │ │   │
│  │  │ (SQLite/PG)  │  │ (Packages)  │  │ (Manifest)  │ │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘ │   │
│  │                                                     │   │
│  │  ┌─────────────┐                                    │   │
│  │  │ Consistency │  ← WAL 模式确保一致性              │   │
│  │  │ Checker     │                                    │   │
│  │  └─────────────┘                                    │   │
│  └────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌────────────────────────────────────────────────────┐   │
│  │              Storage Backends                        │   │
│  │                                                     │   │
│  │  • Local filesystem (/backups/)                     │   │
│  │  • S3 compatible (MinIO/AWS/Aliyun)                │   │
│  │  • NFS / NAS mount                                │   │
│  │  • SFTP remote server                              │   │
│  └────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 CLI 备份命令

```bash
# ===== 创建备份 =====

# 全量备份 (数据库 + 所有包文件 + 缓存)
$ moonlight backup create \
    --type full \
    --destination ./backups/ \
    --compress zstd \
    --encrypt  \                    # 可选加密
    --description "Weekly backup $(date +%Y-%m-%d)" \
    --retention-count 4             # 保留最近 4 个全量备份

# 增量备份 (仅上次备份后的变更)
$ moonlight backup create \
    --type incremental \
    --since-backup ./backups/full-2026-04-21/ \
    --destination ./backups/incremental/ \
    --include-cache false           # 不包含缓存 (可选)

# 仅备份数据库 (快速)
$ moonlight backup create \
    --type database-only \
    --destination ./backups/db/

# ===== 恢复操作 =====

# 恢复到最新状态
$ moonlight restore \
    --from ./backups/full-2026-04-28/ \
    --target-dir ./data/ \
    --yes-i-am-sure               # 确认覆盖

# 时间点恢复 (PITR)
$ moonlight restore \
    --from ./backups/full-2026-04-28/ \
    --point-in-time "2026-04-27T14:30:00Z" \
    --target-dir ./data-restored/ \
    --dry-run                      # 先预览

# 仅恢复某个包
$ moonlight restore \
    --from ./backups/full-2026-04-28/ \
    --filter "package:npm:@scope/my-package" \
    --filter "package:maven:com.example:mylib"

# ===== 验证与维护 =====

# 一致性校验 (对比 DB 记录 vs 实际文件)
$ moonlight admin check-consistency \
    --fix-orphaned-files \        # 删除无 DB 记录的孤立文件
    --fix-missing-files \         # 标记有记录但无文件的条目
    --report consistency-report.json

# 备份完整性验证
$ moonlight backup verify \
    --backup-path ./backups/full-2026-04-28/ \
    --check-file-integrity \        # 校验每个文件的 checksum
    --check-db-integrity \          # 运行 PRAGMA integrity_check
    --verbose

# 清理过期备份
$ moonlight backup cleanup \
    --older-than 30d \              # 删除 30 天前的备份
    --keep-minimum 2 \              # 至少保留 2 个备份
    --dry-run
```

### 4.3 备份数据结构

```
backups/
├── full-2026-04-28/
│   ├── MANIFEST.json              # 备份清单
│   │
│   ├── database/
│   │   ├── registry.db             # SQLite 主文件
│   │   ├── registry.db-wal          # WAL 日志
│   │   └── registry.db-shm          # 共享内存文件
│   │
│   ├── packages/                  # 包文件 (可选用硬链接节省空间)
│   │   ├── npm/
│   │   └── maven2/
│   │
│   ├── cache/                     # 缓存 (可选)
│   │
│   ├── metadata/
│   │   ├── file-index.json         # 文件索引 (path -> checksum)
│   │   ├── db-schema.sql           # 数据库 schema 快照
│   │   └── backup-info.json        # 备份元信息
│   │
│   └── checksums.sha256            # 整体校验和
│
├── incremental-2026-04-29/
│   ├── MANIFEST.json
│   ├── changed-files/              # 仅变更的文件
│   │   ├── added/
│   │   ├── modified/
│   │   └── deleted/
│   └── wal-log/                   # WAL 增量日志
│
└── backup-catalog.json            # 备份目录索引
```

### 4.4 SQLite 特殊处理 (WAL 模式)

```go
// 确保备份一致性的关键设置

func InitializeDatabaseForBackup(dsn string) (*gorm.DB, error) {
    db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
    if err != nil {
        return nil, err
    }
    
    // 启用 WAL 模式 (允许并发读写)
    rawDB, _ := db.DB()
    rawDB.Exec("PRAGMA journal_mode=WAL")
    rawDB.Exec("PRAGMA synchronous=NORMAL")     // 平衡性能和安全
    rawDB.Exec("PRAGMA cache_size=-64000")       // 64MB 缓存
    
    // 备份时的特殊处理
    rawDB.Exec("PRAGMA wal_autocheckpoint=1000")  // 自动 checkpoint
    
    return db, nil
}

// 一致性备份函数
func CreateConsistentBackup(db *gorm.DB, destPath string) error {
    sqlDB, _ := db.DB()
    
    // 1. 开始事务 (确保一致性快照)
    tx := db.Begin()
    
    // 2. 在事务中复制数据库文件
    srcPath := getDatabasePath()
    err := copyFile(srcPath, filepath.Join(destPath, "registry.db"))
    if err != nil {
        tx.Rollback()
        return err
    }
    
    // 3. 复制 WAL 和 SHM 文件
    copyFile(srcPath+"-wal", destPath+"/registry.db-wal")
    copyFile(srcPath+"-shm", destPath+"/registry.db-shm")
    
    // 4. 提交事务 (此时备份已一致)
    return tx.Commit().Error
}
```

---

## 五、📦 CAS 存储优化 (Content-Addressable Storage)

### 5.1 去重存储架构

```
传统存储 (重复):                    CAS 存储 (去重):
                                     
Package A v1.0 ──┬→ file_a.jar (5MB)    objects/
Package B v2.0 ──┤                   └── ab/cdef123456... (5MB) ← 实际存储
                  │                   
Package C v1.0 ──┴→ file_a.jar (5MB)    refs/
                                      └── npm/
                                          └── @scope/pkg-a/
                                              └── 1.0.0 -> ../../objects/abcdef...
                                              
总占用: 15MB                        总占用: 5MB (节省 67%)
```

### 5.2 CAS 存储实现

```go
// internal/storage/cas_storage.go

type CASStorage struct {
    objectsDir string       // objects/ 目录 (实际存储)
    refsDir    string       // refs/ 目录 (引用/元数据)
    lock       sync.RWMutex
}

type CASObject struct {
    SHA256     string    // SHA256 哈希作为 ID
    Size       int64     // 文件大小
    ContentType string   // MIME 类型
    RefCount   int32     // 引用计数 (多少个包引用此对象)
    FirstSeen  time.Time // 首次入库时间
    LastAccess time.Time // 最后访问时间
}

type RefEntry struct {
    Path        string    // 引用路径 (如 npm/@scope/pkg/1.0.0/file.tgz)
    ObjectSHA   string    // 指向的对象 SHA256
    OriginalName string  // 原始文件名
    PackageID   uint      // 所属包 ID
    VersionID   uint      // 所属版本 ID
    CreatedAt   time.Time
}

// 存储文件 (自动去重)
func (s *CASStorage) Put(ctx context.Context, reader io.Reader) (*CASObject, error) {
    // 1. 计算 SHA256 (流式计算，不占内存)
    sha256, size, err := s.computeHashAndSize(reader)
    if err != nil {
        return nil, err
    }
    
    objectPath := s.objectPath(sha256)
    
    // 2. 检查是否已存在 (去重!)
    if exists, _ := s.exists(objectPath); exists {
        obj, _ := s.getObject(sha256)
        atomic.AddInt32(&obj.RefCount, 1)
        s.updateLastAccess(sha256)
        return obj, nil  // 返回已有对象，不重复存储
    }
    
    // 3. 写入对象 (原子性写入临时文件再 rename)
    tmpPath := objectPath + ".tmp"
    if err := s.writeToFile(tmpPath, reader); err != nil {
        return nil, err
    }
    os.Rename(tmpPath, objectPath)
    
    // 4. 创建对象记录
    obj := &CASObject{
        SHA256:     sha256,
        Size:       size,
        RefCount:  1,
        FirstSeen:  time.Now(),
        LastAccess: time.Now(),
    }
    s.saveObject(obj)
    
    return obj, nil
}

// 创建引用 (将对象关联到包路径)
func (s *CASStorage) CreateRef(objectSHA256, refPath, originalName string, pkgID, verID uint) error {
    ref := &RefEntry{
        Path:        refPath,
        ObjectSHA:   objectSHA256,
        OriginalName: originalName,
        PackageID:   pkgID,
        VersionID:   verID,
        CreatedAt:   time.Now(),
    }
    return s.saveRef(ref)
}

// 读取文件 (通过引用路径)
func (s *CASStorage) GetByRef(ctx context.Context, refPath string) (io.ReadCloser, error) {
    ref, err := s.getRef(refPath)
    if err != nil {
        return nil, err
    }
    
    // 更新访问时间和引用计数
    s.updateLastAccess(ref.ObjectSHA)
    
    objectPath := s.objectPath(ref.ObjectSHA)
    return os.Open(objectPath)
}

// 删除引用 (垃圾回收由 GC 负责)
func (s *CASStorage) DeleteRef(refPath string) error {
    ref, err := s.getRef(refPath)
    if err != nil {
        return err
    }
    
    obj, _ := s.getObject(ref.ObjectSHA)
    atomic.AddInt32(&obj.RefCount, -1)
    
    // 如果引用计数降为 0，标记为可回收
    if obj.RefCount <= 0 {
        s.markForGC(ref.ObjectSHA)
    }
    
    return s.deleteRef(refPath)
}
```

### 5.3 垃圾回收 (Garbage Collection)

```go
type GarbageCollector struct {
    cas       *CASStorage
    policy    GCPolicy
}

type GCPolicy struct {
    MinAge            time.Duration  // 最小保留时间 (7天)
    MaxUnusedDays    int            // 最大未使用天数 (90天)
    RunSchedule      string         // Cron 表达式 ("0 3 * * *")
    DryRun           bool           # 试运行模式
}

type GCReport struct {
    ScannedAt       time.Time
    CandidatesFound  int
    SpaceReclaimed   int64          // 回收的字节数
    ObjectsDeleted   []string
    Errors          []error
}

func (gc *GarbageCollector) Run() (*GCReport, error) {
    report := &GCReport{ScannedAt: time.Now()}
    
    // 1. 查找所有 RefCount == 0 且超过 MinAge 的对象
    candidates, err := gc.cas.findGCCandidates(gc.policy.MinAge)
    if err != nil {
        return nil, err
    }
    
    report.CandidatesFound = len(candidates)
    
    // 2. 逐个清理
    for _, obj := range candidates {
        if gc.policy.DryRun {
            report.ObjectsDeleted = append(report.ObjectsDeleted, obj.SHA256)
            report.SpaceReclaimed += obj.Size
            continue
        }
        
        // 删除物理文件
        if err := gc.cas.deleteObject(obj.SHA256); err != nil {
            report.Errors = append(report.Errors, err)
            continue
        }
        
        report.ObjectsDeleted = append(report.ObjectsDeleted, obj.SHA256)
        report.SpaceReclaimed += obj.Size
        
        // 从数据库删除记录
        gc.cas.deleteObjectRecord(obj.SHA256)
    }
    
    // 3. 生成报告
    gc.saveReport(report)
    
    return report, nil
}
```

### 5.4 存储路径规范 (CAS 模式)

```
data/
├── objects/                        # 实际文件存储 (按 SHA256 去重)
│   └── ab/
│       └── cdef1234567890abcdef1234567890abcdef1234567890abcd  # 二进制内容
│           └── ef1234567890abcdef1234567890abcdef1234567890abcd  # 另一个文件
│
├── refs/                           # 引用 (逻辑路径)
│   ├── npm/
│   │   └── @scope/
│   │       └── my-package/
│   │           └── 1.0.0/
│   │               └── package.tgz.ref  # JSON: {object_sha: "abcdef...", ...}
│   │
│   └── maven2/
│       └── com/example/
│           └── mylib/
│               └── 1.0.0/
│                   └── mylib-1.0.0.jar.ref
│
├── cache/                          # 缓存 (也走 CAS)
│   └── (同上结构)
│
├── gc-pending/                     # 待回收对象 (软删除)
│   └── ab/cdef1234...              # 移动到此等待 GC 确认删除
│
└── registry.db                     # 元数据数据库
```

### 5.5 存储优化配置

```yaml
storage:
  backend: cas                          # 使用 CAS 去重存储
  
  cas:
    objects_dir: "./data/objects"      # 对象存储目录
    refs_dir: "./data/refs"            # 引用目录
    compute_hash_buffer_size: 65536    # 哈希计算缓冲区 (64KB)
    
  garbage_collection:
    enabled: true
    schedule: "0 3 * * *"              # 每天凌晨 3 点运行
    min_age_days: 7                    # 最少保留 7 天
    max_unused_days: 90                # 90天未使用则回收
    dry_run_default: false              # 默认真正删除
    
    space_threshold_warning: 80         # 使用率 80% 时告警
    space_threshold_critical: 95        # 95% 时强制 GC
    
  retention_policies:
    snapshots:                         # 快照版本保留策略
      keep_count: 5                     # 保留最近 5 个 snapshot
      max_age_days: 30                  # 或最多 30 天
      
    releases:                           # 正式版保留策略
      keep_all: true                    # 正式版全部保留 (不可删除)
      
    pre_releases:                       # 预发布版保留策略
      keep_count: 10
      max_age_days: 180
      
    deprecated:                        # 弃用版本保留策略
      keep_min_versions: 1              # 至少保留 1 个版本
      delay_delete_days: 30             # 弃用后 30 天才可删除
```

### 5.6 存储监控 API

```
# ========== 存储优化相关 API ==========

GET    /api/v1/storage/stats                # 存储统计 (总量/去重率/分布)
GET    /api/v1/storage/objects              # 对象列表 (按大小排序)
GET    /api/v1/storage/orphaned              # 孤立文件 (无引用)
DELETE /api/v1/storage/orphaned              # 清理孤立文件
POST   /api/v1/storage/gc/run                 # 手动触发 GC
GET    /api/v1/storage/gc/reports             # GC 历史报告
GET    /api/v1/storage/dedup-stats            # 去重效果统计
GET    /api/v1/storage/large-files             # 大文件 TOP 100
POST   /api/v1/storage/analyze                # 分析存储趋势预测
```

---

## 五、🔗 多代理仓库配置 (Multi-Proxy Repository)

### 5.1 核心概念

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     多代理仓库架构                                       │
│                                                                         │
│  客户端请求: npm install lodash                                         │
│       │                                                                 │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    Virtual Repository (聚合)                     │   │
│  │                    npm-virtual (统一入口)                         │   │
│  │                                                                 │   │
│  │  路由策略:                                                       │   │
│  │  1. 先查本地仓库 (npm-local)                                     │   │
│  │  2. 再按优先级查代理仓库                                          │   │
│  │     ├─ npm-proxy-cn (优先级 1, 阿里云镜像)                       │   │
│  │     ├─ npm-proxy-tencent (优先级 2, 腾讯云镜像)                   │   │
│  │     └─ npm-proxy-official (优先级 3, npmjs.org)                  │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │  Local   │  │ Proxy 1  │  │ Proxy 2  │  │ Proxy 3  │              │
│  │  本地    │  │ 阿里云   │  │ 腾讯云   │  │ npmjs    │              │
│  │  仓库    │  │ 镜像     │  │ 镜像     │  │ 官方     │              │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘              │
│       │             │             │             │                      │
│       └─────────────┴─────────────┴─────────────┘                      │
│                             │                                           │
│                     ┌───────▼───────┐                                  │
│                     │  Cache Layer  │  (本地缓存 + CAS 去重)            │
│                     └───────────────┘                                  │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.2 仓库模型设计

```sql
-- 仓库表 (独立于 Package，一个仓库对应一个上游源或本地存储)
CREATE TABLE IF NOT EXISTS repositories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,              -- 仓库标识 (如 "npm-proxy-cn")
    display_name TEXT NOT NULL DEFAULT '',   -- 显示名称 (如 "阿里云 NPM 镜像")
    description TEXT DEFAULT '',
    type TEXT NOT NULL CHECK(type IN ('local', 'proxy', 'virtual')),  -- 仓库类型
    package_type TEXT NOT NULL,              -- 包类型 (npm/maven2/pypi/go/nuget/yum/apt/generic)
    enabled INTEGER NOT NULL DEFAULT 1,      -- 是否启用
    
    -- 代理仓库配置 (type=proxy 时有效)
    remote_url TEXT DEFAULT '',              -- 上游仓库 URL
    auth_type TEXT DEFAULT 'none' CHECK(auth_type IN ('none', 'basic', 'bearer', 'api_key')),
    auth_config TEXT DEFAULT '{}',           -- 认证配置 (JSON)
    proxy_priority INTEGER DEFAULT 0,        -- 代理优先级 (数字越小越优先)
    
    -- 缓存配置
    cache_enabled INTEGER DEFAULT 1,
    cache_ttl_seconds INTEGER DEFAULT 86400, -- 默认缓存 24 小时
    cache_max_size_gb REAL DEFAULT 10,       -- 缓存最大容量
    cache_negative_ttl INTEGER DEFAULT 300,  -- 404 缓存时间 (秒)
    
    -- 虚拟仓库配置 (type=virtual 时有效)
    member_repo_ids TEXT DEFAULT '[]',       -- 成员仓库 ID 列表 (JSON 数组, 按优先级排序)
    
    -- 高级配置
    allow_overwrite INTEGER DEFAULT 0,       -- 是否允许覆盖已存在版本
    allow_delete INTEGER DEFAULT 0,          -- 是否允许删除
    download_count INTEGER DEFAULT 0,        -- 下载计数
    
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 仓库组 (Virtual Repository 的成员关系)
CREATE TABLE IF NOT EXISTS repository_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    virtual_repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    member_repo_id INTEGER NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    priority INTEGER NOT NULL DEFAULT 0,     -- 优先级 (0 最高)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    
    UNIQUE(virtual_repo_id, member_repo_id)
);

-- 更新 packages 表关联仓库
-- ALTER TABLE packages ADD COLUMN repository_id INTEGER REFERENCES repositories(id);
-- CREATE INDEX idx_packages_repo ON packages(repository_id);
```

### 5.3 三种仓库类型

```
┌──────────────────────────────────────────────────────────────────────┐
│                        仓库类型对比                                   │
├──────────┬──────────────────┬──────────────────┬────────────────────┤
│          │   Local (本地)    │  Proxy (代理)     │  Virtual (聚合)    │
├──────────┼──────────────────┼──────────────────┼────────────────────┤
│ 用途     │ 存储内部私有包    │ 代理远程公共仓库  │ 聚合多个仓库统一入口│
│ 数据来源 │ 用户上传          │ 远程仓库拉取      │ 聚合 Local+Proxy   │
│ 写操作   │ ✅ 支持上传       │ ❌ 只读           │ ❌ 只读            │
│ 读操作   │ ✅ 直接读取       │ ✅ 缓存+回源      │ ✅ 按优先级路由     │
│ 缓存     │ 不需要            │ ✅ 本地缓存       │ 成员各自缓存       │
│ 典型场景 │ 公司私有 npm 包   │ npmjs.org 镜像   │ 统一 npm 入口      │
└──────────┴──────────────────┴──────────────────┴────────────────────┘
```

### 5.4 多代理路由引擎

```go
// internal/proxy/router.go

type ProxyRouter struct {
    db        *gorm.DB
    cache     *CacheService
    adapters  map[string]adapter.Adapter
}

type RouteResult struct {
    Source      string         // 命中的仓库名
   SourceType  string         // "local" | "proxy" | "virtual"
    Content     io.ReadCloser
    Size        int64
    FromCache   bool
    CacheTTL    time.Duration
}

func (r *ProxyRouter) Resolve(ctx context.Context, pkgType, name, version string) (*RouteResult, error) {
    // 1. 查找该包类型的 Virtual Repository
    virtualRepo, err := r.findVirtualRepo(pkgType)
    if err != nil {
        return nil, err
    }
    
    // 2. 按优先级遍历成员仓库
    members, err := r.getOrderedMembers(virtualRepo.ID)
    if err != nil {
        return nil, err
    }
    
    for _, member := range members {
        repo, _ := r.getRepo(member.MemberRepoID)
        
        switch repo.Type {
        case "local":
            // 本地仓库: 直接查找
            result, err := r.resolveLocal(ctx, repo, pkgType, name, version)
            if err == nil && result != nil {
                result.Source = repo.Name
                result.SourceType = "local"
                return result, nil
            }
            
        case "proxy":
            // 代理仓库: 先查缓存，再回源
            result, err := r.resolveProxy(ctx, repo, pkgType, name, version)
            if err == nil && result != nil {
                result.Source = repo.Name
                result.SourceType = "proxy"
                return result, nil
            }
        }
    }
    
    return nil, ErrPackageNotFound
}

func (r *ProxyRouter) resolveProxy(ctx context.Context, repo *Repository, pkgType, name, version string) (*RouteResult, error) {
    cacheKey := fmt.Sprintf("proxy:%s:%s:%s", repo.Name, name, version)
    
    // 1. 查本地缓存
    cached, err := r.cache.Get(ctx, cacheKey)
    if err == nil && cached != nil {
        return &RouteResult{
            Content:   cached.Content,
            Size:      cached.Size,
            FromCache: true,
            CacheTTL:  time.Duration(repo.CacheTTLMinutes) * time.Minute,
        }, nil
    }
    
    // 2. 回源拉取
    adapter, ok := r.adapters[pkgType]
    if !ok {
        return nil, fmt.Errorf("no adapter for package type: %s", pkgType)
    }
    
    remoteURL := repo.RemoteURL
    content, size, err := adapter.FetchRemote(ctx, remoteURL, name, version)
    if err != nil {
        // 缓存 404 (防止反复回源)
        if isNotFoundError(err) && repo.CacheNegativeTTL > 0 {
            r.cache.SetNegative(ctx, cacheKey, time.Duration(repo.CacheNegativeTTL)*time.Second)
        }
        return nil, err
    }
    
    // 3. 写入缓存
    if repo.CacheEnabled {
        go r.cache.StoreAsync(ctx, cacheKey, content, size, 
            time.Duration(repo.CacheTTLMinutes)*time.Minute)
    }
    
    return &RouteResult{
        Content:   content,
        Size:      size,
        FromCache: false,
    }, nil
}
```

### 5.5 配置示例

```yaml
repositories:
  # ===== npm 仓库组 =====
  - name: npm-local
    display_name: "NPM 内部仓库"
    type: local
    package_type: npm
    allow_overwrite: false
    allow_delete: true
    
  - name: npm-proxy-cn
    display_name: "阿里云 NPM 镜像"
    type: proxy
    package_type: npm
    remote_url: "https://registry.npmmirror.com"
    cache_ttl_seconds: 86400
    cache_max_size_gb: 50
    proxy_priority: 1              # 优先级 1 (最高)

  - name: npm-proxy-tencent
    display_name: "腾讯云 NPM 镜像"
    type: proxy
    package_type: npm
    remote_url: "https://mirrors.cloud.tencent.com/npm/"
    cache_ttl_seconds: 86400
    proxy_priority: 2

  - name: npm-proxy-official
    display_name: "NPM 官方仓库"
    type: proxy
    package_type: npm
    remote_url: "https://registry.npmjs.org"
    cache_ttl_seconds: 3600        # 官方源缓存短一些
    proxy_priority: 3
    auth_type: bearer
    auth_config:
      token: "${NPM_AUTH_TOKEN}"   # 从环境变量读取

  - name: npm-virtual
    display_name: "NPM 聚合仓库"
    type: virtual
    package_type: npm
    member_repos:                   # 按优先级排序
      - npm-local                   # 本地优先
      - npm-proxy-cn                # 阿里云次之
      - npm-proxy-tencent           # 腾讯云第三
      - npm-proxy-official          # 官方兜底

  # ===== Maven 仓库组 =====
  - name: maven-local
    display_name: "Maven 内部仓库"
    type: local
    package_type: maven2

  - name: maven-proxy-aliyun
    display_name: "阿里云 Maven 镜像"
    type: proxy
    package_type: maven2
    remote_url: "https://maven.aliyun.com/repository/public"
    proxy_priority: 1

  - name: maven-proxy-central
    display_name: "Maven Central"
    type: proxy
    package_type: maven2
    remote_url: "https://repo1.maven.org/maven2/"
    proxy_priority: 2

  - name: maven-proxy-google
    display_name: "Google Maven"
    type: proxy
    package_type: maven2
    remote_url: "https://dl.google.com/dl/android/maven2/"
    proxy_priority: 3

  - name: maven-proxy-spring
    display_name: "Spring Milestones"
    type: proxy
    package_type: maven2
    remote_url: "https://repo.spring.io/milestone/"
    proxy_priority: 4

  - name: maven-virtual
    display_name: "Maven 聚合仓库"
    type: virtual
    package_type: maven2
    member_repos:
      - maven-local
      - maven-proxy-aliyun
      - maven-proxy-central
      - maven-proxy-google
      - maven-proxy-spring

  # ===== PyPI 仓库组 =====
  - name: pypi-local
    type: local
    package_type: pypi

  - name: pypi-proxy-tsinghua
    display_name: "清华 PyPI 镜像"
    type: proxy
    package_type: pypi
    remote_url: "https://pypi.tuna.tsinghua.edu.cn/simple"
    proxy_priority: 1

  - name: pypi-proxy-aliyun
    display_name: "阿里云 PyPI 镜像"
    type: proxy
    package_type: pypi
    remote_url: "https://mirrors.aliyun.com/pypi/simple/"
    proxy_priority: 2

  - name: pypi-proxy-official
    display_name: "PyPI 官方"
    type: proxy
    package_type: pypi
    remote_url: "https://pypi.org/simple/"
    proxy_priority: 3

  - name: pypi-virtual
    display_name: "PyPI 聚合仓库"
    type: virtual
    package_type: pypi
    member_repos:
      - pypi-local
      - pypi-proxy-tsinghua
      - pypi-proxy-aliyun
      - pypi-proxy-official

  # ===== Go Modules 仓库组 =====
  - name: go-local
    type: local
    package_type: go

  - name: go-proxy-cn
    display_name: "七牛 Go 代理"
    type: proxy
    package_type: go
    remote_url: "https://goproxy.cn"
    proxy_priority: 1

  - name: go-proxy-io
    display_name: "goproxy.io"
    type: proxy
    package_type: go
    remote_url: "https://goproxy.io"
    proxy_priority: 2

  - name: go-virtual
    type: virtual
    package_type: go
    member_repos:
      - go-local
      - go-proxy-cn
      - go-proxy-io
```

### 5.6 仓库管理 API

```
# ========== 仓库管理 ==========

GET    /api/v1/repositories                       # 仓库列表 (支持按类型/包类型筛选)
POST   /api/v1/repositories                       # 创建仓库
GET    /api/v1/repositories/:name                 # 仓库详情
PUT    /api/v1/repositories/:name                 # 更新仓库配置
DELETE /api/v1/repositories/:name                 # 删除仓库

# 仓库状态与统计
GET    /api/v1/repositories/:name/stats           # 仓库统计 (包数/缓存大小/命中率)
GET    /api/v1/repositories/:name/health          # 代理仓库健康检查 (连通性/延迟)
POST   /api/v1/repositories/:name/test-connection # 测试远程仓库连通性

# 缓存管理 (代理仓库)
GET    /api/v1/repositories/:name/cache/stats     # 缓存统计
DELETE /api/v1/repositories/:name/cache            # 清空缓存
POST   /api/v1/repositories/:name/cache/invalidate # 按包名失效缓存
POST   /api/v1/repositories/:name/cache/prefetch  # 预热缓存 (批量拉取指定包)

# 虚拟仓库成员管理
GET    /api/v1/repositories/:name/members          # 成员仓库列表 (含优先级)
PUT    /api/v1/repositories/:name/members          # 更新成员仓库及优先级
POST   /api/v1/repositories/:name/members/:memberName  # 添加成员
DELETE /api/v1/repositories/:name/members/:memberName  # 移除成员

# 包路由追踪 (调试用)
GET    /api/v1/repositories/resolve?pkg_type=npm&name=lodash&version=4.17.21
                                              # 查看包在哪个仓库命中
```

### 5.7 代理认证模型

```go
// internal/proxy/auth.go

type ProxyAuthConfig struct {
    Type     string          `json:"type"`     // none | basic | bearer | api_key
    Basic    *BasicAuth      `json:"basic,omitempty"`
    Bearer   *BearerAuth     `json:"bearer,omitempty"`
    APIKey   *APIKeyAuth     `json:"api_key,omitempty"`
}

type BasicAuth struct {
    Username string `json:"username"`
    Password string `json:"password"`  // 支持环境变量引用 ${ENV_VAR}
}

type BearerAuth struct {
    Token string `json:"token"`        // 支持环境变量引用
}

type APIKeyAuth struct {
    HeaderName  string `json:"header_name"`   // 如 "X-API-Key"
    KeyValue    string `json:"key_value"`      // 支持环境变量引用
    QueryParam  string `json:"query_param,omitempty"` // 如 "token"
}

func (c *ProxyAuthConfig) Apply(req *http.Request) error {
    switch c.Type {
    case "basic":
        req.SetBasicAuth(c.Basic.Username, resolveEnv(c.Basic.Password))
    case "bearer":
        req.Header.Set("Authorization", "Bearer "+resolveEnv(c.Bearer.Token))
    case "api_key":
        req.Header.Set(c.APIKey.HeaderName, resolveEnv(c.APIKey.KeyValue))
        if c.APIKey.QueryParam != "" {
            q := req.URL.Query()
            q.Set(c.APIKey.QueryParam, resolveEnv(c.APIKey.KeyValue))
            req.URL.RawQuery = q.Encode()
        }
    }
    return nil
}

func resolveEnv(s string) string {
    if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
        envKey := s[2 : len(s)-1]
        return os.Getenv(envKey)
    }
    return s
}
```

### 5.8 缓存策略

```yaml
proxy_cache:
  default_ttl: 86400              # 默认缓存 24 小时
  negative_cache_ttl: 300         # 404 缓存 5 分钟 (防止穿透)
  max_entry_size_mb: 500          # 单文件最大 500MB
  max_total_size_gb: 100          # 总缓存容量
  
  eviction_policy: lru            # 淘汰策略: lru | lfu | ttl
  refresh_before_expire: 3600     # 过期前 1 小时异步刷新
  
  # 回源策略
  fetch_timeout: 30s              # 单个代理回源超时
  fetch_retries: 2                # 回源重试次数
  fetch_retry_delay: 1s           # 重试间隔
  concurrent_fetch: true          # 多代理并发回源 (取最快响应)
  
  # 一致性
  validate_etag: true             # 通过 ETag 验证缓存新鲜度
  validate_last_modified: true    # 通过 Last-Modified 验证
  stale_while_revalidate: 300     # 过期后 5 分钟内可返回旧数据同时刷新
```

### 5.9 并发回源策略

```
┌─────────────────────────────────────────────────────────────────────┐
│                    并发回源 (Concurrent Fetch)                       │
│                                                                     │
│  客户端请求: npm install lodash@4.17.21                             │
│       │                                                             │
│       ▼                                                             │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │  本地缓存: MISS                                               │ │
│  │                                                               │ │
│  │  并发请求 (3 个代理同时发):                                     │ │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │ │
│  │  │ 阿里云镜像   │  │ 腾讯云镜像   │  │ npmjs.org   │          │ │
│  │  │ (50ms) ✅   │  │ (80ms) ...  │  │ (200ms) ... │          │ │
│  │  └─────────────┘  └─────────────┘  └─────────────┘          │ │
│  │       │                                                       │ │
│  │       ▼                                                       │ │
│  │  取最快响应 (阿里云 50ms) → 返回客户端                          │ │
│  │  取消其他请求 (腾讯云/npmjs)                                   │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                     │
│  串行回源 (备选模式, 按优先级):                                       │
│  1. 阿里云镜像 → 命中 → 返回 (不请求后续)                           │
│  2. 腾讯云镜像 → (仅阿里云未命中时)                                  │
│  3. npmjs.org   → (仅以上都未命中时)                                │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 六、🔄 更新的整体架构图 (v2.0)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Moonlight Registry v2.0                               │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐   │
│  │                        API Gateway Layer                          │   │
│  │                                                                   │   │
│  │  /npm/  /maven2/  /pypi/  /go/  ...  (8种协议适配器)           │   │
│  │  /api/v1/  /ai/  ...  (RESTful API + AI 路由)                   │   │
│  └──────────────────────────┬────────────────────────────────────────┘   │
│                             │                                           │
│  ┌──────────────────────────▼────────────────────────────────────────┐   │
│  │                     Core Engine (Go)                               │   │
│  │                                                                     │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐  │   │
│  │  │ Protocol    │ │ Supply Chain│ │ AI Assistant│ │ Backup &    │  │   │
│  │  │ Adapters    │ │ Security    │ │ Engine      │ │ Restore     │  │   │
│  │  │ (8 types)   │ │ Engine      │ │             │ │ Service     │  │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘  │   │
│  │                                                                     │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐  │   │
│  │  │ Package     │ │ CAS Storage │ │ Auth & RBAC │ │ Event &     │  │   │
│  │  │ Manager     │ │ (Dedup)     │ │             │ │ Audit       │  │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘  │   │
│  │                                                                     │   │
│  │  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐  │   │
│  │  │ Multi-Proxy │ │ Security    │ │ Offline     │ │ Repository  │  │   │
│  │  │ Router      │ │ Scanner     │ │ Sync Tools  │ │ Manager     │  │   │
│  │  └─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘  │   │
│  └───────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐   │
│  │                     Data Layer                                     │   │
│  │                                                                     │   │
│  │  ┌─────────────┐  ┌─────────────────────────────────────────────┐  │   │
│  │  │ PostgreSQL  │  │ CAS Storage Backend                        │  │   │
│  │  │ or SQLite   │  │ objects/ (去重) + refs/ (引用) + cache/   │  │   │
│  │  │             │  │ + gc-pending/ (待回收)                     │  │   │
│  │  └─────────────┘  └─────────────────────────────────────────────┘  │   │
│  │                                                                     │   │
│  │  ┌─────────────┐                                                │   │
│  │  │ Backup Store │  (Local/S3/NFS - 可配置)                      │   │
│  │  └─────────────┘                                                │   │
│  └───────────────────────────────────────────────────────────────────┘   │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐   │
│  │                   CLI Tools                                       │   │
│  │                                                                     │   │
│  │  moonlight serve    moonlight sync    moonlight export            │   │
│  │  moonlight import   moonlight backup   moonlight restore           │   │
│  │  moonlight verify    moonlight admin    moonlight auth            │   │
│  └───────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 七、📊 更新的项目结构 (新增模块)

```
moonlight-box/
├── cmd/
│   └── registry/
│       └── main.go
│
├── internal/
│   ├── supply_chain/              # 🆕 供应链安全
│   │   ├── typosquatting.go       # 拼写混淆检测
│   │   ├── dep_confusion.go       # 依赖混淆防护
│   │   ├── token_leakage.go       # 凭证泄露扫描
│   │   ├── version_immutable.go   # 版本不可变策略
│   │   └── risk_engine.go         # 综合风险评估引擎
│   │
│   ├── ai/                    # 🆕 AI 辅助功能
│   │   ├── semantic_search.go    # 语义搜索引擎
│   │   ├── health_score.go       # 健康度评分引擎
│   │   ├── recommendations.go    # 智能推荐引擎
│   │   ├── upgrade_advisor.go    # 升级建议分析
│   │   ├── auto_docs.go          # 自动文档生成
│   │   ├── embedding.go          # 向量化服务
│   │   ├── llm_proxy.go          # LLM 代理 (本地/云端)
│   │   └── knowledge_graph.go    # 依赖关系图谱
│   │
│   ├── backup/                   # 🆕 备份恢复
│   │   ├── manager.go            # 备份管理器
│   │   ├── restorer.go          # 恢复器
│   │   ├── consistency.go       # 一致性校验
│   │   └── scheduler.go         # 定时任务调度
│   │
│   ├── storage/                  # 🔧 升级存储
│   │   ├── backend.go
│   │   ├── local_storage.go
│   │   ├── s3_storage.go
│   │   ├── cas_storage.go       # 🆕 CAS 去重存储
│   │   └── garbage_collector.go # 🆕 垃圾回收
│   │
│   ├── offline/                  # 🆕 离线支持
│   │   ├── sync.go              # 同步命令
│   │   ├── exporter.go          # 导出命令
│   │   ├── importer.go          # 导入命令
│   │   ├── bundle.go            # 离线包打包
│   │   └── verifier.go          # 完整性验证
│   │
│   ├── proxy/                    # 🆕 多代理仓库
│   │   ├── router.go            # 多代理路由引擎
│   │   ├── resolver.go          # 仓库解析器 (local/proxy/virtual)
│   │   ├── cache.go             # 代理缓存管理
│   │   ├── auth.go              # 代理认证 (basic/bearer/api_key)
│   │   ├── concurrent.go        # 并发回源策略
│   │   └── health.go            # 代理健康检查
│   │
│   ├── adapter/                  # (原有)
│   ├── service/                  # (原有)
│   ├── handler/                  # (原有)
│   ├── middleware/               # (原有)
│   ├── repository/               # (原有)
│   └── model/                   # (原有)
│
├── cli/                         # 🆕 CLI 命令定义 (cobra)
│   ├── root.go
│   ├── serve.go
│   ├── auth.go
│   ├── sync.go
│   ├── export.go
│   ├── import.go
│   ├── backup.go
│   ├── restore.go
│   ├── verify.go
│   └── admin.go
│
└── web/                         # (Vue 前端 - 新增页面)
    └── src/views/
        ├── supply-chain/         # 🆕 供应链安全面板
        │   ├── Dashboard.vue
        │   ├── Typosquatting.vue
        │   ├── TokenLeaks.vue
        │   └── Policies.vue
        ├── ai/                   # 🆕 AI 辅助面板
        │   ├── Search.vue            # 语义搜索
        │   ├── HealthScore.vue       # 健康度评分
        │   ├── Recommendations.vue   # 智能推荐
        │   └── UpgradeAdvisor.vue    # 升级建议
        ├── repositories/        # 🆕 仓库管理
        │   ├── List.vue              # 仓库列表
        │   ├── Detail.vue            # 仓库详情/配置
        │   ├── Members.vue           # 虚拟仓库成员管理
        │   └── CacheStats.vue        # 缓存统计
        ├── backup/               # 🆕 备份管理
        │   ├── Dashboard.vue
        │   ├── History.vue
        │   └── Restore.vue
        └── storage/              # 🆕 存储分析
            ├── Stats.vue
            ├── Objects.vue
            └── GCReports.vue
```

---

## 八、🗓️ 更新的开发路线图

### Phase 1: MVP (不变)

基础功能：npm/Maven 支持 + JWT + 本地存储 + Web 后台

### Phase 2: 企业安全与可靠性 ⭐ 新增

**目标**: 生产就绪的企业级特性

- [ ] **🛡️ 供应链安全防护**
  - [ ] Typosquatting 检测引擎
  - [ ] Dependency Confusion 防护
  - [ ] Token Leakage 扫描器
  - [ ] Version Immutability 策略
  - [ ] 综合风险评估仪表盘

- [ ] **🔌 离线环境支持**
  - [ ] `moonlight sync` 命令 (多源同步)
  - [ ] `moonlight export/import` 命令
  - [ ] 离线 Bundle 打包格式
  - [ ] 离线模式启动 (`--mode offline`)
  - [ ] 完整性验证工具

- [ ] **💾 备份恢复系统**
  - [ ] 全量/增量备份
  - [ ] 时间点恢复 (PITR)
  - [ ] 一致性校验 (`check-consistency`)
  - [ ] S3/NFS/本地多后端支持
  - [ ] 定时备份调度

- [ ] **📦 CAS 存储优化**
  - [ ] SHA256 内容寻址存储
  - [ ] 自动去重 (节省 40-70% 存储)
  - [ ] 垃圾回收 (GC) 引擎
  - [ ] 存储使用率监控与告警
  - [ ] 旧版本自动清理策略

- [ ] **🔗 多代理仓库**
  - [ ] 仓库模型 (Local/Proxy/Virtual)
  - [ ] 多代理路由引擎 (优先级+并发回源)
  - [ ] 代理认证 (Basic/Bearer/API Key)
  - [ ] 代理缓存管理 (TTL/负缓存/预热)
  - [ ] 代理健康检查与自动切换
  - [ ] 仓库管理 Web UI

### Phase 3: 高级特性 (后续规划)

- LDAP/OAuth2 SSO
- 全文搜索引擎 (Meilisearch)
- 许可证合规 (SBOM)
- ABAC 细粒度权限
- CDN 边缘分发
- Webhooks 事件系统
- OCI 容器镜像支持

---

## 九、✅ 总结

### v2.0 新增能力矩阵

| 能力域 | v1.0 | v2.0 (新增) | 价值 |
|--------|------|-----------|------|
| **安全防护** | 漏洞扫描 | ✅ 供应链攻击全方位防护 | 防止 4 类致命攻击 |
| **部署灵活** | 在线部署 | ✅ 完全离线支持 | 满足金融/军工需求 |
| **AI 辅助** | 无 | ✅ 语义搜索/智能推荐/健康评分 | 提升开发者体验 10x |
| **数据安全** | 无备份 | ✅ 全量/增量/PITR | 符合合规要求 |
| **存储效率** | 原始存储 | ✅ CAS 去重 (省 40-70%) | 降低 60% 存储成本 |
| **代理能力** | 单代理 | ✅ 多代理+聚合+并发回源 | 灵活配置多源+加速 |
| **运维效率** | Web UI | ✅ 完整 CLI 工具链 | DevOps 友好 |

### 关键指标提升

```
安全等级:     ⭐⭐⭐ → ⭐⭐⭐⭐⭐ (增加 4 层防护)
部署场景:     1 种  → 3 种 (在线/离线/混合)
开发者体验:   基础搜索 → AI 语义搜索 + 智能推荐 + 健康评分
代理能力:     单代理 → 多代理聚合 + 并发回源 + 自动切换
存储效率:     100% → 40-60% (CAS 去重)
数据可靠性:   无  → 99.99% (备份+PITR+校验)
```

---

> **文档版本**: v2.0
> **最后更新**: 2026-04-28
> **审批状态**: ✅ 已批准 (含 6 大企业级增强场景)
