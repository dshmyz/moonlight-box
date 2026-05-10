# 版本号提取最佳实践

## 原则
1. **信任远程仓库的URL结构**（远程仓库遵循协议规范）
2. **从协议规定的位置提取版本号**（而不是自己解析）
3. **使用生态系统标准库验证版本号**（而不是自己写正则表达式）

## 各语言生态系统的正确做法

### Go模块
**协议**: GOPROXY协议
**URL格式**: `/$module/@v/$version.zip`
**提取方式**: 从URL路径提取
**示例**: 
- URL: `/github.com/gin-gonic/gin/@v/v1.9.1.zip`
- 版本号: `v1.9.1` (直接从URL提取)

**代码**:
```go
// 从URL路径提取版本号
version := strings.TrimSuffix(path.Base(urlPath), ".zip")
// version = "v1.9.1"
```

### NPM
**协议**: NPM Registry API
**URL格式**: `/$package/-/$package-$version.tgz`
**提取方式**: 从URL路径提取包名和文件名，从文件名提取版本号
**示例**:
- URL: `/lodash/-/lodash-4.17.21.tgz`
- 包名: `lodash` (从URL路径提取)
- 文件名: `lodash-4.17.21.tgz`
- 版本号: `4.17.21` (从文件名提取：移除包名前缀和.tgz后缀)

**代码**:
```go
// 从URL路径提取包名和文件名
// URL: /lodash/-/lodash-4.17.21.tgz
parts := strings.SplitN(fullPath, "/-/", 2)
pkgName := parts[0]  // "lodash"
filename := parts[1] // "lodash-4.17.21.tgz"

// 从文件名提取版本号
// 文件名格式: {package}-{version}.tgz
filename = strings.TrimSuffix(filename, ".tgz")
version := strings.TrimPrefix(filename, pkgName + "-")
// version = "4.17.21"
```

### Maven
**协议**: Maven Repository Layout
**URL格式**: `/$groupId/$artifactId/$version/$artifactId-$version.$extension`
**提取方式**: 从URL路径提取
**示例**:
- URL: `/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom`
- 版本号: `32.1.3-jre` (从路径提取)

**代码**:
```go
// 从URL路径提取版本号
// 路径格式: /groupId/artifactId/version/filename
parts := strings.Split(fullPath, "/")
version := parts[len(parts)-2]
// version = "32.1.3-jre"
```

### PyPI
**协议**: PEP 503 - Simple Repository API
**URL格式**: `/packages/$hash/$filename`
**提取方式**: 从Simple API的HTML响应中提取版本号
**示例**:
- Simple API: `/simple/requests/`
- HTML响应: `<a href="../../packages/70/8e/.../requests-2.31.0-py3-none-any.whl#sha256=...">requests-2.31.0-py3-none-any.whl</a>`
- 版本号: `2.31.0` (从链接文本或href中提取)

**代码**:
```go
// 从Simple API的HTML响应中提取版本号
// 解析HTML，找到对应的链接
// 从链接文本中提取版本号
// 例如: "requests-2.31.0-py3-none-any.whl" -> "2.31.0"
```

## 特殊情况处理

### 作用域NPM包
**URL**: `/@babel/core/-/core-7.12.3.tgz`
**包名**: `@babel/core`
**版本号**: `7.12.3`

### Maven SNAPSHOT版本
**URL**: `/com/example/my-app/1.0-SNAPSHOT/my-app-1.0-SNAPSHOT.jar`
**版本号**: `1.0-SNAPSHOT`

### PyPI wheel文件
**文件名**: `package-1.0.0-py3-none-any.whl`
**版本号**: `1.0.0`

## 总结

关键点：
1. **Go**: 从URL路径直接提取 ✅
2. **NPM**: 从文件名提取，但要使用包名作为前缀 ✅
3. **Maven**: 从URL路径直接提取 ✅
4. **PyPI**: 从Simple API的HTML响应中提取 ⚠️ 需要改进

不要：
1. ❌ 使用硬编码的正则表达式解析版本号
2. ❌ 自己推测版本号格式
3. ❌ 忽略协议规范

要：
1. ✅ 遵循协议规范
2. ✅ 从协议规定的位置提取版本号
3. ✅ 信任远程仓库的URL结构
