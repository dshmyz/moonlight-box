# QIM-Server 性能与扩展性重构检查清单

## 阶段一：Handler 模块化拆分

### auth_handler.go
- [ ] Login 函数正确处理
- [ ] Register 函数正确处理
- [ ] VerifyTwoFA 函数正确处理
- [ ] CheckTwoFAStatus 函数正确处理
- [ ] CheckVersion 函数正确处理
- [ ] Logout 函数正确处理
- [ ] RefreshToken 函数正确处理

### user_handler.go
- [ ] GetCurrentUser 函数正确处理
- [ ] UpdateUser 函数正确处理
- [ ] SearchUsers 函数正确处理
- [ ] CreateUser 函数正确处理
- [ ] GetAIConfig 函数正确处理
- [ ] UpdateAIConfig 函数正确处理

### conversation_handler.go
- [ ] GetConversations 函数正确处理
- [ ] GetConversation 函数正确处理
- [ ] CreateSingleConversation 函数正确处理
- [ ] CreateGroupConversation 函数正确处理
- [ ] CreateDiscussionConversation 函数正确处理
- [ ] PinConversation 函数正确处理
- [ ] SetConversationMute 函数正确处理
- [ ] DeleteConversation 函数正确处理

### message_handler.go
- [ ] GetMessages 函数正确处理
- [ ] SendMessage 函数正确处理
- [ ] StreamMessage 函数正确处理
- [ ] MarkConversationAsRead 函数正确处理
- [ ] GetMessageReadUsers 函数正确处理
- [ ] RecallMessage 函数正确处理
- [ ] RemindMessage 函数正确处理
- [ ] DeleteMessage 函数正确处理
- [ ] SearchMessages 函数正确处理
- [ ] GetMessageQuoteChain 函数正确处理
- [ ] GetMessagesByFilter 函数正确处理

### group_handler.go
- [ ] AddMemberToGroup 函数正确处理
- [ ] RemoveMemberFromGroup 函数正确处理
- [ ] ExitGroup 函数正确处理
- [ ] UpdateGroupInfo 函数正确处理
- [ ] SetMemberRole 函数正确处理
- [ ] TransferOwner 函数正确处理
- [ ] UpdateAnnouncement 函数正确处理

### file_handler.go
- [ ] UploadFile 函数正确处理
- [ ] GetFiles 函数正确处理
- [ ] DownloadFile 函数正确处理
- [ ] DeleteFile 函数正确处理

### organization_handler.go
- [ ] GetOrganizationTree 函数正确处理
- [ ] CreateDepartment 函数正确处理
- [ ] AddUserToDepartment 函数正确处理

### app_handler.go
- [ ] GetApps 函数正确处理
- [ ] GetAllApps 函数正确处理
- [ ] CreateApp 函数正确处理
- [ ] UpdateApp 函数正确处理
- [ ] DeleteApp 函数正确处理
- [ ] GetMiniApps 函数正确处理
- [ ] GetMiniApp 函数正确处理
- [ ] CreateMiniApp 函数正确处理
- [ ] UpdateMiniApp 函数正确处理
- [ ] DeleteMiniApp 函数正确处理

### notification_handler.go
- [ ] GetNotifications 函数正确处理
- [ ] MarkNotificationAsRead 函数正确处理
- [ ] MarkAllNotificationsAsRead 函数正确处理
- [ ] GetEvents 函数正确处理
- [ ] CreateEvent 函数正确处理
- [ ] GetEvent 函数正确处理
- [ ] UpdateEvent 函数正确处理
- [ ] DeleteEvent 函数正确处理
- [ ] GetTasks 函数正确处理
- [ ] CreateTask 函数正确处理
- [ ] UpdateTask 函数正确处理
- [ ] DeleteTask 函数正确处理
- [ ] UpdateTaskStatus 函数正确处理

### channel_handler.go
- [ ] CreateChannel 函数正确处理
- [ ] GetChannels 函数正确处理
- [ ] SubscribeChannel 函数正确处理
- [ ] UnsubscribeChannel 函数正确处理
- [ ] CreateChannelMessage 函数正确处理
- [ ] GetChannelMessages 函数正确处理

### shortlink_handler.go
- [ ] CreateShortLink 函数正确处理
- [ ] GetShortLinks 函数正确处理
- [ ] RedirectShortLink 函数正确处理

### statistics_handler.go
- [ ] GetStatistics 函数正确处理

### misc_handler.go
- [ ] GetBots 函数正确处理
- [ ] GetNotes 函数正确处理
- [ ] GetNote 函数正确处理
- [ ] CreateNote 函数正确处理
- [ ] UpdateNote 函数正确处理
- [ ] DeleteNote 函数正确处理
- [ ] CreateFolder 函数正确处理
- [ ] GetFolderTree 函数正确处理
- [ ] GetSystemMessages 函数正确处理
- [ ] CreateSystemMessage 函数正确处理
- [ ] UpdateSystemMessage 函数正确处理

### routes.go 集成
- [ ] 所有路由正确引用新的 handler
- [ ] API 路径保持不变
- [ ] 中间件配置正确

## 阶段二：Service 层

### message_service.go
- [ ] SendMessage 逻辑正确
- [ ] GetMessages 预加载正确
- [ ] SearchMessages 逻辑正确
- [ ] RecallMessage 逻辑正确
- [ ] DeleteMessage 逻辑正确

### conversation_service.go
- [ ] GetConversations 逻辑正确
- [ ] CreateSingleConversation 逻辑正确
- [ ] CreateGroupConversation 逻辑正确
- [ ] UpdateConversation 逻辑正确
- [ ] DeleteConversation 逻辑正确

### user_service.go
- [ ] UpdateUserStatus 逻辑正确
- [ ] GetUser 逻辑正确

### Handler 调用 Service
- [ ] handler 不直接调用 database.GetDB()
- [ ] handler 通过 service 层操作数据

## 阶段三：WebSocket Hub 优化

### Hub 结构优化
- [ ] clients 使用 sync.Map
- [ ] userClients 使用 sync.Map
- [ ] 锁竞争减少

### 消息队列优化
- [ ] Client send channel buffer 优化
- [ ] broadcast 性能提升

## 阶段四：本地缓存

### LRU 缓存
- [ ] 本地缓存实现正确
- [ ] TTL 策略有效
- [ ] LRU 淘汰正常

### 缓存集成
- [ ] user_service 使用缓存
- [ ] conversation_service 使用缓存
- [ ] 缓存命中率 > 50%

## 阶段五：配置与错误处理

### 配置优化
- [ ] 配置结构体拆分清晰
- [ ] 配置验证生效
- [ ] config.yaml 兼容

### 统一错误处理
- [ ] 标准错误码定义
- [ ] 统一响应格式
- [ ] handler 使用统一格式

## 集成验证

- [ ] 项目编译通过
- [ ] 单元测试通过 (如有)
- [ ] API 接口路径不变
- [ ] WebSocket 连接正常
- [ ] 消息发送/接收正常
