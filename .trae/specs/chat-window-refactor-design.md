# ChatWindow.vue 拆分设计文档

## 概述

将 ChatWindow.vue（约 3579 行）拆分为多个职责单一的小组件和 composables，目标是缩减至 1000 行以内，同时**不改变任何现有功能和样式**。

## 拆分原则

1. **保持功能和样式不变**：仅进行代码重组，不修改任何业务逻辑和视觉效果
2. **渐进式重构**：每个子任务独立可验证，逐步推进
3. **通过 props/events 通信**：新组件与 ChatWindow 之间使用标准的 Vue 组件通信方式
4. **保持现有导入和依赖**：不改变外部 API 和依赖关系

## 拆分任务清单

### 任务 1：抽离消息输入区域组件 (MessageInputArea.vue)

**职责**：处理消息输入相关的所有 UI 和交互

**包含内容**：
- 工具栏按钮（语音、视频、屏幕共享、表情、文件、图片、截图、消息管理、小程序）
- 表情面板（常用表情、表情符号、动物与自然分类）
- @成员面板（搜索、选择成员、@所有人）
- 小程序列表面板（MiniAppManager 组件）
- 待发送文件预览区域
- 文本输入框 (textarea)
- 发送按钮和快捷键提示

**Props**：
- `conversation`: Conversation 类型
- `pendingFiles`: File[] 类型
- `showEmojiPanel`: boolean
- `showAtMembersPanel`: boolean
- `showMiniAppList`: boolean
- `conversationMembers`: Member[] 类型
- `commonEmojis`: string[] 类型
- `faceEmojis`: string[] 类型
- `animalEmojis`: string[] 类型
- `isElectron`: boolean
- `inputMessage`: string 类型 (v-model)

**Emits**：
- `update:inputMessage`: string
- `toggle-emoji-panel`
- `toggle-at-members-panel`
- `toggle-mini-app-list`
- `select-file`
- `select-image`
- `take-screenshot`
- `open-message-manager`
- `start-voice-call`
- `start-video-call`
- `start-screen-share`
- `send`: content: string
- `insert-emoji`: emoji: string
- `select-at-member`: member: Member
- `select-at-all`
- `remove-pending-file`: index: number
- `handle-paste`: event: ClipboardEvent

**预估减少行数**：~250 行

### 任务 2：抽离群成员侧边栏组件 (MemberSidebar.vue)

**职责**：群成员列表展示和搜索

**包含内容**：
- 成员列表头部（标题、折叠按钮、搜索切换）
- 成员搜索输入框
- 成员列表项（头像、名称、角色标识）
- 侧边栏折叠/展开状态

**Props**：
- `conversation`: Conversation 类型
- `isMembersSidebarExpanded`: boolean
- `showMemberSearch`: boolean
- `memberSearchQuery`: string 类型 (v-model)

**Emits**：
- `update:memberSearchQuery`: string
- `toggle-sidebar-expanded`
- `toggle-member-search`
- `show-member-context-menu`: event: MouseEvent, member: Member
- `start-private-chat`: member: Member

**预估减少行数**：~150 行

### 任务 3：抽离群管理面板组件 (GroupManagementPanel.vue)

**职责**：群信息管理、群公告编辑、群解散确认

**包含内容**：
- 修改群名称弹窗
- 编辑群公告弹窗
- 解散群聊确认对话框
- 头部下拉菜单（修改群名、编辑公告、解散群聊）

**Props**：
- `conversation`: Conversation 类型
- `showHeaderMenu`: boolean
- `showEditGroupInfoModal`: boolean
- `showEditAnnouncementModal`: boolean
- `showConfirmDialog`: boolean
- `editGroupName`: string 类型 (v-model)
- `editAnnouncementContent`: string 类型 (v-model)
- `confirmDialogTitle`: string
- `confirmDialogMessage`: string

**Emits**：
- `update:showHeaderMenu`: boolean
- `update:showEditGroupInfoModal`: boolean
- `update:showEditAnnouncementModal`: boolean
- `update:showConfirmDialog`: boolean
- `update:editGroupName`: string
- `update:editAnnouncementContent`: string
- `edit-group-info`
- `edit-group-announcement`
- `confirm-delete-group`
- `save-group-info`: groupName: string
- `save-group-announcement`: announcement: string
- `handle-confirm-action`
- `close-confirm-dialog`
- `invite-members`

