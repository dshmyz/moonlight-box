---
name: "claude-code-best-practise"
description: "Claude Code 最佳实践汇总，包含角色驱动开发流程、MUST/SHOULD/NEVER 规则、19 个技能、44 条命令等。Invoke when user asks for Claude Code best practices or wants to improve development workflow."
---

# Claude Code Best Practise

本技能汇总自 [cc-best](https://github.com/xiaobei930/cc-best) 项目，提供完整的 Claude Code 开发最佳实践。

## 核心价值

**问题**：Claude Code 功能强大，但从头配置工作流、编码规范和安全规则需要数小时。

**解决方案**：预配置的角色（PM → Lead → Designer → Dev → QA）模拟真实团队协作，内置安全护栏。

## 角色驱动开发流程

```
PM → Clarify → Lead → Designer → Dev → QA → Verify → Commit → Clear
```

### 7 个角色职责

| 角色 | 命令 | 职责 |
|------|------|------|
| PM | `/cc-best:pm` | 需求分析，创建任务分解 |
| Clarify | `/cc-best:clarify` | 需求澄清，处理低置信度项 |
| Lead | `/cc-best:lead` | 技术方案设计 |
| Designer | `/cc-best:designer` | UI 设计指导 |
| Dev | `/cc-best:dev` | 代码实现 |
| QA | `/cc-best:qa` | 功能验收测试 |
| Verify | `/cc-best:verify` | 构建 + 类型 + lint + 安全检查 |

### 开发模式

| 模式 | 命令 | 适用场景 | 特点 |
|------|------|----------|------|
| 自主迭代 | `/cc-best:iterate` | 任务清晰 | 完全自主，无需干预 |
| 结对编程 | `/cc-best:pair` | 学习、敏感操作 | 每步确认，人机协作 |
| 长循环 | `/cc-best:cc-ralph` | 小时级批处理 | 需要 ralph-loop 插件 |

### 自动角色选择逻辑

```
当前状态 → 选择角色 → 执行 → 更新 progress.md → 读取下一个任务 → 立即执行（不等待）
```

| 当前状态 | 选择角色 | 动作 |
|----------|----------|------|
| 无需求文档 | `/cc-best:pm` | 需求分析 |
| REQ 有低置信度项 | `/cc-best:clarify` | 需求澄清 |
| 有 REQ，无设计 | `/cc-best:lead` | 技术设计 |
| 有设计，前端任务 | `/cc-best:designer` | UI 设计指导 |
| 有任务待实现 | `/cc-best:dev` | 代码实现 |
| 代码就绪待验证 | `/cc-best:verify` | 构建 + 类型 + lint + 测试 + 安全 |
| 验证通过 | `/cc-best:qa` | 功能验收 |

## 开发模式详解

### /cc-best:iterate 自主迭代

当用户说 `/cc-best:iterate "add dark mode toggle"` 时，Claude 会：
1. 📋 `/cc-best:pm` → 分析需求，创建任务分解
2. 🏗️ `/cc-best:lead` → 设计技术方案
3. 💻 `/cc-best:dev` → 编写代码，创建测试
4. 🧪 `/cc-best:qa` → 运行测试，验证质量
5. ✅ `/cc-best:commit` → 提交更改

### /cc-best:pair 结对编程

5 个强制确认检查点：

| 检查点 | 示例 |
|--------|------|
| 理解确认 | "我理解你需要 X，对吗？" |
| 设计选择 | "选项 A 还是 B？我推荐 A，因为..." |
| 破坏性操作 | "即将删除 X，确认？" |
| 外部调用 | "将调用生产 API，继续？" |
| 提交确认 | "提交消息：'...'，可以吗？" |

学习模式：`/cc-best:pair --learn "teach me unit testing"` — Claude 详细解释每一步。

## 技能体系（19 个技能）

| 领域 | 技能 | 覆盖范围 |
|------|------|----------|
| Backend | backend, api, database | Python, TS, Java, Go, C# |
| Frontend | frontend, native | Web + iOS/macOS/Tauri |
| Quality | quality, testing, security, debug | TDD, OWASP, 性能分析 |
| Architecture | architecture, devops, git | ADR, CI/CD, 分支策略 |
| Routing | model | 任务→模型推荐 |
| Session | session, learning, compact, exploration | 生命周期 + 知识管理 |
| Research | search-first, second-opinion | 搜索策略，交叉验证 |

## 编码规范（43 条规则，10 个目录）

```
rules/
├── common/          # 通用规则
├── python/          # Python 规范
├── frontend/        # 前端规范
├── java/            # Java 规范
├── csharp/          # C# 规范
├── cpp/             # C++ 规范
├── embedded/        # 嵌入式规范
├── ui/              # UI 规范
├── rust/            # Rust 规范
└── go/              # Go 规范
```

## 安全 Hooks（30 个）

| 触发 | 功能 | 脚本 |
|------|------|------|
| PreToolUse | 验证危险命令 | validate-command.js |
| PreToolUse | git push 前确认 | pause-before-push.js |
| PreToolUse | 保护敏感文件 | protect-files.js |
| PreToolUse | 阻止随机 .md 创建 | block-random-md.js |
| PreToolUse | 长任务警告 | long-running-warning.js |
| PostToolUse | 自动格式化代码 | format-file.js |
| PostToolUse | 检查 console.log | check-console-log.js |
| PostToolUse | TypeScript 类型检查 | typescript-check.js |
| SessionStart | 会话健康检查 | session-check.js |
| PreCompact | 压缩前保存状态 | pre-compact.js |

## 最佳实践

### 1. 保持 CLAUDE.md 简洁

- 控制在 100 行以内
- 详细规格放在 rules/ 目录

### 2. 使用 Memory Bank

- 每个任务完成后更新 progress.md
- 在 architecture.md 记录重要决策

### 3. 上下文管理

- 普通模式：频繁使用 /clear 避免上下文溢出
- /cc-best:iterate 模式：不要手动 clear，保持循环连续性

### 4. 不要过度加载 MCP

- 每个项目启用不超过 10 个 MCP 服务器
- 使用 disabledMcpServers 禁用未使用的

### 5. 定期清理

- 删除未使用的语言规则
- 移除未使用的命令

## MCP 临时目录管理

MCP 工具会在项目中自动创建临时目录：

| 目录 | 来源 | 用途 |
|------|------|------|
| .playwright-mcp/ | MCP 自动创建 | Playwright MCP 临时文件 |
| .claude/mcp-data/ | MCP 自动创建 | MCP 共享数据 |
| \*-mcp/ | MCP 自动创建 | 其他 MCP 工具目录 |
| .claude/screenshots/ | 模板定义 | 手动保存的截图（有意义）|

清理脚本：
```bash
# 预览要删除的文件（dry run）
bash cleanup.sh --dry-run

# 清理 7 天以上的文件（默认）
bash cleanup.sh

# 清理 3 天以上的文件
bash cleanup.sh --days 3

# 清理所有 MCP 临时文件
bash cleanup.sh --all
```

## 与 Superpowers 的对比

| 场景 | 推荐 | 原因 |
|------|------|------|
| 团队协作 | CC-Best | 角色工作流（PM→Lead→Dev→QA） |
| 多语言栈 | CC-Best | 7 种语言编码规范目录 |
| 国内团队 | CC-Best | 双语文档 |
| solo 开发者 | Superpowers | 更轻量，git worktree 自动化 |
| 需要 git worktree | Superpowers | 自动创建隔离分支 |

**可以共存！** 使用 CC-Best 做工作流，Superpowers 做 git 自动化。

## 资源链接

- GitHub: https://github.com/xiaobei930/cc-best
- 主页: https://xiaobei930.github.io/cc-best/
- 官方最佳实践: https://docs.anthropic.com/
- CLAUDE.md 完整指南: https://docs.anthropic.com/en/docs/claude-code/claude-md
