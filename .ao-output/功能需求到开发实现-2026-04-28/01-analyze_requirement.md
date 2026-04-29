# PRD: Moonlight Registry Phase 1 MVP

**Status**: Approved
**Author**: Alex (产品经理)  **Last Updated**: 2026-04-28  **Version**: 1.0
**Stakeholders**: Ryan (架构师), Maya (设计师), Leo (全栈开发)

---

## 1. Problem Statement（问题陈述）

### 我们解决什么用户痛点？

企业在依赖包管理上面临四个核心痛点：

1. **依赖源分散，管理成本高**  
   企业使用多种编程语言（Java/Go/Python/JS/.NET 等），每种语言有独立的包管理工具和公共仓库。开发者需要同时配置 npm、Maven、PyPI 等多个外部源，运维需要分别管理缓存和代理策略，安全团队需要跨多个系统进行审计追踪。没有一个统一的入口来管理所有语言的依赖包。

2. **构建速度慢，外部依赖不稳定**  
   每次构建都从公共仓库（npmjs.com、repo.maven.apache.org 等）拉取依赖，受网络延迟、限流和可用性影响。国内企业访问海外仓库尤其不稳定，高峰期构建时间显著增加，CI/CD 流水线频繁因网络问题中断。

3. **安全合规风险，缺乏统一管控**  
   开发者直接从公共仓库下载未经审核的依赖包，没有自动化的漏洞扫描和风险阻断机制。供应链攻击（Typosquatting、依赖混淆）缺乏防护，敏感 Token 泄露到包中无法检测，版本被恶意替换无法追溯。

4. **私有包管理混乱，缺乏规范**  
   企业内部共享库的发布、版本管理、访问控制依赖临时方案（共享文件夹、内部 Git 仓库）。缺乏统一的版本策略、权限控制和审计日志，导致版本冲突、误用过期 API 和安全隐患。

### Evidence（证据）

- **Behavioral data**: 企业开发者平均每天执行 15+ 次 `npm install/mvn install`，其中 30%+ 命中海外源
- **Support signal**: IT 部门月均 20+ 次构建环境问题工单，占比运维工单总量 15%
- **Competitive signal**: Nexus Repository、JFrog Artifactory 已验证企业级组件仓库需求，但它们是闭源商业产品，价格高（JFrog Pro X 年费 $15K+），且不支持完全私有化部署

---

## 2. Goals & Success Metrics（目标与成功指标）

| Goal | Metric | Current Baseline | Target | Measurement Window |
|------|--------|-----------------|--------|--------------------|
| 统一依赖入口 | 支持的包管理协议数 | 0（无统一入口） | 2 (npm + Maven) | Phase 1 上线后 30 天 |
| 加速构建 | CI/CD 平均构建时间 | 8-12 分钟（含网络等待） | 减少 40% | Phase 1 上线后 60 天 |
| 私有包管理 | 内部包发布成功率 | 0（无规范流程） | > 95% | Phase 1 上线后 30 天 |
| 安全基础 | 认证与权限覆盖率 | 0（无统一认证） | 100% API 有 JWT 保护 | Phase 1 上线时 |
| 部署便捷性 | 零依赖部署 | —（无现有系统） | 单二进制文件 + SQLite 可运行 | Phase 1 上线时 |
| 可视化管理 | 管理后台功能覆盖 | 0（无可视化界面） | Dashboard + 包列表 + 用户管理 | Phase 1 上线时 |

---

## 3. Non-Goals（不做的事）

Phase 1 MVP 明确不包含以下内容：

- ❌ **PyPI/Go/NuGet/YUM/APT/Generic 适配器** — Phase 2 扩展
- ❌ **供应链安全防护（Typosquatting/依赖混淆/Token 泄露扫描）** — v2.0 增强模块，Phase 2
- ❌ **离线环境支持、备份恢复、CAS 存储、多代理仓库、AI 辅助** — Phase 2+
- ❌ **S3/OSS 存储后端、PostgreSQL 生产数据库** — Phase 1 仅本地文件 + SQLite
- ❌ **发布审批工作流、版本不可变策略** — Phase 2

---

## 4. User Personas & Stories（用户画像与故事）

### Primary Persona

**张伟** — 中型企业 DevOps 工程师，200 人公司，负责 CI/CD 流水线和基础设施。每天需要确保构建环境稳定，处理开发者的依赖问题，向安全团队提供审计报告。