**预估减少行数**：~200 行

### 任务 4：抽离上下文菜单组件

**职责**：消息和成员的右键上下文菜单

#### 4.1 MessageContextMenu.vue

**Props**：
- `visible`: boolean
- `position`: { x: number, y: number }
- `selectedMessage`: Message 类型

**Emits**：
- `preview-image`: imageUrl: string
- `download-file`: fileUrl: string
- `save-file-as`: fileUrl: string
- `copy-message`
- `forward-message`
- `quote-message`
- `add-to-note`
- `recall-message`
- `send-message-reminder`

#### 4.2 MemberContextMenu.vue

**Props**：
- `visible`: boolean
- `position`: { x: number, y: number }
- `selectedMember`: Member 类型
- `currentUserId`: number
- `conversation`: Conversation 类型

**Emits**：
- `remove-member`
- `view-member-info`
- `set-admin`
- `transfer-owner`
- `send-private-message`

**预估减少行数**：~150 行

### 任务 5：抽离弹窗组件集合

**职责**：各种模态框和弹窗

#### 5.1 ReadUsersModal.vue
- 已读用户列表弹窗

#### 5.2 ScreenshotPreviewDialog.vue
- 截图预览对话框

#### 5.3 ImagePreviewDialog.vue
- 图片预览对话框

#### 5.4 SharePreviewDialog.vue
- 分享内容预览对话框

#### 5.5 CallModal.vue
- 语音/视频通话模态框

**预估减少行数**：~350 行

### 任务 6：抽离工具函数和状态到 Composables

#### 6.1 useChatRequest.ts
- request 函数
- getToken 函数
- formatDate 函数

#### 6.2 useChatUtils.ts
- getFileIcon 函数
- formatFileSize 函数
- renderMarkdown 函数
- getAvatarUrl 函数（已存在于 utils）
- shouldShowTimeDivider 函数
- formatTime 函数

#### 6.3 useChatState.ts
- $message 提示系统
- 确认对话框状态管理
- streamingMessages 状态

#### 6.4 useMessageActions.ts
- 消息复制、转发、引用、撤回等操作
- 消息搜索相关逻辑
- 文件处理逻辑（选择、预览、上传）

#### 6.5 useMemberActions.ts
- 成员管理操作（移除、设管理员、转让群主）
- 私聊相关逻辑
- 成员过滤和搜索

#### 6.6 useEmojiState.ts
- 表情面板状态管理
- @成员面板状态管理
- 表情插入逻辑

**预估减少行数**：~300 行

### 任务 7：清理和验证

- 清理 ChatWindow.vue 中不再使用的导入
- 确保所有功能正常工作
- 验证样式无变化
- 运行 lint 和 typecheck

## 执行顺序

```
任务 1 (MessageInputArea) → 无依赖
任务 2 (MemberSidebar) → 无依赖
任务 3 (GroupManagementPanel) → 无依赖
任务 4 (ContextMenus) → 无依赖，可与任务 1-3 并行
任务 5 (Dialogs) → 无依赖，可与任务 1-3 并行
任务 6 (Composables) → 依赖任务 1-5 完成，因为需要知道最终的函数使用情况
任务 7 (清理验证) → 依赖所有任务完成
```

## 预期效果

| 指标 | 重构前 | 重构后 | 改善 |
|------|--------|--------|------|
| ChatWindow.vue 行数 | ~3579 | < 1000 | -72% |
| 新增组件数量 | 0 | 9 | 9 个 |
| 新增 composables | 0 | 6 | 6 个 |
| 单文件最大行数 | ~3579 | < 1000 | -72% |

## 风险和注意事项

1. **不改变功能和样式**：所有拆分仅涉及代码重组，不修改任何业务逻辑和视觉效果
2. **渐进式验证**：每完成一个任务，都需要验证功能是否正常工作
3. **保持 Props/Events 接口稳定**：新组件与 ChatWindow 之间的接口需要清晰定义
4. **测试策略**：由于项目无自动化测试，需要人工验证每个阶段的核心功能
5. **CSS 样式保持不变**：样式类名和结构保持完全一致，仅调整文件组织
