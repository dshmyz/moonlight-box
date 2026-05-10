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
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/npm-virtual/lodash"
sleep 1

echo "NPM包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'npm' LIMIT 5;"

# ===== 测试2: Maven =====
echo ""
echo "【2】Maven - 请求guava pom文件（触发远程代理拉取）..."
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom"
sleep 1

echo "Maven包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'maven' LIMIT 5;"

# ===== 测试3: PyPI =====
echo ""
echo "【3】PyPI - 请求requests包（触发远程代理拉取）..."
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/pypi-virtual/simple/requests/"
sleep 1

echo "PyPI包版本记录:"
sqlite3 -header "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'pypi' LIMIT 5;"

# ===== 测试4: Go =====
echo ""
echo "【4】Go - 请求gin模块go.mod（触发远程代理拉取）..."
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/go-virtual/github.com/gin-gonic/gin/@v/v1.9.1.mod"
sleep 1

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
    pv.version as '版本号'
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  ORDER BY p.type, p.name
  LIMIT 20;
"
echo ""
