# 代理仓库超时时间与高级配置设计

## 背景

当前代理仓库的配置项较少，缺少超时时间、失败缓存策略、重定向处理、证书校验等高级配置能力。需要为代理仓库增加更多可配置参数，提升灵活性和稳定性。

## 目标

1. 支持全局默认超时 + 仓库级别超时覆盖
2. 支持大文件的流式处理，避免一次性加载到内存
3. 支持自定义失败缓存策略（可配置哪些状态码缓存及缓存时长）
4. 支持重定向处理策略配置
5. 支持 SSL 证书校验开关配置
6. 验证并优化现有连接池配置

## 架构设计

### 配置层级

```
全局配置 (defaults.go)
  └─ proxy.default_timeout: 30s           # 默认读取超时
  └─ proxy.connect_timeout: 10s           # 连接超时
  └─ proxy.large_file_threshold: 50MB     # 大文件判断阈值
  └─ proxy.max_redirects: 10              # 默认最大重定向次数
  └─ proxy.insecure_skip_verify: false    # 默认不跳过证书校验

仓库模型 (repository.go) — 新增代理高级配置字段
  └─ timeout_seconds: int                 # 超时时间，0=使用全局默认
  └─ max_redirects: int                   # 最大重定向次数，0=使用全局默认，-1=不跟随
  └─ insecure_skip_verify: bool           # 是否跳过SSL证书校验
  └─ failure_cache_rules: string (JSON)   # 失败缓存规则，JSON数组
```

### Repository 模型新增字段

```go
// 代理高级配置（仅 proxy 类型仓库使用）
TimeoutSeconds      int    `json:"timeout_seconds" gorm:"default:0"`
MaxRedirects        int    `json:"max_redirects" gorm:"default:0"`
InsecureSkipVerify  bool   `json:"insecure_skip_verify" gorm:"default:false"`
FailureCacheRules   string `json:"failure_cache_rules" gorm:"type:text"`
```

#### 失败缓存规则格式

```json
[
  { "status_code": 404, "ttl_seconds": 300 },
  { "status_code_range": [500, 599], "ttl_seconds": 60 },
  { "status_code": 403, "ttl_seconds": 600 }
]
```

- `status_code`: 精确匹配状态码
- `status_code_range`: 范围匹配 [start, end]
- `ttl_seconds`: 缓存时长
- 按数组顺序匹配，第一条命中即生效
- 空数组表示不缓存任何失败响应

### 组件改动

#### 1. RemoteClient (internal/proxy/client.go)

- `http.Transport` 改为支持按仓库配置动态创建（主要差异是 TLS 配置）
- 新增 `DialContext` 配置控制 TCP 连接超时
- `Get` 方法改为接受 `RequestOptions` 参数，包含超时、重定向、证书等配置
- 新增 `GetStream` 方法，返回 `io.ReadCloser`，支持流式读取
- 超时通过 `context.WithTimeout` 实现
- 重定向通过 `http.Client.CheckRedirect` 控制

```go
type RequestOptions struct {
    ConnectTimeout     time.Duration
    ReadTimeout        time.Duration
    MaxRedirects       int    // -1 表示不跟随
    InsecureSkipVerify bool
}

func (c *RemoteClient) GetStream(ctx context.Context, url string, opts RequestOptions, auth *ProxyAuthConfig) (io.ReadCloser, string, error)
```

#### 2. ProxyRouter (internal/proxy/router.go)

- `resolveProxy` 改为流式处理
- 根据 `Content-Length` 预判大文件（>50MB），大文件动态延长 readTimeout
- 使用 `io.TeeReader` 边读边写缓存
- 失败响应根据 `FailureCacheRules` 决定是否缓存及缓存时长

#### 3. API Handler (internal/handler/repository_handler.go)

- `Create` 和 `Update` 接口支持新增的配置参数

#### 4. 全局配置 (internal/config/defaults.go)

- 新增 proxy 相关默认配置项

### 数据流

#### 创建仓库

```json
POST /api/repositories
{
  "name": "maven-central",
  "type": "proxy",
  "remote_url": "https://repo.maven.apache.org/maven2",
  "timeout_seconds": 60,
  "max_redirects": 5,
  "insecure_skip_verify": false,
  "failure_cache_rules": [
    { "status_code": 404, "ttl_seconds": 300 },
    { "status_code_range": [500, 599], "ttl_seconds": 60 }
  ]
}
```

