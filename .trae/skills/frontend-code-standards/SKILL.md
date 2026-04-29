---
name: "frontend-code-standards"
description: "Enforces QIM frontend coding standards for Vue 3 + TypeScript. Invoke when adding new features, creating components, or refactoring frontend code in the qim-client project."
---

# QIM 前端代码规范

## 1. 组件拆分原则（最重要）

**核心规则：一个文件只干一件事。如果功能可以抽象成独立组件，必须拆分成子组件，禁止往一个文件里持续追加代码。**

### 1.1 拆分信号

出现以下任一情况时，必须拆分组件：

- 模板中出现多个独立的 `v-if` 块控制不同功能的显示（如 MainContextMenus.vue 中同时包含右键菜单、动作菜单、用户菜单、成员菜单、群聊菜单、设置菜单、主题菜单、更多菜单）
- 一个组件的 props 超过 8 个，或 emits 超过 10 个
- 模板代码超过 150 行
- script 中同时处理多个独立业务逻辑（如同时处理消息、成员、通话、屏幕共享）
- 样式代码超过 300 行且包含多个独立模块的样式

### 1.2 拆分示例

**反例（禁止）：**

```vue
<!-- MainContextMenus.vue - 同时包含 8 种菜单 -->
<template>
  <div v-if="showMenu">...</div>
  <div v-if="showActionMenu">...</div>
  <div v-if="showUserMenu">...</div>
  <!-- 更多菜单... -->
</template>
```

**正例（推荐）：**

```vue
<!-- MainContextMenus.vue - 只作为容器 -->
<template>
  <ConversationMenu v-if="showMenu" ... />
  <ActionMenu v-if="showActionMenu" ... />
  <UserContextMenu v-if="showUserMenu" ... />
</template>

<!-- ConversationMenu.vue - 独立组件 -->
<template>
  <div class="context-menu">
    <div class="context-menu-item" @click="emit('pin')">...</div>
  </div>
</template>
```

### 1.3 目录结构规范

```
src/components/
  ├── menus/           # 菜单类组件
  │   ├── ConversationMenu.vue
  │   ├── ActionMenu.vue
  │   └── UserContextMenu.vue
  ├── modals/          # 弹窗类组件
  │   ├── AboutDialog.vue
  │   ├── LogoutDialog.vue
  │   └── UpdateDialog.vue
  ├── chat/            # 聊天相关组件
  │   ├── ChatWindow.vue      # 只负责布局容器
  │   ├── MessageList.vue     # 消息列表
  │   ├── MessageInput.vue    # 输入框
  │   └── MemberSidebar.vue   # 成员侧边栏
  └── shared/          # 通用基础组件
      ├── ContextMenu.vue
      └── Dialog.vue
```

## 2. Props / Emits 规范

### 2.1 接口定义

必须为 props 和 emits 定义 TypeScript 接口：

```typescript
interface Props {
  visible: boolean
  position: { x: number; y: number }
  conversation: Conversation | null
}

interface Emits {
  (e: 'close'): void
  (e: 'select', item: MenuItem): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
```

### 2.2 避免 props 爆炸

如果 props 超过 8 个，说明组件职责过重，需要拆分。

### 2.3 emit 命名规范

- 使用短横线连接的小写命名：`close-menu`、`select-item`
- 事件名必须是动词或动词短语

## 3. 逻辑复用规范

### 3.1 优先使用 Composables

可复用的业务逻辑必须提取到 `composables/` 目录：

```typescript
// composables/useChatRequest.ts
export function useChatRequest(baseUrl: string) {
  const getToken = (): string | null => localStorage.getItem('token')
  const request = async (url: string, options?: RequestInit) => { ... }
  return { getToken, request }
}
```

### 3.2 Composables 命名规范

- 文件名：`useXxx.ts`
- 函数名：`useXxx`
- 一个 composable 只处理一个职责领域

### 3.3 禁止在组件中重复实现通用逻辑

以下逻辑必须提取到 composables：

- HTTP 请求封装
- 时间格式化
- 文件处理
- WebSocket 消息处理
- 本地存储读写

## 4. 样式规范

### 4.1 样式拆分

组件样式必须跟随组件拆分：

```
├── ChatWindow.vue          # 只包含布局框架样式
├── MessageList.vue         # 消息列表样式
├── MessageItem.vue         # 单条消息样式
└── MessageBubble.vue       # 消息气泡样式
```

### 4.2 样式作用域

所有组件样式必须使用 `scoped`：

```vue
<style scoped>
.message-item { ... }
</style>
```

### 4.3 CSS 变量使用

使用 CSS 变量支持主题切换，禁止硬编码颜色值：

```css
/* 正确 */
.message-bubble {
  background: var(--primary-color);
  color: var(--text-color);
}

/* 错误 */
.message-bubble {
  background: #409eff;
  color: #333;
}
```

## 5. 类型定义规范

### 5.1 接口提取

多个组件共享的类型必须提取到 `src/types/` 目录：

```typescript
// types/index.ts
export interface Conversation {
  id: string | number
  name: string
  type: 'single' | 'group' | 'discussion'
  pinned?: boolean
  muted?: boolean
}

export interface Message {
  id: string
  content: string
  type: 'text' | 'image' | 'file'
  timestamp: number
  sender: User
  isSelf: boolean
}
```

### 5.2 禁止 any

除与外部库交互外，禁止使用 `any` 类型。

## 6. 错误处理规范

### 6.1 分析问题根因

**核心规则：解决问题要找到根因，禁止通过添加条件判断来掩盖错误。**

**反例（禁止）：**

```typescript
// 掩盖错误 - 不知道为什么会 undefined，直接加判断
const name = user?.name || ''
```

**正例（推荐）：**

```typescript
// 先分析为什么 user 可能为 undefined
// 如果是数据加载时机问题，应该在加载完成后再渲染
// 如果是类型定义问题，应该修正类型

// 方案1：使用类型守卫
if (!user) {
  throw new Error('User is required but not loaded')
}
const name = user.name

// 方案2：在父组件确保数据存在后再渲染子组件
<UserProfile v-if="user" :user="user" />
```

### 6.2 异步错误处理

所有异步操作必须有错误处理：

```typescript
try {
  const response = await request('/api/xxx')
  // 处理响应
} catch (error) {
  console.error('操作失败:', error)
  $message.error('操作失败，请重试')
}
```

## 7. 新功能开发流程

添加新功能时，按以下步骤执行：

1. **分析功能边界**：这个功能是否独立？是否可以复用？
2. **设计组件结构**：需要哪些组件？数据如何流转？
3. **检查现有代码**：是否有可复用的 composables 或组件？
4. **实现组件**：遵循本规范的所有规则
5. **验证规范**：检查是否满足组件拆分、类型定义、样式规范等要求

## 8. 重构检查清单

重构或添加功能后，检查以下项目：

- [ ] 一个文件是否只负责一个独立功能？
- [ ] props 是否超过 8 个？
- [ ] 是否有可复用的逻辑未提取到 composables？
- [ ] 是否定义了 Props 和 Emits 接口？
- [ ] 样式是否使用了 scoped？
- [ ] 是否使用了 CSS 变量而非硬编码颜色？
- [ ] 是否避免了 any 类型？
- [ ] 错误处理是否找到根因而非掩盖？
- [ ] 新增代码是否有重复逻辑可以复用现有代码？
