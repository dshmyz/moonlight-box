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

# ===== 测试1: NPM =====
echo ""
echo "【1】NPM - 请求lodash元数据（触发远程代理拉取）..."
HTTP1=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/npm-virtual/lodash")
echo "HTTP状态: $HTTP1"
sleep 2

echo "NPM包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'npm' LIMIT 5;"

# ===== 测试2: Maven =====
echo ""
echo "【2】Maven - 请求guava pom文件（触发远程代理拉取）..."
HTTP2=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
echo "HTTP状态: $HTTP2"
sleep 2

echo "Maven包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'maven' LIMIT 5;"

# ===== 测试3: PyPI =====
echo ""
echo "【3】PyPI - 请求requests simple API（触发远程代理拉取）..."
HTTP3=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/pypi-virtual/simple/requests/")
echo "HTTP状态: $HTTP3"
sleep 2

echo "PyPI包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'pypi' LIMIT 5;"

# ===== 测试4: Go =====
echo ""
echo "【4】Go - 请求gin模块go.mod（触发远程代理拉取）..."
HTTP4=$(curl -s -o /dev/null -w "%{http_code}" "${BASE_URL}/repo/go-virtual/github.com/gin-gonic/gin/@v/v1.9.1.mod")
echo "HTTP状态: $HTTP4"
sleep 2

echo "Go包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'go' LIMIT 5;"

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
    pv.storage_path as '存储路径'
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  ORDER BY p.type, p.name
  LIMIT 20;
"
echo ""
echo "注意观察:"
echo "- NPM: 版本号应该是纯语义化版本号（如 4.17.21）"
echo "- Maven: 版本号应该带后缀（如 32.1.3-jre）"
echo "- PyPI: 版本号应该是纯版本号（如 2.31.0）"
echo "- Go: 版本号应该带v前缀（如 v1.9.1）"
echo ""
