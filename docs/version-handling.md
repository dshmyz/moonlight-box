# 版本号处理策略

## 问题
不应该硬编码规则来匹配版本号，应该遵循各语言生态系统的协议规范。

## 当前实现分析

### 1. Go模块
**协议**: GOPROXY协议 (https://go.dev/ref/mod#goproxy-protocol)
**版本号来源**: URL路径 `/$module/@v/$version.zip`
**是否正确**: ✅ 正确，版本号在URL中明确指定

### 2. NPM
**协议**: NPM Registry API (https://github.com/npm/registry/blob/main/docs/REGISTRY-API.md)
**版本号来源**: 
- 元数据API: `GET /{package}` 返回JSON，包含 `versions["version"].dist.tarball`
- Tarball下载: `GET /{package}/-/{package}-{version}.tgz`

**问题**: 当前从文件名解析版本号，应该从元数据中获取

**正确做法**:
1. 先获取包元数据
2. 从元数据中找到对应版本的tarball URL
3. 从元数据中提取正确的版本号

### 3. Maven
**协议**: Maven Repository Layout (https://cwiki.apache.org/confluence/display/MAVENOLD/Repository+Layout+-+Final)
**版本号来源**: 路径结构 `/$groupId/$artifactId/$version/$artifactId-$version.$extension`
**是否正确**: ✅ 正确，版本号在路径中明确指定

### 4. PyPI
**协议**: PEP 503 - Simple Repository API (https://peps.python.org/pep-0503/)
**版本号来源**:
- Simple API: `GET /simple/{project}/` 返回HTML，包含所有版本链接
- 文件下载: 链接中包含版本号

**问题**: 当前从文件名解析版本号，格式多种多样

**正确做法**:
1. 先获取Simple API响应
2. 从HTML中解析出所有版本链接
3. 从链接中提取版本号

## 改进方案

### 方案1: 从元数据中获取版本号（推荐）
- 优点: 最准确，遵循协议
- 缺点: 需要额外的HTTP请求

### 方案2: 信任远程仓库的URL结构
- 优点: 简单，无需额外请求
- 缺点: 依赖URL结构的稳定性

### 方案3: 使用生态系统标准库验证版本号
- 优点: 确保版本号格式正确
- 缺点: 仍然需要从某处提取版本号

## 建议

对于代理仓库，最安全的做法是：
1. **Go**: 直接从URL路径提取版本号（已正确）
2. **NPM**: 从元数据API获取版本号（需要改进）
3. **Maven**: 直接从路径提取版本号（已正确）
4. **PyPI**: 从Simple API获取版本号（需要改进）

对于本地仓库，版本号由用户上传时指定，应该信任用户的输入。
