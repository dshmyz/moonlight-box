# 版本阻断功能设计

## 概述

管理端支持配置阻断规则，当某个包的特定版本被阻断时，所有下载请求（本地仓和代理仓）都会被拦截，返回 403 和详细的阻断原因。同时记录阻断日志，包含请求方 IP 和时间。

## 架构

```
客户端请求 → [BlockCheck 中间件] → 适配器路由 → 下载处理
                  ↓
           查询阻断规则表
                  ↓
           命中规则 → 记录审计日志 → 返回 403
           未命中 → 放行
```

## 后端设计

### 数据模型

**BlockRule 表**

| 字段 | 类型 | 说明 |
|------|------|------|
| ID | uint | 主键 |
| PackageName | string(200) | 包名（如 `lodash`） |
| Version | string(100) | 版本号（如 `4.17.20` 或 `4.*`） |
| MatchType | string(20) | `exact`（精确匹配）或 `wildcard`（通配符） |
| PackageType | string(20) | 包类型（`npm` / `maven`） |
| Reason | string(500) | 阻断原因说明 |
| Enabled | bool | 是否启用，默认 true |
| CreatedBy | *uint | 创建人 |
| CreatedAt | time | 创建时间 |
| UpdatedAt | time | 更新时间 |

**匹配逻辑**

- 精确匹配：`packageName == rule.PackageName && version == rule.Version`
- 通配符匹配：将 `*` 转为正则（如 `4.*` → `^4\..*$`），包名也支持通配符
- 查询时先查精确匹配规则，再查通配符规则，命中即返回

### 阻断检查中间件

- 挂载在适配器路由组（`/npm/*`, `/maven2/*`）
- 从 URL 解析包名和版本号
- 调用 `BlockRuleService.IsBlocked(packageType, packageName, version)`
- 命中时：
  1. 写入 `AuditLog`（action=block, resource_name=包名@版本, ip_address, details=JSON{rule_id, reason}）
  2. 返回 `403 {"code": 403, "message": "包 lodash@4.17.20 已被阻断: 安全漏洞"}`

### API 端点

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/api/v1/block-rules` | 需要 | 列出阻断规则（支持分页和搜索） |
| POST | `/api/v1/block-rules` | 需要 | 创建阻断规则 |
| PUT | `/api/v1/block-rules/:id` | 需要 | 更新阻断规则 |
| DELETE | `/api/v1/block-rules/:id` | 需要 | 删除阻断规则 |

### 审计日志

复用现有 `AuditLog` 模型，新增 `ActionBlock AuditAction = "block"` 常量。

阻断时记录：
- `action`: "block"
- `resource_type`: "package"
- `resource_name`: "lodash@4.17.20"
- `ip_address`: 客户端 IP
- `details`: JSON `{"rule_id": 1, "reason": "安全漏洞", "match_type": "exact"}`

## 前端设计

### 新增页面：阻断规则管理

路径：`/admin/block-rules`

**规则列表 Tab**
- 表格列：包名、版本、匹配类型、包类型、原因、启用状态、创建时间、操作
- 操作：编辑、删除、启用/禁用切换
- 搜索：按包名过滤
- 创建按钮：打开创建对话框

**创建/编辑对话框**
- 表单字段：包名、版本、匹配类型（下拉选择）、包类型（下拉选择）、原因（文本域）、启用开关
- 匹配类型选择"通配符"时，版本字段显示提示："支持 * 通配符，如 4.* 匹配所有 4.x 版本"

**阻断日志 Tab**
- 表格列：时间、包名@版本、匹配规则、客户端 IP、阻断原因
- 按时间倒序排列
- 支持按包名搜索

### 侧边栏

新增菜单项：「阻断规则」（Shield 图标），位于「缓存管理」之前

### 路由

新增 `/admin/block-rules` 路由，需要认证

## 文件变更清单

### 后端新增文件
- `internal/model/block_rule.go` - BlockRule 模型
- `internal/repository/block_rule_repo.go` - 数据访问层
- `internal/service/block_rule_service.go` - 业务逻辑层
- `internal/handler/block_rule_handler.go` - HTTP 处理器
- `internal/middleware/block.go` - 阻断检查中间件

### 后端修改文件
- `internal/model/audit.go` - 新增 ActionBlock 常量
- `internal/database/migration.go` - 新增 BlockRule 表自动迁移
- `cmd/registry/main.go` - 注册路由和中间件

### 前端新增文件
- `src/api/blockRule.ts` - API 请求层
- `src/views/BlockRuleList.vue` - 阻断规则管理页面
- `src/components/block-rule/BlockRuleForm.vue` - 创建/编辑表单组件
- `src/components/block-rule/BlockLogTable.vue` - 阻断日志表格组件

### 前端修改文件
- `src/router/index.ts` - 新增路由
- `src/components/layout/AppSidebar.vue` - 新增菜单项
