---
name: "awesome-claude-skills"
description: "A curated list of practical Claude Skills, resources, and tools for customizing Claude AI workflows. Invoke when user wants to explore Claude skills or find skill resources."
---

# Awesome Claude Skills

本技能基于 [ComposioHQ/awesome-claude-skills](https://github.com/ComposioHQ/awesome-claude-skills) 仓库，提供精选的 Claude 技能和资源。

## 什么是 Claude Skills？

Claude Skills 是可定制的工作流，教授 Claude 如何根据您的独特需求执行特定任务。技能使 Claude 能够在所有 Claude 平台上以可重复、标准化的方式执行任务。

## 技能分类

### 文档处理

- **docx** - 创建、编辑、分析带有跟踪更改、评论和格式的 Word 文档
- **pdf** - 提取文本、表格、元数据，合并和注释 PDF
- **pptx** - 读取、生成和调整幻灯片、布局、模板
- **xlsx** - 电子表格操作：公式、图表、数据转换
- **Markdown to EPUB Converter** - 将 Markdown 文档和聊天摘要转换为专业的 EPUB 电子书文件

### 开发和代码工具

- **artifacts-builder** - 使用现代前端 Web 技术（React、Tailwind CSS、shadcn/ui）创建复杂的多组件 claude.ai HTML 构件的工具套件
- **aws-skills** - 采用 CDK 最佳实践的 AWS 开发、成本优化 MCP 服务器和无服务器/事件驱动架构模式
- **Changelog Generator** - 通过分析历史并将技术提交转换为对客户友好的发布说明，自动从 git 提交创建面向用户的变更日志
- **Claude Code Terminal Title** - 为每个 Claud-Code 终端窗口提供动态标题，描述正在执行的工作，这样您就不会忘记哪个窗口在做什么
- **D3.js Visualization** - 教授 Claude 生成 D3 图表和交互式数据可视化
- **FFUF Web Fuzzing** - 集成 ffuf Web 模糊测试器，以便 Claude 可以运行模糊测试任务并分析漏洞结果
- **finishing-a-development-branch** - 通过呈现清晰的选项并处理所选工作流来指导开发工作的完成
- **iOS Simulator** - 使 Claude 能够与 iOS 模拟器交互，用于测试和调试 iOS 应用程序
- **jules** - 将编码任务委托给 Google Jules AI 代理，用于 GitHub 存储库上的异步错误修复、文档、测试和功能实现
- **LangSmith Fetch** - 通过自动从 LangSmith Studio 获取和分析执行跟踪来调试 LangChain 和 LangGraph 代理。Claude Code 的第一个 AI 可观测性技能
- **MCP Builder** - 指导创建高质量的 MCP（模型上下文协议）服务器，用于使用 Python 或 TypeScript 将外部 API 和服务与 LLM 集成
- **move-code-quality-skill** - 根据官方 Move Book Code Quality Checklist 分析 Move 语言包的 Move 2024 Edition 合规性和最佳实践
- **Playwright Browser Automation** - 用于测试和验证 Web 应用程序的模型调用 Playwright 自动化
- **prompt-engineering** - 教授众所周知的提示工程技术和模式，包括 Anthropic 最佳实践和代理说服原则
- **pypict-claude-skill** - 使用 PICT（成对独立组合测试）设计用于需求或代码的综合测试用例，生成具有成对覆盖率的优化测试套件
- **reddit-fetch** - 当 WebFetch 被阻止或返回 403 错误时，通过 Gemini CLI 获取 Reddit 内容
- **Skill Creator** - 提供创建有效 Claude 技能的指导，这些技能通过专业知识、工作流和工具集成扩展能力
- **Skill Seekers** - 在几分钟内自动将任何文档网站转换为 Claude AI 技能
- **software-architecture** - 实现设计模式，包括清洁架构、SOLID 原则和全面的软件设计最佳实践
- **subagent-driven-development** - 为单个任务分派独立的子代理，迭代之间设有代码审查检查点，以实现快速、受控的开发
- **test-driven-development** - 在实现功能或修复错误时使用，在编写实现代码之前

### 数据和分析

- **data-visualization** - 数据可视化和图表生成
- **spreadsheet-analysis** - 电子表格数据分析和处理

### 业务和营销

- **content-marketing** - 内容营销和文案撰写
- **social-media** - 社交媒体内容创建和管理

### 通信和写作

- **email-writing** - 电子邮件撰写和管理
- **technical-writing** - 技术文档编写

### 创意和媒体

- **art-generation** - 艺术和图像生成
- **music-creation** - 音乐创作和编辑

### 生产力和组织

- **task-management** - 任务管理和组织
- **time-tracking** - 时间跟踪和管理

### 协作和项目管理

- **project-management** - 项目管理和协调
- **team-collaboration** - 团队协作和沟通

### 安全和系统

- **security-audit** - 安全审计和漏洞评估
- **system-administration** - 系统管理和维护

### 通过 Composio 进行应用自动化

- **connect-apps** - 连接 Claude 到 500+ 应用程序，执行实际操作 - 发送电子邮件、创建问题、发布到 Slack

## 快速入门：连接 Claude 到 500+ 应用程序

1. **安装插件**
   ```bash
   claude --plugin-dir ./connect-apps-plugin
   ```

2. **运行设置**
   ```
   /connect-apps:setup
   ```
   当被询问时粘贴您的 API 密钥。（在 dashboard.composio.dev 获取免费密钥）

3. **重启并尝试**
   ```bash
   exit
   claude
   ```

   如果您收到电子邮件，Claude 现在已连接到 500+ 应用程序。

## 资源链接

- **GitHub 仓库**：https://github.com/ComposioHQ/awesome-claude-skills
- **Composio 仪表板**：https://dashboard.composio.dev
- **支持的应用程序**：查看所有支持的应用程序

## 贡献

欢迎贡献！请查看仓库的贡献指南了解详情。

## 许可证

MIT 许可证 - 自由使用和修改
