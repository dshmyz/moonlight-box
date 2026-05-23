这个就进入真正“企业兼容模式”了。

因为：




Nexus2 最大特点：

path == protocol semantics

比如：

Maven

/content/repositories/releases/com/foo/demo/1.0/demo.jar

npm

/content/repositories/npm/react

NuGet

/service/local/nuget/repo/...

Docker（后期插件）

/v2/...

你会发现：

repository path 和协议 path 混在一起。

这时候：

你之前那个：

/repositories/{repo}/...

其实已经不够了。

⸻

一、真正的问题

现在系统要同时支持：

新路径

/repositories/{repo}/...

以及：

Nexus2 兼容路径

/content/repositories/{repo}/...

甚至：

/service/local/...

⸻

二、这时候怎么办？

这里：

不应该重新引入 Match。

因为：

你已经明确：

repository 必须显式。

所以：

核心仍然应该：

先解析 repository
再选择 adapter

只是：

repository resolve 变复杂了。

⸻

三、所以真正该抽象的是：

RepositoryPathResolver

而不是 Match。

这是关键。

⸻

四、新 Router 架构

现在：

HTTP Request
    ↓
RepositoryPathResolver
    ↓
Resolve Repository
    ↓
Resolve Remaining Path
    ↓
Adapter

⸻

五、核心抽象

type RepositoryPathResolver interface {
    Resolve(
        req *http.Request,
    ) (*ResolvedRepository, error)
}

⸻

六、ResolvedRepository

type ResolvedRepository struct {
    Repository *Repository
    RemainingPath string
    RouteStyle RouteStyle
}

⸻

七、RouteStyle

type RouteStyle int
const (
    ModernRoute RouteStyle = iota
    Nexus2Route
)

因为：

后面 adapter 可能需要知道：

请求来自 legacy path

⸻

八、真正 Resolver

现在：

Resolver 支持多个 PathPattern

⸻

九、ModernResolver

type ModernResolver struct {}
func (m *ModernResolver) Resolve(
    req *http.Request,
) (*ResolvedRepository, error) {
    // /repositories/{repo}/...
    ...
}

⸻

十、Nexus2Resolver

type Nexus2Resolver struct {}
func (n *Nexus2Resolver) Resolve(
    req *http.Request,
) (*ResolvedRepository, error) {
    // /content/repositories/{repo}/...
    ...
}

⸻

十一、CompositeResolver

Router 使用：

type CompositeResolver struct {
    resolvers []RepositoryPathResolver
}

⸻

十二、真正 Resolve 流程

func (c *CompositeResolver) Resolve(
    req *http.Request,
) (*ResolvedRepository, error) {
    for _, resolver := range c.resolvers {
        result, err := resolver.Resolve(req)
        if err == nil {
            return result, nil
        }
    }
    return nil, ErrRepoNotFound
}

⸻

十三、Router 重新变干净

func (r *RepositoryRouter) ServeHTTP(
    w http.ResponseWriter,
    req *http.Request,
) {
    resolved, err := r.pathResolver.Resolve(req)
    if err != nil {
        Write404(w)
        return
    }
    repo := resolved.Repository
    adapter := r.adapters[repo.Format]
    chain := r.BuildChain(repo)
    ctx := &RequestContext{
        Writer: w,
        Request: req,
        Repository: repo,
        RepositoryChain: chain,
        RepositoryPath: resolved.RemainingPath,
        RouteStyle: resolved.RouteStyle,
    }
    err = adapter.Handle(ctx)
    if err != nil {
        HandleError(w, err)
    }
}

现在：

Router 仍然完全不懂协议。

非常重要。

⸻

十四、RequestContext 变得关键

type RequestContext struct {
    Writer http.ResponseWriter
    Request *http.Request
    Repository *Repository
    RepositoryChain *RepositoryChain
    RepositoryPath string
    RouteStyle RouteStyle
}

