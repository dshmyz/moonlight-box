# API 与管理后台功能分类重组设计

## 一、背景

当前项目的 API 模块和管理后台导航存在以下问题：

1. **API 层混乱**：`repository.ts` 包含 3 个不相关 API；`auth.ts` 混合了登录和 CAS 回调；`SecurityCenter.vue` 直接裸调 `request` 而无 API 封装
2. **导航扁平**：15 个菜单项无分组，概念重叠（系统设置 vs 系统配置）
3. **路径不一致**：`file.ts` 使用绝对路径，其他文件使用相对路径
4. **复用缺失**：安全扫描 API 未封装，组件内直接调用 `request`

## 二、设计目标

- 侧边栏按业务域分组折叠，提升导航效率
- 每个 API 文件职责单一，按业务域划分
- 统一 API 调用风格，消除裸调 `request`
- 最小化变更范围，不修改后端路由

## 三、侧边栏导航重组

### 3.1 分组结构

```
📊 仪表盘                    （顶层，始终可见）

📦 制品管理                  （折叠组）
   ├── 包管理
   └── 仓库管理

🛡️ 安全合规                  （折叠组）
   ├── 安全中心
   └── 阻断规则

💾 存储与缓存                （折叠组）
   ├── 存储管理
   ├── 缓存管理
   └── 文件浏览

⚙️ 系统管理                  （折叠组）
   ├── 用户管理
   ├── 审计日志
   ├── 系统配置
   ├── 系统信息
   ├── 备份管理
   ├── Webhook 管理
   └── CAS 设置
```

### 3.2 关键变更

- 原"系统设置"页面（只有 CAS 配置）重命名为"CAS 设置"，路由 `/admin/settings` → `/admin/cas-settings`
- 侧边栏使用 `el-sub-menu` 实现折叠分组
- 仪表盘作为顶层菜单项，不归入任何分组

## 四、前端 API 模块重组

### 4.1 重组后文件清单

```
web/src/api/
├── request.ts          # 不变 — Axios 实例
├── auth.ts             # 精简 — 登录/登出/刷新/资料
├── casConfig.ts        # 扩展 — CAS 配置管理 + CAS 认证回调
├── package.ts          # 不变 — 包搜索/版本管理
├── repository.ts       # 精简 — 仓库 CRUD + 成员管理
├── cache.ts            # 新建 — 缓存统计/清理/失效
├── public.ts           # 新建 — 公开仓库配置
├── security.ts         # 新建 — 安全扫描/漏洞/CVE 阻断
├── storageBackend.ts   # 不变 — 存储后端管理
├── file.ts             # 修正 — 路径风格统一
├── system.ts           # 不变 — 系统配置 + 系统信息
├── backup.ts           # 不变 — 备份管理
├── webhook.ts          # 不变 — Webhook 管理
└── blockRule.ts        # 不变 — 阻断规则
```

### 4.2 各文件详细内容

#### `auth.ts`（精简）

```ts
export const authApi = {
  login(username: string, password: string)    // POST /auth/login
  logout()                                      // POST /auth/logout
  refreshToken()                                // POST /auth/refresh
  getProfile()                                  // GET /auth/profile
}
```

移除：`getCASConfig()`、`casCallback()` → 移入 `casConfig.ts`

#### `casConfig.ts`（扩展）

```ts
export const casConfigApi = {
  getConfig()                                   // GET /cas/config
  updateConfig(config: CASConfig)               // PUT /cas/config
  deleteConfig()                                // DELETE /cas/config
}

export const casAuthApi = {
  getCASConfig()                                // GET /auth/cas/config
  casCallback(ticket: string)                   // GET /auth/cas/callback
}
```

新增：从 `auth.ts` 移入 CAS 认证回调

#### `cache.ts`（新建）

```ts
export const cacheApi = {
  getStats()                                    // GET /cache/stats
  clear()                                       // DELETE /cache
  invalidate(data: { pattern: string })         // POST /cache/invalidate
}
```

来源：从 `repository.ts` 中移出

#### `public.ts`（新建）

```ts
export const publicRepoApi = {
  getRepoConfig(name: string)                   // GET /public/repo/:name
}
```

来源：从 `repository.ts` 中移出

#### `security.ts`（新建）

