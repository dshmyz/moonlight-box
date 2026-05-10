#!/bin/bash

BASE="http://localhost:9081"
DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"

echo "========================================="
echo "从头测试各语言包版本号是否符合协议规则"
echo "========================================="
echo ""

echo "1. 测试NPM包（阿里云镜像）"
echo "-----------------------------------------"
echo "NPM协议: /{package}/-/{package}-{version}.tgz"
echo "测试包: lodash@4.17.21"
HTTP1=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/npm-proxy-cn/lodash/-/lodash-4.17.21.tgz")
echo "HTTP状态码: $HTTP1"

if [ "$HTTP1" = "200" ]; then
    echo "检查数据库中的版本号..."
    VERSION=$(sqlite3 $DB "SELECT pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.name = 'lodash' AND p.type = 'npm' ORDER BY pv.id DESC LIMIT 1")
    echo "数据库版本号: $VERSION"
    
    if [[ $VERSION == "4.17.21" ]]; then
        echo "✅ 版本号正确，符合NPM协议规范"
    else
        echo "❌ 版本号不正确，期望: 4.17.21，实际: $VERSION"
    fi
else
    echo "❌ 下载失败"
fi
echo ""

echo "2. 测试Maven包（阿里云镜像）"
echo "-----------------------------------------"
echo "Maven协议: /{groupId}/{artifactId}/{version}/{artifactId}-{version}.{ext}"
echo "测试包: com.google.guava:guava@32.1.3-jre"
HTTP2=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
echo "HTTP状态码: $HTTP2"

if [ "$HTTP2" = "200" ]; then
    echo "检查数据库中的版本号..."
    VERSION=$(sqlite3 $DB "SELECT pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.name = 'com.google.guava:guava' AND p.type = 'maven' ORDER BY pv.id DESC LIMIT 1")
    echo "数据库版本号: $VERSION"
    
    if [[ $VERSION == "32.1.3-jre" ]]; then
        echo "✅ 版本号正确，符合Maven协议规范"
    else
        echo "❌ 版本号不正确，期望: 32.1.3-jre，实际: $VERSION"
    fi
else
    echo "❌ 下载失败"
fi
echo ""

echo "3. 测试Go包（goproxy.cn）"
echo "-----------------------------------------"
echo "Go协议: /{module}/@v/{version}.zip"
echo "测试包: github.com/gin-gonic/gin@v1.9.1"
HTTP3=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/go-proxy-goproxy-cn/github.com/gin-gonic/gin/@v/v1.9.1.zip")
echo "HTTP状态码: $HTTP3"

if [ "$HTTP3" = "200" ]; then
    echo "检查数据库中的版本号..."
    VERSION=$(sqlite3 $DB "SELECT pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.name = 'github.com/gin-gonic/gin' AND p.type = 'go' ORDER BY pv.id DESC LIMIT 1")
    echo "数据库版本号: $VERSION"
    
    if [[ $VERSION == "v1.9.1" ]]; then
        echo "✅ 版本号正确，符合Go协议规范"
    else
        echo "❌ 版本号不正确，期望: v1.9.1，实际: $VERSION"
    fi
else
    echo "❌ 下载失败"
fi
echo ""

echo "========================================="
echo "测试完成"
echo "========================================="
