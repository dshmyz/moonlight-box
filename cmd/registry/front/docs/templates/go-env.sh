#!/bin/bash
# Go 环境变量配置脚本
# 使用方法: source go-env.sh

export GOPROXY=https://your-moonlight-domain/go,https://proxy.golang.org,direct
export GOPRIVATE=your-moonlight-domain
export GOSUMDB=off

echo "Go proxy environment variables set successfully"
echo "GOPROXY=$GOPROXY"
echo "GOPRIVATE=$GOPRIVATE"
echo "GOSUMDB=$GOSUMDB"
