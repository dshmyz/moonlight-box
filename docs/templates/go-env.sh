# Go 环境变量配置模板
# 将此文件复制到 ~/.bashrc 或 ~/.zshrc

# Go 模块代理配置
export GOPROXY=http://your-registry:9081/go,https://proxy.golang.org,direct

# 私有模块配置
export GOPRIVATE=your-registry,github.com/your-org

# 校验和数据库配置
# 当前版本不支持校验和数据库，请禁用
export GOSUMDB=off

# 或者使用官方校验和数据库（需要网络访问）
# export GOSUMDB=sum.golang.org

# Go 模块缓存
export GOMODCACHE=~/.go-mod-cache

# Go 代理超时
export GOPROXY_TIMEOUT=30s

# 使用说明：
# 1. 下载公共模块：go get example.com/package@latest
# 2. 下载私有模块：GOPRIVATE=your-registry go get your-registry/module@latest
# 3. 验证配置：go env GOPROXY