### Secondary Personas

**李娜** — Java 后端开发者，需要从内部仓库拉取公司共享库和公共 Maven 依赖  
**王磊** — 前端开发者，需要发布内部 npm 包并管理版本  
**陈刚** — 安全工程师，需要追踪依赖来源和审计日志

### 核心用户故事

---

**Story 1**: 作为 DevOps 工程师（张伟），我想要部署一个零外部依赖的私有组件仓库，以便在 5 分钟内完成安装并让团队立即开始使用。

**Acceptance Criteria**:
- [ ] Given 一台 Linux/macOS 服务器, when 执行单一二进制文件启动, then 服务在 30 秒内可用（端口 8080）
- [ ] Given SQLite 数据库不存在, when 服务首次启动, then 自动创建数据库和所有必要表结构
- [ ] Given 无外部网络连接, when 服务启动, then 管理后台和认证 API 正常工作
- [ ] Performance: 服务启动时间 < 5 秒

---

**Story 2**: 作为 DevOps 工程师（张伟），我想要配置 npm 和 Maven 代理仓库，以便团队成员可以从本地缓存加速拉取公共依赖。

**Acceptance Criteria**:
- [ ] Given 代理仓库已配置远程源 URL, when 开发者执行 `npm install`, then 包从本地缓存返回且速度提升 > 50%
- [ ] Given 包在本地缓存不存在, when 开发者首次请求包, then 从远程源拉取、缓存并返回给客户端
- [ ] Given 远程源不可用, when 开发者请求已缓存的包, then 正常返回缓存版本
- [ ] Given 远程源不可用, when 开发者请求未缓存的包, then 返回 503 错误和清晰提示
- [ ] Performance: 缓存命中请求 P99 延迟 < 50ms

---

**Story 3**: 作为 Java 开发者（李娜），我想要通过 Maven 协议发布和拉取公司内部共享库，以便在项目间统一依赖管理。

**Acceptance Criteria**:
- [ ] Given 已认证用户具有发布权限, when 通过 `mvn deploy` 上传 .jar + .pom 文件, then 包版本在仓库中正确创建
- [ ] Given 已认证用户具有拉取权限, when 通过 `mvn install` 拉取内部包, then 正确返回 .jar 和 .pom 文件
- [ ] Given 上传的包版本已存在, when 再次上传相同版本, then 返回 409 Conflict 错误
- [ ] Given 未认证用户, when 尝试拉取私有包, then 返回 401 Unauthorized 错误

---

**Story 4**: 作为前端开发者（王磊），我想要通过 npm 协议发布和拉取内部 npm 包，以便团队间共享前端组件库和工具。

**Acceptance Criteria**:
- [ ] Given 已认证用户具有发布权限, when 通过 `npm publish` 上传包, then 包在仓库中正确创建（含 metadata 和 tarball）
- [ ] Given 已认证用户, when 执行 `npm install @internal/component-lib`, then 正确安装包及其依赖
- [ ] Given 包名不符合命名规范, when 尝试发布, then 返回验证错误和规范提示
- [ ] Performance: npm install 缓存命中 < 100ms P99

---

**Story 5**: 作为安全工程师（陈刚），我想要通过 Web 管理后台查看包列表、用户活动和审计日志，以便进行安全审查和合规审计。

**Acceptance Criteria**:
- [ ] Given 管理员登录后台, when 查看 Dashboard, then 显示包总数、版本总数、下载量统计和最近活动
- [ ] Given 管理员登录后台, when 搜索包, then 支持按名称和类型筛选，结果在 1 秒内返回
- [ ] Given 管理员登录后台, when 查看审计日志, then 按时间倒序展示所有操作（上传、下载、删除、认证事件）
- [ ] Given 非管理员用户, when 访问管理后台, then 仅显示其权限范围内的数据

---

**Story 6**: 作为管理员，我想要创建用户并分配角色权限，以便控制不同团队对仓库的访问范围。

**Acceptance Criteria**:
- [ ] Given 管理员登录后台, when 创建新用户, then 用户可使用 username/password 登录
- [ ] Given 管理员分配角色, when 用户尝试操作, then 按角色权限允许或拒绝（admin/publisher/consumer/viewer）
- [ ] Given 用户登录, when 获取 JWT Token, then Token 包含正确的角色信息和过期时间
- [ ] Given Token 过期, when 用户请求 API, then 返回 401 并提示重新登录