#### 代理请求

```
1. ProxyRouter.Resolve 获取仓库配置
2. 计算实际超时:
   if repo.timeout_seconds > 0: 使用仓库值
   else: 使用全局默认值
3. 检查 Content-Length 判断是否大文件:
   if size > 50MB: readTimeout *= 2
4. 构建 RequestOptions:
   - ConnectTimeout: 全局 connect_timeout
   - ReadTimeout: 计算后的超时
   - MaxRedirects: repo.max_redirects 或全局默认
   - InsecureSkipVerify: repo.insecure_skip_verify
5. context.WithTimeout 设置超时
6. RemoteClient.GetStream 发起流式请求
7. io.TeeReader 边读边写缓存
8. 返回 RouteResult（流式 Content）

失败处理:
- 响应状态码匹配 FailureCacheRules 时，写入负向缓存
- 缓存 key: "proxy_error:{repo}:{path}:{status_code}"
```

### 连接池优化结论

现有配置已足够：
- `MaxIdleConns: 100`
- `MaxIdleConnsPerHost: 20`
- `IdleConnTimeout: 90s`

不需要额外限流措施，原因：
1. 已有连接池复用机制
2. 代理请求都是短连接
3. 缓存机制减少重复请求
4. 并发控制应在 HTTP server 层而非客户端

### Transport 管理

由于 `InsecureSkipVerify` 不同时需要不同的 Transport，采用延迟初始化策略：

```go
type TransportManager struct {
    secureTransport   *http.Transport    // 默认 Transport
    insecureTransport *http.Transport    // 跳过证书校验的 Transport
}

func (m *TransportManager) GetTransport(insecure bool) *http.Transport
```

- 应用启动时初始化两个 Transport
- 根据仓库配置选择使用哪个 Transport
- 避免每次请求都创建新的 Transport

### 错误处理

- 区分连接超时（`context.DeadlineExceeded` during dial）和读取超时
- 重定向次数超限返回明确错误
- 证书校验失败返回明确错误
- 404 负向缓存逻辑兼容现有 FailureCacheRules
- 流式中断时清理部分写入的缓存

### 前端改动

前端表单采用分组设计，根据仓库类型动态显示对应配置区域。

#### 表单分组

**基础信息（所有类型）**
- 仓库名称（必填，英文+连字符）
- 显示名称
- 描述
- 类型（local/proxy/virtual）
- 包类型（npm/maven2）

**代理配置（仅 proxy 类型显示）**
- 远程地址（必填）
- 认证类型：下拉选择 none/basic/bearer/api_key
  - 选择 basic 时显示用户名/密码输入
  - 选择 bearer 时显示 token 输入
  - 选择 api_key 时显示 header name/key value 输入
- 优先级：数字输入，默认 0

**超时与连接（仅 proxy 类型显示）**
- 超时时间（秒）：数字输入，默认 0（使用全局默认 30s）
- 最大重定向次数：数字输入，默认 0（使用全局默认 10），支持 -1（不跟随）
- 跳过证书校验：开关，默认关闭

**缓存配置（所有类型，proxy 类型额外显示失败缓存规则）**
- 启用缓存：开关，默认开启
- 缓存 TTL（秒）：数字输入，默认 86400（24小时）
- 负向缓存 TTL（秒）：数字输入，默认 300（仅 proxy）
- 缓存最大大小（GB）：数字输入，默认 10
- 失败缓存规则：可视化表单 + JSON 模式切换（仅 proxy）
  - 可视化模式：规则列表，每行可添加/删除，支持状态码精确匹配或范围匹配，设置 TTL
  - JSON 模式：JSON textarea，支持格式化校验

**权限控制（local 和 proxy 类型）**
- 允许覆盖：开关，默认关闭
- 允许删除：开关，默认关闭

**虚拟仓成员（仅 virtual 类型显示）**
- 成员仓库选择：多选下拉，从已有 local/proxy 仓库中选择
- 成员优先级：拖拽排序或数字设置