```ts
export interface Vulnerability {
  cve_id: string
  severity: string
  cvss_score: number
  title: string
  dependency_name: string
  current_version: string
  fixed_version?: string
  references: string
}

export interface SecurityStats {
  total_scans: number
  critical: number
  high: number
  medium: number
  low: number
}

export interface ScanResult {
  id: number
  version_id: number
  scan_status: string
  total_vulnerabilities: number
  critical_count: number
  high_count: number
  medium_count: number
  low_count: number
  scanned_at: string
}

export interface SecurityDashboard {
  recent_vulnerabilities: ScanResult[]
}

export const securityApi = {
  getStatistics()                                                    // GET /security/statistics
  listVulnerabilities(params?: { page?: number; page_size?: number; severity?: string; pkg_type?: string })  // GET /security/vulnerabilities
  getDashboard()                                                     // GET /security/dashboard
  getScanResult(packageId: number)                                   // GET /security/packages/:id/scan
  triggerFullScan()                                                  // POST /security/scan/full
  triggerScan(packageId: number)                                     // POST /security/packages/:id/scan/trigger
  blockByCVE(cve: string)                                           // POST /security/block/:cve
}
```

来源：从 `SecurityCenter.vue` 中裸调 `request` 的逻辑抽离

#### `repository.ts`（精简）

```ts
export const repositoryApi = {
  list(params?)                                 // GET /repositories
  get(name: string)                             // GET /repositories/:name
  create(data)                                  // POST /repositories
  update(name: string, data)                    // PUT /repositories/:name
  delete(name: string)                          // DELETE /repositories/:name
  getMembers(name: string)                      // GET /repositories/:name/members
  addMember(name: string, data)                 // POST /repositories/:name/members
  removeMember(name: string, memberName: string) // DELETE /repositories/:name/members/:memberName
}
```

移除：`publicRepoApi` → 移入 `public.ts`，`cacheApi` → 移入 `cache.ts`

#### `file.ts`（修正路径）

```ts
export const fileApi = {
  browse(path: string = '/')                    // GET /files/browse（修正为相对路径）
  stats(path: string)                           // GET /files/stats
  download(path: string)                        // GET /files/download
}
```

修正：将 `/api/v1/files/browse` 改为 `/files/browse`（baseURL 已是 `/api/v1`）

## 五、路由和视图层调整

### 5.1 路由变更

| 变更类型 | 详情 |
|----------|------|
| 重命名路由 | `/admin/settings` → `/admin/cas-settings` |
| 重命名视图 | `Settings.vue` → `CASSettings.vue` |

### 5.2 视图 import 变更

| 视图文件 | 变更 |
|----------|------|
| `SecurityCenter.vue` | 裸调 `request` → 使用 `securityApi` |
| `CacheManagement.vue` | `cacheApi` 从 `@/api/repository` → `@/api/cache` |
| `BrowsePage.vue` | `publicRepoApi` 从 `@/api/repository` → `@/api/public` |
| `AppSidebar.vue` | 重构为分组折叠结构 |

### 5.3 不变更的部分

- 后端 API 路由不变
- `Dashboard.vue`、`PackageList.vue`、`PackageDetail.vue`、`RepositoryList.vue`、`BlockRuleList.vue`、`StorageManagement.vue`、`UserManagement.vue`、`AuditLogs.vue`、`BackupManagement.vue`、`WebhookManagement.vue`、`SystemConfig.vue`、`SystemInfo.vue` 等视图的业务逻辑不变

## 六、实施步骤

1. 新建 `cache.ts`、`public.ts`、`security.ts` 三个 API 文件
2. 精简 `repository.ts`（移出 `cacheApi` 和 `publicRepoApi`）
3. 精简 `auth.ts`（移出 CAS 回调），扩展 `casConfig.ts`
4. 修正 `file.ts` 路径风格
5. 更新 `SecurityCenter.vue` 使用 `securityApi`
6. 更新 `CacheManagement.vue` 的 `cacheApi` import
7. 更新 `BrowsePage.vue` 的 `publicRepoApi` import
8. 重命名 `Settings.vue` → `CASSettings.vue`，更新路由
9. 重构 `AppSidebar.vue` 为分组折叠结构
10. 更新 `router/index.ts` 路由配置
11. 验证所有页面功能正常
