# QIM-Server 性能与扩展性重构任务清单

## 阶段一：代码结构分层

### 任务 1: Handler 模块化拆分 ✅
- [x] 1.1 创建 handler/ 目录，将现有 handler.go 拆分
- [x] 1.2 拆分 auth_handler.go - 登录、注册、2FA、登出、Token刷新
- [x] 1.3 拆分 user_handler.go - 用户资料获取/更新、AI配置
- [x] 1.4 拆分 conversation_handler.go - 会话列表、创建、详情、置顶、免打扰、删除
- [x] 1.5 拆分 message_handler.go - 消息发送/获取/搜索/撤回/删除
- [x] 1.6 拆分 group_handler.go - 群成员增删、管理员设置、群主转让、公告更新
- [x] 1.7 拆分 file_handler.go - 文件上传/下载/删除
- [x] 1.8 拆分 organization_handler.go - 组织架构、部门创建、用户部门关联
- [x] 1.9 拆分 app_handler.go - 应用、小程序、笔记等
- [x] 1.10 拆分 notification_handler.go - 通知、日历、任务
- [x] 1.11 拆分 channel_handler.go - 频道相关
- [x] 1.12 拆分 shortlink_handler.go - 短链接
- [x] 1.13 拆分 statistics_handler.go - 统计报表
- [x] 1.14 拆分 misc_handler.go - 其他杂项 (版本检查等)
- [x] 1.15 更新 app/routes.go 引用新的 handler

### 任务 2: Service 层抽取 ✅
- [x] 2.1 创建 service/ 目录结构
- [x] 2.2 抽取 message_service.go - 消息发送核心逻辑
- [x] 2.3 抽取 conversation_service.go - 会话列表、创建、管理
- [x] 2.4 抽取 user_service.go - 用户状态更新、获取用户信息
- [x] 2.5 handler 层通过 service 调用业务逻辑

## 阶段二：WebSocket Hub 优化 ✅

### 任务 3: Hub 内部结构优化 ✅
- [x] 3.1 将 clients map 替换为 sync.Map
- [x] 3.2 将 userClients map 替换为 sync.Map
- [x] 3.3 优化 mutex 使用，减少锁竞争
- [x] 3.4 保留 conversationMembers mutex 用于特定场景

### 任务 4: 本地消息队列优化 ✅
- [x] 4.1 为 Client 添加 buffered channel 优化发送 (256 -> 1024)
- [x] 4.2 优化 broadcast 消息发送逻辑
- [x] 4.3 添加消息批量发送机制

## 阶段三：数据库优化 ✅

### 任务 5: 数据库查询优化 ✅
- [x] 5.1 分析并列出所有 N+1 查询问题
- [x] 5.2 为 Message 添加必要的预加载 (Sender, QuotedMessage, QuotedMessage.Sender)
- [x] 5.3 为 Conversation 添加 Members 预加载
- [x] 5.4 优化 GetConversations 减少循环内查询

## 阶段四：本地缓存层 ✅

### 任务 6: LRU 缓存实现 ✅
- [x] 6.1 创建 cache/local_cache.go 实现通用 LRU 缓存
- [x] 6.2 实现用户信息缓存 (UserCache)
- [x] 6.3 实现会话成员缓存 (ConversationMemberCache)
- [x] 6.4 添加缓存失效策略 (LRU 淘汰)

### 任务 7: 缓存集成 ✅
- [x] 7.1 在 message_service 中预留缓存接口
- [x] 7.2 在 user_service 中集成缓存
- [x] 7.3 在 conversation_service 中集成缓存

## 阶段五：配置与错误处理 ✅

### 任务 8: 配置结构优化 ✅
- [x] 8.1 将 config/config.go 拆分为独立配置结构体
- [x] 8.2 添加配置验证逻辑
- [x] 8.3 更新 app/init.go 使用新的配置结构

### 任务 9: 统一错误处理 ✅
- [x] 9.1 创建 pkg/errors/errors.go 定义标准错误码
- [x] 9.2 创建 pkg/response/response.go 统一响应格式
- [x] 9.3 在 handler 中使用统一响应格式 (auth_handler, user_handler, conversation_handler)

## 验证方式

- ✅ 项目编译通过
- ✅ Handler 按模块拆分
- ✅ Service 层实现核心业务逻辑
- ✅ Hub 使用 sync.Map 优化
- ✅ LRU 缓存实现
- ✅ 统一错误处理和响应格式
