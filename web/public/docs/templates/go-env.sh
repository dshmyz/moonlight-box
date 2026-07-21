#!/bin/bash
# Go 环境变量配置脚本
# 使用方法: source go-env.sh
# 也可将此文件内容复制到 ~/.bashrc 或 ~/.zshrc

# Go 模块代理配置
export GOPROXY=https://your-moonlight-domain/go,https://proxy.golang.org,direct

# 私有模块配置（多个用逗号分隔）
export GOPRIVATE=your-moonlight-domain,github.com/your-org

# 校验和数据库配置
# 当前版本不支持校验和数据库，请禁用
export GOSUMDB=off
# 或者使用官方校验和数据库（需要网络访问）
# export GOSUMDB=sum.golang.org

# Go 模块缓存
export GOMODCACHE=~/.go-mod-cache

# Go 代理超时
export GOPROXY_TIMEOUT=30s

echo "Go proxy environment variables set successfully"
echo "GOPROXY=$GOPROXY"
echo "GOPRIVATE=$GOPRIVATE"
echo "GOSUMDB=$GOSUMDB"

# 使用说明：
# 1. 下载公共模块：go get example.com/package@latest
# 2. 下载私有模块：GOPRIVATE=your-moonlight-domain go get your-moonlight-domain/module@latest
# 3. 验证配置：go env GOPROXY
