# QIM Client 大文件拆分 - 实施计划

## 安全重构原则
- 使用SearchReplace工具每次只修改必要的部分
- 不要重写整个文件，只进行增量修改
- 每次修改后立即测试，确保功能正常
- 频繁提交，保持代码随时可工作

## Main.vue 拆分任务

### [ ] Task 1: 阅读Main.vue代码结构，分析可拆分组件
- **Priority**: P0
- **Depends On**: None
- **Description**:
  - 详细阅读Main.vue的template、script、style部分
  - 识别可拆分的组件单元
  - 确定组件之间的props和emit关系
- **Acceptance Criteria Addressed**: AC-1
- **Test Requirements**:
  - `human-judgment` TR-1.1: 确认Main.vue代码分析结果
  - `human-judgment` TR-1.2: 确认拆分方案合理性
- **Notes**: 这是基础工作，需要仔细分析

### [/] Task 2: 拆分SideOptions组件
- **Priority**: P0
- **Depends On**: Task 1
- **Description**:
  - 创建SideOptions.vue组件文件
  - 使用SearchReplace从Main.vue中提取SideOptions相关代码
  - 使用`<style scoped>`确保样式隔离
  - 更新Main.vue中的导入和使用
- **Acceptance Criteria Addressed**: AC-1, AC-3
- **Test Requirements**:
  - `programmatic` TR-2.1: 项目能够正常构建
  - `human-judgment` TR-2.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分
- **Status**: 已完成 ✓

### [ ] Task 3: 拆分Sidebar组件
- **Priority**: P0
- **Depends On**: Task 2
- **Description**:
  - 创建Sidebar.vue组件文件
  - 使用SearchReplace从Main.vue中提取Sidebar相关代码
  - 包含用户信息、搜索、会话列表等
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-1, AC-3
- **Test Requirements**:
  - `programmatic` TR-3.1: 项目能够正常构建
  - `human-judgment` TR-3.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分

### [ ] Task 4: 拆分ConversationList组件
- **Priority**: P0
- **Depends On**: Task 3
- **Description**:
  - 创建ConversationList.vue组件文件
  - 使用SearchReplace从Main.vue中提取ConversationList相关代码
  - 使用`<style scoped>`确保样式隔离
  - 保持事件处理逻辑
- **Acceptance Criteria Addressed**: AC-1, AC-3
- **Test Requirements**:
  - `programmatic` TR-4.1: 项目能够正常构建
  - `human-judgment` TR-4.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分

### [ ] Task 5: 拆分OrgTree组件
- **Priority**: P0
- **Depends On**: Task 4
- **Description**:
  - 创建OrgTree.vue组件文件
  - 使用SearchReplace从Main.vue中提取OrgTree相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-1, AC-3
- **Test Requirements**:
  - `programmatic` TR-5.1: 项目能够正常构建
  - `human-judgment` TR-5.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分

### [/] Task 6: 拆分AppPanel组件
- **Priority**: P0
- **Depends On**: Task 5
- **Description**:
  - 创建AppPanel.vue组件文件
  - 使用SearchReplace从Main.vue中提取AppPanel相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-1, AC-3
- **Test Requirements**:
  - `programmatic` TR-6.1: 项目能够正常构建
  - `human-judgment` TR-6.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分
- **Status**: 已完成 ✓

### [ ] Task 7: 拆分弹窗组件
- **Priority**: P0
- **Depends On**: Task 6
- **Description**:
  - 创建各种弹窗组件文件（AboutDialog、SettingsDialog等）
  - 使用SearchReplace从Main.vue中提取弹窗相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-1, AC-3
- **Test Requirements**:
  - `programmatic` TR-7.1: 项目能够正常构建
  - `human-judgment` TR-7.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分

## ChatWIndow.vue 拆分任务

### [ ] Task 8: 阅读ChatWIndow.vue代码结构，分析可拆分组件
- **Priority**: P0
- **Depends On**: Task 7
- **Description**:
  - 详细阅读ChatWIndow.vue的template、script、style部分
  - 识别可拆分的组件单元
  - 确定组件之间的props和emit关系
- **Acceptance Criteria Addressed**: AC-2
- **Test Requirements**:
  - `human-judgment` TR-8.1: 确认ChatWIndow.vue代码分析结果
  - `human-judgment` TR-8.2: 确认拆分方案合理性
