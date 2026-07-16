# 阻断执行与审计一致性设计

## 目标

让代理、本地和虚拟仓库中的包阻断遵循同一规则，并将每一次实际阻断以可查询、可统计的 `block` 审计动作保存。

## 范围

- `HostedRuntime` 接收并执行 `PackageBlocker`，在读取具体 artifact 和按包名/版本查询时拒绝命中规则的请求。
- 运行时初始化向本地仓库及虚拟仓库的本地成员注入同一个阻断器。
- 路由层对预检命中和 Plugin 返回 `ErrBlocked` 两条路径，都写入 `action=block`、HTTP 403 的审计记录。
- 审计适配器映射阻断事件为 `model.ActionBlock`，其余下载事件保持 `model.ActionPackageDownload`。
- 回归测试覆盖阻断检查、阻断审计动作及日志/统计的查询口径。

## 数据流

协议插件解析出 ArtifactKey 或 ArtifactQuery 后调用仓库运行时。代理与本地运行时均在其可获得的包名和版本上执行 `PackageBlocker`；虚拟仓库沿用成员运行时的结果。任何命中最终回到 `RepositoryRouter` 的 `ErrBlocked` 分支，该分支写入审计记录并返回 403。对路由预检直接命中的请求，路由器同样写入 `block` 记录后返回 403。

## 错误与兼容性

阻断服务查询错误继续保持当前的 fail-open 行为，不把规则数据库暂时不可用变成所有包下载不可用。条件元数据无法验证的请求仍记录为 `condition_unverified` 并放行；它不是阻断记录。现有下载日志的动作不改变。

## 验收标准

1. 已启用的精确规则会阻止 HostedRuntime 对匹配 ArtifactKey 的读取。
2. 已启用的精确规则会阻止 HostedRuntime 对匹配 ArtifactQuery 的查询。
3. 路由预检命中与运行时 `ErrBlocked` 均写入 `action=block`、状态 403。
4. `GET /block-rules/logs` 与阻断统计查询能读取这些记录。
