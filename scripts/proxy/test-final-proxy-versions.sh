#!/bin/bash
set -euo pipefail

DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"
BASE_URL="http://localhost:9081"

echo ""
echo "=========================================="
echo "  远程代理拉取包版本号测试"
echo "=========================================="

# 清理数据
sqlite3 "$DB" "DELETE FROM package_files; DELETE FROM package_versions; DELETE FROM packages; DELETE FROM cache_entries;"
echo "✓ 数据库已清空"

# === 1. NPM ===
echo ""
echo "【1】NPM - 下载lodash tarball..."
NPM_HTTP=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/npm-virtual/lodash/-/lodash-4.17.21.tgz")
echo "HTTP: ${NPM_HTTP}"
sleep 1

echo "NPM版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'npm' LIMIT 3;"

# === 2. Maven ===
echo ""
echo "【2】Maven - 下载guava pom..."
MAVEN_HTTP=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
echo "HTTP: ${MAVEN_HTTP}"
sleep 1

echo "Maven版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'maven' LIMIT 3;"

# === 3. PyPI ===
echo ""
echo "【3】PyPI - 下载requests wheel..."
PYPI_HTTP=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/pypi-proxy-tuna/packages/packages/fc/da/92479a81b18d97b5325003653dbd38538f573d696a19560b385b293a9828/requests-2.31.0-py3-none-any.whl")
echo "HTTP: ${PYPI_HTTP}"
sleep 1

echo "PyPI版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'pypi' LIMIT 3;"

# === 4. Go ===
echo ""
echo "【4】Go - 下载gin go.mod..."
GO_HTTP=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/go-proxy-goproxy-cn/github.com/gin-gonic/gin/@v/v1.9.1.mod")
echo "HTTP: ${GO_HTTP}"
sleep 1

echo "Go版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'go' LIMIT 3;"

# === 汇总 ===
echo ""
echo "=========================================="
echo "  所有包类型版本号汇总"
echo "=========================================="
sqlite3 -header -column "$DB" "
  SELECT 
    p.type as '类型',
    p.name as '包名',
    pv.version as '版本号',
    substr(pv.storage_path, 1, 50) as '存储路径'
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  ORDER BY p.type, p.name
  LIMIT 20;
"
echo ""