- **Notes**: 这是基础工作，需要仔细分析

### [x] Task 9: 拆分ChatHeader组件
- **Priority**: P0
- **Depends On**: Task 8
- **Description**:
  - 创建ChatHeader.vue组件文件
  - 使用SearchReplace从ChatWindow.vue中提取ChatHeader相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-2, AC-3
- **Test Requirements**:
  - `programmatic` TR-9.1: 项目能够正常构建
  - `human-judgment` TR-9.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分
- **Status**: 已完成 ✓

### [x] Task 10: 拆分MessageList组件
- **Priority**: P0
- **Depends On**: Task 9
- **Description**:
  - 创建MessageListView.vue组件文件
  - 使用SearchReplace从ChatWindow.vue中提取MessageList相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-2, AC-3
- **Test Requirements**:
  - `programmatic` TR-10.1: 项目能够正常构建
  - `human-judgment` TR-10.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分
- **Status**: 已完成 ✓

### [ ] Task 11: 拆分MessageInput组件
- **Priority**: P0
- **Depends On**: Task 10
- **Description**:
  - 创建MessageInput.vue组件文件
  - 使用SearchReplace从ChatWIndow.vue中提取MessageInput相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-2, AC-3
- **Test Requirements**:
  - `programmatic` TR-11.1: 项目能够正常构建
  - `human-judgment` TR-11.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分

### [x] Task 12: 拆分EmojiPanel组件
- **Priority**: P0
- **Depends On**: Task 11
- **Description**:
  - 创建EmojiPanel.vue组件文件
  - 使用SearchReplace从ChatWindow.vue中提取EmojiPanel相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-2, AC-3
- **Test Requirements**:
  - `programmatic` TR-12.1: 项目能够正常构建
  - `human-judgment` TR-12.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分
- **Status**: 已完成 ✓

### [ ] Task 13: 拆分MembersSidebar组件
- **Priority**: P0
- **Depends On**: Task 12
- **Description**:
  - 创建MembersSidebar.vue组件文件
  - 使用SearchReplace从ChatWIndow.vue中提取MembersSidebar相关代码
  - 使用`<style scoped>`确保样式隔离
- **Acceptance Criteria Addressed**: AC-2, AC-3
- **Test Requirements**:
  - `programmatic` TR-13.1: 项目能够正常构建
  - `human-judgment` TR-13.2: 组件功能正常，样式无冲突
- **Notes**: 小步重构，每次只拆分一个组件，只修改必要的部分

## 测试与验证任务

### [ ] Task 14: 功能完整性测试
- **Priority**: P0
- **Depends On**: Task 7, Task 13
- **Description**:
  - 运行开发服务器
  - 测试登录、消息发送、应用使用等核心功能
  - 确保所有现有功能正常运行
- **Acceptance Criteria Addressed**: AC-4
- **Test Requirements**:
  - `programmatic` TR-14.1: 开发服务器能够正常启动
  - `programmatic` TR-14.2: 核心功能测试通过
- **Notes**: 测试过程中注意观察功能是否正常

### [ ] Task 15: 样式一致性验证
- **Priority**: P0
- **Depends On**: Task 14
- **Description**:
  - 查看页面样式
  - 对比重构前后的页面截图
  - 确保所有页面样式与重构前完全一致
  - 检查是否有样式冲突
- **Acceptance Criteria Addressed**: AC-5
- **Test Requirements**:
  - `human-judgment` TR-15.1: 页面样式与重构前一致
  - `human-judgment` TR-15.2: 无样式冲突
- **Notes**: 重点关注聊天界面和应用界面的样式

### [ ] Task 16: 代码审查
- **Priority**: P1
- **Depends On**: Task 15
- **Description**:
  - 检查重构后的代码结构
  - 确保代码可读性和可维护性提高
  - 确认所有导入路径正确
- **Acceptance Criteria Addressed**: AC-1, AC-2, AC-3
- **Test Requirements**:
  - `human-judgment` TR-16.1: 代码结构清晰，组件划分合理
  - `human-judgment` TR-16.2: 样式文件组织规范，便于维护
- **Notes**: 参考Vue项目最佳实践进行代码审查
