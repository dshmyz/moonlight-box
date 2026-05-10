#!/bin/bash

BASE="http://localhost:9081"
DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"

echo "========================================="
echo "测试各语言包版本号是否符合协议规则"
echo "========================================="
echo ""

echo "1. 测试NPM包（阿里云镜像）"
echo "-----------------------------------------"
echo "NPM使用语义化版本（Semantic Versioning）: MAJOR.MINOR.PATCH"
echo "测试包: lodash@4.17.21"
HTTP1=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/npm-proxy-cn/lodash/-/lodash-4.17.21.tgz")
echo "HTTP状态码: $HTTP1"

if [ "$HTTP1" = "200" ]; then
    echo "检查数据库中的版本号..."
    VERSION=$(sqlite3 $DB "SELECT version FROM package_versions WHERE package_id = (SELECT id FROM packages WHERE name = 'lodash') ORDER BY id DESC LIMIT 1")
    echo "数据库版本号: $VERSION"
    
    if [[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "✅ 版本号符合NPM语义化版本规范"
    else
        echo "❌ 版本号不符合NPM语义化版本规范"
    fi
else
    echo "❌ 下载失败"
fi
echo ""

echo "2. 测试Maven包（阿里云镜像）"
echo "-----------------------------------------"
echo "Maven版本号通常为: MAJOR.MINOR.PATCH-QUALIFIER"
echo "测试包: com.google.guava:guava:32.1.3-jre"
HTTP2=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
echo "HTTP状态码: $HTTP2"

if [ "$HTTP2" = "200" ]; then
    echo "检查数据库中的版本号..."
    VERSION=$(sqlite3 $DB "SELECT version FROM package_versions WHERE package_id = (SELECT id FROM packages WHERE name = 'com.google.guava:guava') ORDER BY id DESC LIMIT 1")
    echo "数据库版本号: $VERSION"
    
    if [[ $VERSION == "32.1.3-jre" ]]; then
        echo "✅ 版本号符合Maven版本规范"
    else
        echo "❌ 版本号不符合Maven版本规范，期望: 32.1.3-jre，实际: $VERSION"
    fi
else
    echo "❌ 下载失败"
fi
echo ""

echo "3. 测试PyPI包（清华镜像）"
echo "-----------------------------------------"
echo "PyPI使用PEP 440版本规范: MAJOR.MINOR.PATCH"
echo "测试包: requests@2.31.0"
echo "先获取包文件列表..."
HTML=$(curl -s "${BASE}/repo/pypi-proxy-tuna/simple/requests/")
echo "解析文件URL..."

FILE_URL=$(echo "$HTML" | grep -o 'href="[^"]*requests-2.31.0[^"]*\.whl[^"]*"' | head -1 | sed 's/href="//;s/"//')
if [ -z "$FILE_URL" ]; then
    echo "❌ 无法找到requests-2.31.0的wheel文件"
else
    echo "找到文件: $FILE_URL"
    
    if [[ $FILE_URL == ../../packages/* ]]; then
        FILE_URL="${BASE}/repo/pypi-proxy-tuna/packages/${FILE_URL#../../packages/}"
    elif [[ $FILE_URL == /packages/* ]]; then
        FILE_URL="${BASE}/repo/pypi-proxy-tuna/packages/${FILE_URL#/packages/}"
    fi
    
    echo "下载URL: $FILE_URL"
    HTTP3=$(curl -s -o /dev/null -w "%{http_code}" "$FILE_URL")
    echo "HTTP状态码: $HTTP3"
    
    if [ "$HTTP3" = "200" ]; then
        echo "检查数据库中的版本号..."
        VERSION=$(sqlite3 $DB "SELECT version FROM package_versions WHERE package_id = (SELECT id FROM packages WHERE name = 'requests') ORDER BY id DESC LIMIT 1")
        echo "数据库版本号: $VERSION"
        
        if [[ $VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "✅ 版本号符合PyPI PEP 440版本规范"
        else
            echo "❌ 版本号不符合PyPI PEP 440版本规范"
        fi
    else
        echo "❌ 下载失败"
    fi
fi
echo ""

echo "4. 测试Go包（goproxy.cn）"
echo "-----------------------------------------"
echo "Go使用语义化版本: vMAJOR.MINOR.PATCH"
echo "测试包: github.com/gin-gonic/gin@v1.9.1"
HTTP4=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/go-proxy-goproxy-cn/github.com/gin-gonic/gin/@v/v1.9.1.zip")
echo "HTTP状态码: $HTTP4"

if [ "$HTTP4" = "200" ]; then
    echo "检查数据库中的版本号..."
    VERSION=$(sqlite3 $DB "SELECT version FROM package_versions WHERE package_id = (SELECT id FROM packages WHERE name = 'github.com/gin-gonic/gin') ORDER BY id DESC LIMIT 1")
    echo "数据库版本号: $VERSION"
    
    if [[ $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo "✅ 版本号符合Go语义化版本规范"
    else
        echo "❌ 版本号不符合Go语义化版本规范"
    fi
else
    echo "❌ 下载失败"
fi
echo ""

echo "========================================="
echo "测试完成"
echo "========================================="
