---
name: "backend-git-workflow"
description: "提炼自 qim-server 的 Go 后端代码规范。开发 Java/Go 后端时使用，包含项目结构、命名、错误处理、数据库、API 设计等规范。"
---

# Backend Code Standards (Go/Java Backend)

基于 qim-server 项目提炼的后端开发规范，适用于 Java/Go 后端服务开发。

## 项目结构规范

```
qim-server/
├── handler/          # HTTP 处理器（类似 Controller）
├── service/          # 业务逻辑层
├── model/            # 数据模型定义
├── middleware/       # 中间件
├── pkg/
│   ├── errors/      # 自定义错误定义
│   └── response/    # 统一响应封装
├── database/         # 数据库初始化和连接
├── cache/            # 缓存实现
├── config/           # 配置加载
├── app/              # 应用初始化、路由设置
├── ws/               # WebSocket 实现
├── ai/               # AI 服务集成
├── main.go           # 入口文件
└── config.yaml       # 配置文件
```

### 分层职责

| 层级 | 职责 | 命名示例 |
|------|------|---------|
| `handler` | 处理 HTTP 请求/响应、参数校验、权限检查 | `GetUser`, `UpdateUser`, `CreateUser` |
| `service` | 业务逻辑、数据处理、缓存管理 | `UserService`, `GetUser()`, `UpdateUserStatus()` |
| `model` | 数据结构定义、ORM 映射 | `User`, `Conversation`, `Message` |
| `middleware` | 认证、权限、日志等横切关注点 | `AuthMiddleware`, `RequireRole` |

## 命名规范

### 文件命名

```go
// Go: 小写下划线或驼峰
user_handler.go
userService.go
conversation_handler.go

// Java: 驼峰
UserHandler.java
UserService.java
```

### 函数命名

```go
// Handler: 动作 + 资源名 (PascalCase)
func GetUser(c *gin.Context) {}
func UpdateUser(c *gin.Context) {}
func CreateUser(c *gin.Context) {}
func DeleteUser(c *gin.Context) {}
func SearchUsers(c *gin.Context) {}

// Service: 动词 + 名词或名词 (PascalCase)
func (s *UserService) GetUser(userID uint) (*model.User, error) {}
func (s *UserService) UpdateUserStatus(userID uint, status string) error {}
func (s *UserService) SearchUsers(keyword string, limit int) ([]model.User, error) {}
```

### 变量命名

```go
// 通用变量: 驼峰或短名
userID, convID, msgID, db, err

// 数据库模型实例
var user model.User
var users []model.User

// 请求结构体
var req struct {
    Username string `json:"username" binding:"required"`
    Password string `json:"password" binding:"required"`
}
```

## Model 规范

### Go Model 定义

```go
type User struct {
    ID               uint           `json:"id" gorm:"primarykey"`
    Username         string         `json:"username" gorm:"uniqueIndex;size:50;not null"`
    PasswordHash     string         `json:"-" gorm:"size:255;not null"`
    Nickname         string         `json:"nickname" gorm:"size:100"`
    Avatar           string         `json:"avatar" gorm:"size:500"`
    Signature        string         `json:"signature" gorm:"type:text"`
    Status           string         `json:"status" gorm:"size:20;default:'offline'"`
    CreatedAt        time.Time      `json:"created_at"`
    UpdatedAt        time.Time      `json:"updated_at"`
    DeletedAt        gorm.DeletedAt `json:"-" gorm:"index"`
}
```

### 规范要点

- `json:"-"` 用于敏感字段（如 PasswordHash）
- 使用 `gorm.DeletedAt` 实现软删除
- 关联字段使用 `gorm:"foreignkey:XXX"`
- 索引使用 `uniqueIndex` 或 `index`

## Handler 规范

### 标准函数签名

```go
func HandlerName(c *gin.Context) {
    // 1. 获取上下文参数
    userID, _ := c.Get("user_id")

    // 2. 绑定并校验请求参数
    var req struct {
        Nickname string `json:"nickname"`
        Avatar   string `json:"avatar"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, "参数错误")
        return
    }

    // 3. 获取数据库连接
    db := database.GetDB()

    // 4. 执行业务逻辑
    // ...

    // 5. 返回响应
    response.Success(c, data)
}
```

### 错误处理模式

```go
// 数据库查询错误
if err := db.First(&user, userID).Error; err != nil {
    if err == gorm.ErrRecordNotFound {
        response.NotFound(c, "用户不存在")
    } else {
        response.InternalServerError(c, "查询失败")
    }
    return
}

// 业务错误
if count > 0 {
    response.BadRequest(c, "用户名已存在")
    return
}

// 创建/更新失败
if err := db.Create(&user).Error; err != nil {
    response.InternalServerError(c, "创建用户失败")
    return
}
```

## Service 规范

### Service 结构体定义

```go
type UserService struct{}

func NewUserService() *UserService {
    return &UserService{}
}
```

### 错误定义

```go
// 定义业务错误
var ErrUserNotFound = errors.New("user not found")

