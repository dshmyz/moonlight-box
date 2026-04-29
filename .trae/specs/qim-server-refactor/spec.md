# QIM-Server 性能与扩展性重构计划

## Why

当前 qim-server 存在以下问题和改进空间：

1. **handler.go 职责过重** - 137KB+ 的文件包含 97 个函数，混合了所有业务逻辑
2. **WebSocket Hub 可优化** - 消息广播性能有提升空间
3. **数据库查询未优化** - 存在 N+1 查询问题
4. **无消息持久化机制** - 离线消息无法可靠处理
5. **代码组织缺乏分层** - handler 直接操作数据库，缺少 service 层

## What Changes

### 阶段一：代码结构分层
- [ ] **拆分 handler.go 为模块化 handler**
  - auth_handler.go - 认证相关
  - conversation_handler.go - 会话相关
  - message_handler.go - 消息相关
  - file_handler.go - 文件相关
  - organization_handler.go - 组织架构
  - user_handler.go - 用户相关

- [ ] **新增 service 层**
  - message_service.go - 消息业务逻辑
  - conversation_service.go - 会话业务逻辑
  - user_service.go - 用户状态管理

### 阶段二：WebSocket Hub 优化
- [ ] **Hub 内部结构优化**
  - 使用 sync.Map 替代 map + mutex
  - 优化消息广播性能
  - 添加连接稳定性监控

- [ ] **本地消息队列优化**
  - 使用 channel 缓存替代直接发送
  - 优化锁竞争

### 阶段三：数据库优化
- [ ] **优化数据库操作**
  - 修复 N+1 查询问题
  - 添加必要的数据库索引
  - 批量预加载关联数据

### 阶段四：缓存层（内存）
- [ ] **添加本地缓存**
  - 用户信息缓存 (LRU)
  - 会话成员缓存
  - 热点数据缓存

### 阶段五：配置与错误处理
- [ ] **配置结构优化**
  - 拆分配置结构体
  - 添加配置验证

- [ ] **统一错误处理**
  - 定义标准错误码
  - 统一响应格式

## Impact

### 性能提升
- WebSocket 广播性能提升
- 数据库查询减少
- Handler 响应速度提升

### 可维护性提升
- 代码按职责清晰分层
- 每个文件 < 1000 行
- 便于定位和修改问题

### 扩展性
- 后续可轻松添加 Redis 等外部组件
- Service 层为分布式改造留有接口

## ADDED Requirements

### Requirement: Handler 模块化
系统 SHALL 将 handler.go 拆分为独立的 handler 模块，每个模块不超过 800 行。

#### Scenario: 添加新 API
- **WHEN** 需要添加新的消息类型 API
- **THEN** 只需修改 message_handler.go，不影响其他 handler

### Requirement: Service 层
系统 SHALL 提供 service 层，handler 不直接操作数据库。

#### Scenario: 发送消息
- **WHEN** 用户发送消息
- **THEN** handler 调用 message_service.SendMessage()，service 处理业务逻辑

### Requirement: 本地缓存
系统 SHALL 提供本地内存缓存，加速热点数据访问。

#### Scenario: 获取用户信息
- **WHEN** 频繁查询同一用户信息
- **THEN** 第二次查询从缓存返回，延迟 < 1ms

## MODIFIED Requirements

### Requirement: 配置管理
现有 config.yaml 配置格式保持兼容，配置结构体进行拆分重组。

## 目标目录结构

```
qim-server/
├── config/
│   └── config.go              # 拆分为独立配置结构体
├── handler/
│   ├── auth_handler.go       # 认证相关 (登录、注册、2FA、登出)
│   ├── user_handler.go       # 用户相关
│   ├── conversation_handler.go # 会话相关
│   ├── message_handler.go    # 消息相关
│   ├── group_handler.go      # 群组相关
│   ├── file_handler.go       # 文件相关
│   └── organization_handler.go # 组织架构
├── service/
│   ├── message_service.go    # 消息业务逻辑
│   ├── conversation_service.go # 会话业务逻辑
│   └── user_service.go       # 用户状态管理
├── cache/
│   └── local_cache.go        # 本地内存缓存 (LRU)
├── ws/
│   ├── hub.go                # Hub 优化
│   ├── client.go              # Client 优化
│   └── handler.go             # WebSocket 消息处理
├── middleware/
│   └── auth.go
├── model/
│   └── model.go
├── app/
│   ├── init.go
│   └── routes.go
├── main.go
└── go.mod
```

## 重构顺序

1. **Handler 拆分** - 按功能模块拆分现有 handler.go
2. **Service 层** - 抽取业务逻辑到 service
3. **Hub 优化** - 优化 WebSocket 性能
4. **本地缓存** - 添加 LRU 缓存
5. **数据库优化** - 修复 N+1 问题
6. **配置优化** - 拆分配置结构体
