# QIM Client 大文件拆分 - 产品需求文档

## Overview
- **Summary**: 对QIM Client项目中的大型Vue组件进行拆分，将Main.vue和ChatWIndow.vue拆分为多个职责明确的子组件，并将相关样式也拆分出来，提高代码可维护性和可读性。
- **Purpose**: 解决大型组件难以维护和理解的问题，使每个组件职责单一，便于后续功能扩展和团队协作。
- **Target Users**: 开发团队成员，包括前端开发人员和代码维护人员。

## Goals
- 将Main.vue拆分为多个独立组件
- 将ChatWIndow.vue拆分为多个独立组件
- 将每个组件的样式拆分到独立的样式文件中
- 确保拆分后不改动任何现有功能和样式
- 确保样式隔离，不产生样式冲突
- 提高代码可维护性和可读性

## Non-Goals (Out of Scope)
- 不修改任何现有功能逻辑
- 不改变任何现有页面样式
- 不添加新功能
- 不修改其他已合理拆分的文件

## Background & Context
- Main.vue (11970行) 是最大的组件，包含侧边栏、主内容区、弹窗等大量功能
- ChatWIndow.vue (6956行) 是第二大的组件，包含聊天窗口、消息列表、输入区域等功能
- 这些大型组件难以维护，需要拆分为更小的、可管理的组件
- 样式文件也需要拆分到独立文件中，使用scoped样式确保样式隔离
- 遵循cg.md的小步重构原则，每次只做一个小改动
- **重要**：如果重构过程中出现失败，使用SearchReplace工具只修改必要的部分，不要重写整个文件

## Functional Requirements

### Main.vue 拆分方案

#### FR-1: 拆分侧边栏组件
- **SideOptions.vue** - 左侧垂直选项栏（最近联系人、组织架构、群聊、应用等选项）
- **Sidebar.vue** - 侧边栏（包含用户信息、搜索、联系人列表等）
- **ConversationList.vue** - 最近联系人会话列表
- **GroupList.vue** - 已有组件，保留
- **OrgTree.vue** - 组织架构树形结构

#### FR-2: 拆分主内容区域组件
- **ChatArea.vue** - 聊天区域容器
- **GroupDetailPanel.vue** - 群聊详情面板
- **UserProfilePanel.vue** - 用户资料面板
- **AppPanel.vue** - 应用面板

#### FR-3: 拆分弹窗和模态框
- **NetworkError.vue** - 网络错误提示
- **AboutDialog.vue** - 关于对话框
- **SettingsDialog.vue** - 设置对话框
- **UpdateDialog.vue** - 更新对话框
- **LogoutDialog.vue** - 退出登录对话框

#### FR-4: 拆分子应用组件
- **RecentApps.vue** - 最近使用的应用
- **AllApps.vue** - 所有应用列表
- **AppCategories.vue** - 应用分类

### ChatWIndow.vue 拆分方案

#### FR-5: 拆分聊天头部组件
- **ChatHeader.vue** - 聊天头部（头像、名称、状态、操作按钮）

#### FR-6: 拆分消息相关组件
- **MessageList.vue** - 消息列表容器
- **MessageSearch.vue** - 消息搜索
- **TimeDivider.vue** - 时间分隔线

#### FR-7: 拆分消息输入区域
- **MessageInput.vue** - 消息输入区域
- **EmojiPanel.vue** - 表情选择面板
- **AtMembersPanel.vue** - @成员选择面板

#### FR-8: 拆分侧边栏组件
- **MembersSidebar.vue** - 群成员侧边栏

### 样式拆分方案

#### FR-9: 样式拆分原则
- 每个Vue组件都使用`<style scoped>`来确保样式隔离
- 如果多个组件共享相同样式，提取到公共样式文件（如/assets/styles/目录下）
- 避免使用全局样式，优先使用scoped样式
- 拆分后的样式文件应与组件文件放在同一目录或对应的styles目录下

#### FR-10: 样式文件组织
- `/assets/styles/` 目录存放全局公共样式
- 每个组件目录下的`*.css`文件存放该组件专用的样式
- 使用有意义的样式类名，避免样式冲突
- 对于重复使用的样式类，使用CSS变量或公共类名

### 安全重构原则

#### FR-11: 安全的重构方法
- 使用SearchReplace工具每次只修改必要的部分
- 不要重写整个文件，只进行增量修改
- 每次修改后立即测试，确保功能正常
- 频繁提交，保持代码随时可工作
- 如果修改失败，回滚到上一个可工作的版本

## Non-Functional Requirements
- **NFR-1**: 代码结构清晰，组件职责单一
- **NFR-2**: 组件之间通过props和emit进行通信
- **NFR-3**: 重构过程遵循小步原则，每次拆分后测试
- **NFR-4**: 代码可读性和可维护性提高
- **NFR-5**: 样式隔离，不产生样式冲突

## Constraints
- **Technical**: 保持现有技术栈不变，使用Vue 3 + TypeScript
- **Business**: 确保重构过程中项目可以正常构建和运行
- **Dependencies**: 保持现有依赖不变

## Assumptions
- 现有代码功能正常，样式显示正确
- 重构过程中可以运行开发服务器进行测试
- 项目使用Vite构建工具

## Acceptance Criteria

### AC-1: Main.vue拆分完成
- **Given**: 现有的Main.vue组件
- **When**: 拆分为多个子组件
- **Then**: Main.vue的代码量显著减少，每个子组件职责明确
- **Verification**: `human-judgment`
- **Notes**: 保持所有功能不变

### AC-2: ChatWIndow.vue拆分完成
- **Given**: 现有的ChatWIndow.vue组件
- **When**: 拆分为多个子组件
- **Then**: ChatWIndow.vue的代码量显著减少，每个子组件职责明确
- **Verification**: `human-judgment`
- **Notes**: 保持所有功能不变

### AC-3: 样式拆分完成
- **Given**: 现有的组件样式
- **When**: 拆分为独立的样式文件
- **Then**: 每个组件的样式独立，不产生冲突
- **Verification**: `human-judgment`
- **Notes**: 使用scoped样式确保隔离

### AC-4: 功能保持完整
- **Given**: 拆分后的代码
- **When**: 运行项目
- **Then**: 所有现有功能正常运行
- **Verification**: `programmatic`
- **Notes**: 测试登录、消息发送、应用使用等核心功能

### AC-5: 样式保持不变
- **Given**: 拆分后的代码
- **When**: 查看页面
- **Then**: 所有页面样式与拆分前完全一致
- **Verification**: `human-judgment`
- **Notes**: 对比拆分前后的页面截图

## Open Questions
- [ ] 具体的拆分粒度需要根据代码内容进一步确定
- [ ] 组件之间的状态管理方式需要确定（props/emit vs provide/inject）
- [ ] 样式冲突的排查和测试方案需要确定