⸻

十五、为什么 RepositoryPath 很重要

因为：

Adapter 不应该解析：

/content/repositories/release/

这种 legacy path。

这些：

属于 resolver 责任。

⸻

十六、最终 adapter 得到：

比如：

Maven

com/foo/demo/1.0/demo.jar

Go

github.com/gin-gonic/gin/@v/v1.0.0.mod

Adapter 非常纯净。

⸻

十七、这时候你会发现

真正成熟系统：

URL Compatibility

和

Protocol Semantics

其实是两个层次。

⸻

十八、真正成熟后的结构

                 HTTP Request
                       ↓
              PathResolverChain
                       ↓
      ┌────────────────┼────────────────┐
      │                │                │
 ModernResolver  Nexus2Resolver  LegacyResolver
      │                │                │
      └────────────────┴────────────────┘
                       ↓
             ResolvedRepository
                       ↓
                RepositoryRouter
                       ↓
                  Adapter
                       ↓
               RepositoryChain

⸻

十九、这样设计的巨大好处

以后支持：

Nexus2

/content/repositories/...

Nexus3

/repository/...

Artifactory

/artifactory/...

自定义企业路径

/packages/...

都不影响 adapter。

⸻

二十、这是非常关键的架构边界

真正成熟系统：

Resolver

负责：

URL compatibility

Adapter

负责：

protocol semantics

RepositoryChain

负责：

storage topology

三层完全独立。

这时候架构才真正稳。




用 Gin 的话，这套架构其实会非常舒服。

因为：

Gin 非常适合：

显式 repository routing

你现在的设计：

