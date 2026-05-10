# 删除权限控制 + 审计日志设计文档

## 概述

为包删除功能添加基于 RBAC 的权限控制和操作审计日志，确保删除操作可追溯、可管控。

## 权限模型

### 权限定义

| Resource | Action | 说明 |
|----------|--------|------|
| `package` | `delete` | 删除任意包版本 |
| `package` | `delete_own` | 仅删除自己上传的包版本 |
| `system` | `admin` | 超级管理员（已有） |

### 角色权限映射

| 角色 | 权限 |
|------|------|
| `admin` | `system:admin`（已有） |
| `maintainer` | `package:delete` |
| `developer` | `package:delete_own` |

## 审计日志模型

### AuditLog 表

```go
type AuditLog struct {
    ID           uint      `gorm:"primaryKey"`
    UserID       uint      `gorm:"index;not null"`
    Action       string    `gorm:"size:50;not null"`           // "package.delete"
    ResourceType string    `gorm:"size:50;not null"`           // "package"
    ResourceID   uint      `gorm:"index"`                      // 包ID
    RepoName     string    `gorm:"size:100"`                   // 仓库名
    Details      string    `gorm:"type:text"`                  // JSON: {name, version, uploader}
    IP           string    `gorm:"size:50"`                    // 操作人IP
    UserAgent    string    `gorm:"size:500"`                   // 客户端标识
    CreatedAt    time.Time `gorm:"autoCreateTime"`
}
```

## 架构

### 组件

| 组件 | 职责 |
|------|------|
| `middleware/rbac.go` | 已有，无需修改 |
| `model/audit_log.go` | 新增审计日志模型 |
| `repository/audit_repo.go` | 新增审计日志数据访问 |
| `service/audit_service.go` | 新增审计日志服务 |
| `database/migration.go` | 添加审计日志表和默认权限 |
| `handler/repo_router.go` | 在 HandleDelete 中调用审计服务 |

### 数据流

```
DELETE /repo/:repoName/*path
    ↓
RequirePermission("package", "delete") 中间件
    ↓ 检查用户权限
    ↓
HandleDelete
    ↓ 获取包信息（用于审计）
    ↓
Adapter.Delete
    ↓ 删除数据库记录 + 存储文件
    ↓
AuditService.LogDelete
    ↓ 写入审计日志
```

## 错误处理

- 权限不足：返回 403 Forbidden
- 包不存在：返回 404 Not Found
- 审计日志写入失败：不影响删除操作，仅记录错误日志

## 测试

- 单元测试：权限中间件、审计服务
- 集成测试：有权限用户可删除、无权限用户被拒绝、审计日志正确记录