// 在 Service 中返回
func (s *UserService) GetUser(userID uint) (*model.User, error) {
    db := database.GetDB()
    var user model.User
    if err := db.First(&user, userID).Error; err != nil {
        return nil, ErrUserNotFound
    }
    return &user, nil
}
```

### 缓存使用

```go
func (s *UserService) GetUser(userID uint) (*model.User, error) {
    cacheKey := fmt.Sprintf("user:%d", userID)

    // 读缓存
    if data, ok := cache.UserCache.Get(cacheKey); ok {
        if jsonData, ok := data.([]byte); ok {
            var user model.User
            if err := json.Unmarshal(jsonData, &user); err == nil {
                return &user, nil
            }
        }
    }

    // 读数据库
    db := database.GetDB()
    var user model.User
    if err := db.First(&user, userID).Error; err != nil {
        return nil, ErrUserNotFound
    }

    // 写缓存
    if jsonData, err := json.Marshal(&user); err == nil {
        cache.UserCache.Put(cacheKey, jsonData)
    }

    return &user, nil
}
```

## Response 规范

### 统一响应封装

```go
// pkg/response/response.go
func Success(c *gin.Context, data interface{}) {
    c.JSON(http.StatusOK, gin.H{
        "code":    0,
        "message": "success",
        "data":    data,
    })
}

func BadRequest(c *gin.Context, message string) {
    Error(c, http.StatusBadRequest, 400, message)
}

func NotFound(c *gin.Context, message string) {
    Error(c, http.StatusNotFound, 404, message)
}

func InternalServerError(c *gin.Context, message string) {
    Error(c, http.StatusInternalServerError, 500, message)
}
```

### Handler 中使用

```go
response.Success(c, gin.H{
    "id":       user.ID,
    "username": user.Username,
    "nickname": user.Nickname,
})

response.BadRequest(c, "参数错误")
response.NotFound(c, "用户不存在")
response.InternalServerError(c, "服务器内部错误")
```

## 错误处理规范

### 自定义错误包

```go
// pkg/errors/errors.go
var (
    ErrBadRequest        = NewAppError(400, "请求参数错误")
    ErrUnauthorized      = NewAppError(401, "未授权")
    ErrForbidden         = NewAppError(403, "禁止访问")
    ErrNotFound          = NewAppError(404, "资源不存在")
    ErrConflict          = NewAppError(409, "资源冲突")
    ErrInternalServer    = NewAppError(500, "服务器内部错误")
)

type AppError struct {
    Code    int
    Message string
    Err     error
}
```

### 错误处理原则

1. **尽早返回错误**: 错误条件使用 early return
2. **不吞掉错误**: 必须处理或返回错误
3. **日志记录**: 关键操作失败时记录日志
4. **用户友好消息**: 对外返回中文提示

## 数据库规范

### 连接管理

```go
// database/database.go
var DB *gorm.DB

func Init(cfg *config.Config) *gorm.DB {
    // ... 初始化逻辑
    return DB
}

func GetDB() *gorm.DB {
    return DB
}
```

### 查询规范

```go
// 条件查询
db.Where("nickname LIKE ? OR username LIKE ?", "%"+query+"%", "%"+query+"%")

// 分页
query.Limit(pageSize).Offset(offset)

// 预加载关联
db.Preload("Sender").Preload("QuotedMessage")

// 更新
db.Model(&user).Updates(updates)

// 软删除
db.Delete(&user) // 使用 gorm.DeletedAt
```

## 路由规范

### 分组路由

```go
// app/routes.go
func SetupRoutes(r *gin.Engine) {
    api := r.Group("/api/v1")
    {
        // 公开路由
        auth := api.Group("/auth")
        {
            auth.POST("/login", handler.Login)
            auth.POST("/register", handler.Register)
        }

        // 需要认证的路由
        authed := api.Group("")
        authed.Use(middleware.AuthMiddleware(cfg.JWT.Secret))
        {
            authed.GET("/users/me", handler.GetCurrentUser)
            authed.PUT("/users/me", handler.UpdateUser)
        }
    }
}
```

### RESTful 风格

| 方法 | 路径 |  Handler | 说明 |
|------|------|----------|------|
| GET | /users/me | GetCurrentUser | 获取当前用户 |
| PUT | /users/me | UpdateUser | 更新当前用户 |
| GET | /users/search | SearchUsers | 搜索用户 |
| POST | /users | CreateUser | 创建用户 |
| DELETE | /conversations/:id | DeleteConversation | 删除会话 |

## Middleware 规范

### AuthMiddleware

```go
func AuthMiddleware(secret string) gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        // ... 验证逻辑

        if err != nil || !token.Valid {
            c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "认证令牌无效"})
            c.Abort()
            return
        }

        c.Set("user_id", claims.UserID)
        c.Set("username", claims.Username)
        c.Next()
    }
}
```

### RequireRole

```go
func RequireRole(roles ...string) gin.HandlerFunc {
    return func(c *gin.Context) {
        // 检查用户角色
        // ...
        if !hasRole {
            c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "无权限操作"})
            c.Abort()
            return
        }
        c.Next()
    }
}
```

## 配置规范

### 配置结构

```go
type Config struct {
    Server   ServerConfig
    Database DatabaseConfig
    JWT      JWTConfig
    Cluster  ClusterConfig
    Storage  StorageConfig
    AI       ai.AIConfig
}
```

### 环境变量覆盖

```go
port := os.Getenv("PORT")
if port != "" {
    cfg.Server.Port = port
}
```

## Git 提交规范

### Commit Message 格式

```
<type>: <subject>

<body>
```

### Type 类型

- `feat`: 新功能
- `fix`: 修复 bug
- `docs`: 文档更新
- `style`: 代码格式（不影响功能）
- `refactor`: 重构
- `perf`: 性能优化
- `test`: 测试相关
- `chore`: 构建/工具相关

### 示例

```
feat: 添加用户搜索功能

- 支持按昵称和用户名搜索
- 返回结果包含用户基本信息

closes #123
```

## 注意事项

1. **无魔法数字**: 使用常量或配置
2. **日志记录**: 关键操作添加日志
3. **参数校验**: 所有输入必须校验
4. **安全第一**: 敏感数据不返回前端
5. **前后一致**: 遵循项目现有风格
