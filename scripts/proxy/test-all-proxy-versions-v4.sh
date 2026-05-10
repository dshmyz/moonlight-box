#!/bin/bash
set -euo pipefail

BASE_URL="http://localhost:9081"
DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"

echo ""
echo "=========================================="
echo "  测试远程代理拉取包的版本号"
echo "=========================================="

# 清空包数据
echo "清理数据库..."
sqlite3 "$DB" "DELETE FROM package_files; DELETE FROM package_versions; DELETE FROM packages;"
echo "✓ 数据库已清空"

# ===== 测试1: NPM (下载tarball触发存储) =====
echo ""
echo "【1】NPM - 下载lodash tarball（触发远程代理拉取并存储）..."
HTTP1=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/npm-virtual/lodash/-/lodash-4.17.21.tgz")
echo "HTTP状态: $HTTP1"
sleep 1

echo "NPM包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'npm' LIMIT 3;"

# ===== 测试2: Maven (下载pom触发存储) =====
echo ""
echo "【2】Maven - 下载guava pom（触发远程代理拉取并存储）..."
HTTP2=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
echo "HTTP状态: $HTTP2"
sleep 1

echo "Maven包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'maven' LIMIT 3;"

# ===== 测试3: PyPI (直接请求代理仓库) =====
echo ""
echo "【3】PyPI - 下载requests wheel（直接请求代理仓库触发拉取）..."
HTTP3=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/pypi-proxy-tuna/packages/packages/fc/da/92479a81b18d97b5325003653dbd38538f573d696a19560b385b293a9828/requests-2.31.0-py3-none-any.whl")
echo "HTTP状态: $HTTP3"
sleep 1

echo "PyPI包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'pypi' LIMIT 3;"

# ===== 测试4: Go (直接请求代理仓库) =====
echo ""
echo "【4】Go - 请求gin go.mod（直接请求代理仓库触发拉取）..."
HTTP4=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/go-proxy-goproxy-cn/github.com/gin-gonic/gin/@v/v1.9.1.mod")
echo "HTTP状态: $HTTP4"
sleep 1

echo "Go包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'go' LIMIT 3;"

# ===== 汇总 =====
echo ""
echo "=========================================="
echo "  所有包类型版本号汇总"
echo "=========================================="
sqlite3 -header -column "$DB" "
  SELECT 
    p.type as '类型',
    p.name as '包名',
    pv.version as '版本号',
    substr(pv.storage_path, 1, 60) as '存储路径'
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  ORDER BY p.type, p.name
  LIMIT 20;
"
echo ""
echo "分析结果:"
echo "- NPM: 版本号格式应为 4.17.21/package.tgz"
echo "- Maven: 版本号应为 32.1.3-jre"
echo "- PyPI: 版本号应为 2.31.0"
echo "- Go: 版本号应为 v1.9.1"
echo ""