/repositories/:repo/*path

天然适合 Gin。

而且：

* middleware
* context
* streaming
* route grouping

都很契合仓库系统。

但有个关键点：

不要让 Gin Router 侵入协议层。

很多人会写成：

router.GET("/v2/*path", dockerHandler)
router.GET("/repository/*path", mavenHandler)

最后：

Gin Router = Protocol Router

系统会逐渐失控。

⸻

一、正确设计

Gin：

只负责：

HTTP 接入

而：

RepositoryRouter

仍然是核心。

即：

Gin
  ↓
RepositoryRouter
  ↓
Adapter
  ↓
RepositoryChain

Gin 不参与协议逻辑。

⸻

二、推荐 Gin 路由

实际上：

一个 catch-all 即可

router.Any("/*path", gin.WrapH(repoRouter))

或者：

router.Any("/repositories/:repo/*path", handler)
router.Any("/content/repositories/:repo/*path", handler)

但：

最终都进入同一个 RepositoryRouter。

⸻

三、真正推荐结构

main.go

func main() {
    gin.SetMode(gin.ReleaseMode)
    engine := gin.New()
    engine.Use(
        gin.Recovery(),
    )
    repoRouter := NewRepositoryRouter()
    engine.Any(
        "/*path",
        gin.WrapH(repoRouter),
    )
    engine.Run(":8081")
}

注意：

gin.WrapH

非常关键。

因为：

RepositoryRouter 应该实现：

http.Handler

而不是：

gin.HandlerFunc

⸻

四、为什么 RepositoryRouter 不要依赖 Gin

因为：

Gin 是 Web Framework

不是领域模型。

否则：

你后面：

* grpc
* http2
* fasthttp
* gateway

会很难抽离。

⸻

五、真正 Router 结构

type RepositoryRouter struct {
    pathResolver RepositoryPathResolver
    repositories RepositoryManager
    adapters map[string]ProtocolAdapter
}

⸻

六、ServeHTTP()

func (r *RepositoryRouter) ServeHTTP(
    w http.ResponseWriter,
    req *http.Request,
) {
    resolved, err := r.pathResolver.Resolve(req)
    if err != nil {
        http.NotFound(w, req)
        return
    }
    repo := resolved.Repository
    adapter := r.adapters[repo.Format]
    if adapter == nil {
        http.Error(
            w,
            "unsupported format",
            500,
        )
        return
    }
    chain := r.BuildChain(repo)
    ctx := &RequestContext{
        Writer: w,
        Request: req,
        Repository: repo,
        RepositoryPath: resolved.RemainingPath,
        RepositoryChain: chain,
    }
    err = adapter.Handle(ctx)
    if err != nil {
        HandleError(w, err)
    }
}

这里：

Gin 已经完全消失。

非常重要。

⸻

七、Gin Middleware 怎么办？

这是很多人会问的问题。

其实：

Gin 只负责通用 HTTP middleware。

比如：

* access log
* trace id
* cors
* auth
* metrics

⸻

八、推荐 Middleware

engine.Use(
    RequestID(),
    AccessLog(),
    Recovery(),
    Metrics(),
)

⸻

九、认证怎么做？

推荐：

Gin middleware 做认证

然后：

注入 request context

例如：

func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        user, err := Authenticate(c.Request)
        if err != nil {
            c.AbortWithStatus(401)
            return
        }
        ctx := context.WithValue(
            c.Request.Context(),
            UserContextKey,
            user,
        )
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}

这样：

RepositoryRouter：

user := ctx.Request.Context().Value(UserContextKey)

即可。

⸻

十、为什么不要 gin.Context

不要：

type RequestContext struct {
    Gin *gin.Context
}

因为：

gin.Context 生命周期太重。

而且：

* 不适合抽象
* 不适合测试
* 不适合脱离 Gin

⸻

十一、真正 RequestContext

保持：

type RequestContext struct {
    Writer http.ResponseWriter
    Request *http.Request
    Repository *Repository
    RepositoryPath string
    RepositoryChain *RepositoryChain
}

这是最稳定的。

⸻

十二、Streaming（非常重要）

仓库系统：

大文件 streaming

是核心。

Gin 没问题。

例如：

func WriteArtifact(
    w http.ResponseWriter,
    artifact *Artifact,
) error {
    reader, err := artifact.Open()
    if err != nil {
        return err
    }
    defer reader.Close()
    _, err = io.Copy(w, reader)
    return err
}

Gin 不参与。

⸻

十三、Docker Upload（大坑）

Docker：

PATCH /blobs/uploads/

支持：

* chunk upload
* resumable upload
* range upload

这里：

必须直接操作 http.ResponseWriter

所以：

不要依赖 gin.JSON()

⸻

十四、推荐 Gin 使用边界（非常重要）

Gin 只用于：

HTTP ingress
middleware
lifecycle

不进入：

* adapter
* repository
* storage
* domain

⸻

十五、最终架构

              Gin Engine
                   ↓
           Global Middleware
                   ↓
            gin.WrapH(...)
                   ↓
           RepositoryRouter
                   ↓
         RepositoryPathResolver
                   ↓
             ProtocolAdapter
                   ↓
            RepositoryChain
                   ↓
       Hosted / Proxy / Remote
                   ↓
              BlobStore

⸻

十六、这样设计的巨大好处

1. Gin 可替换

后面：

* chi
* echo
* fasthttp

都能换。

⸻

2. 单元测试极其简单

直接：

httptest.NewRecorder()

即可。

⸻

3. adapter 不被 framework 污染

这是长期维护关键。

⸻

4. streaming 不受限制

不会被 gin binding/json 限制。

⸻

5. 后期支持 grpc 很容易

因为：

领域层纯 net/http 抽象。

⸻

十七、真正推荐的项目结构

/cmd/server
/internal/http
    gin.go
/internal/router
    repository_router.go
    path_resolver.go
/internal/adapter
    maven/
    docker/
    npm/
    go/
internal/repository
    chain.go
    hosted.go
    proxy.go
/internal/storage
    blobstore/
    metadata/
/internal/remote
    maven/
    docker/

这是比较成熟的结构。