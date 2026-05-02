#!/bin/bash
set -euo pipefail

BASE="http://localhost:9081"
DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"
PKG_DIR="/Users/gracegaoya/work/project/moonlight-box/data/packages"

echo ""
echo "=========================================="
echo "  测试阿里云代理仓库版本号"
echo "=========================================="

# 清理缓存和数据库
echo "清理缓存和数据库..."
rm -rf "$PKG_DIR/npm" "$PKG_DIR/maven" "$PKG_DIR/maven2" "$PKG_DIR/go" "$PKG_DIR/pypi" 2>/dev/null || true
sqlite3 "$DB" "DELETE FROM package_files; DELETE FROM package_versions; DELETE FROM packages; DELETE FROM cache_entries;"
echo "✓ 清理完成"

# ===== 1. NPM (阿里云镜像) =====
echo ""
echo "【1】NPM - 从阿里云镜像下载 lodash@4.17.21..."
HTTP1=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/npm-proxy-cn/lodash/-/lodash-4.17.21.tgz")
echo "HTTP: ${HTTP1}"
sleep 2

echo "NPM版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'npm' LIMIT 3;"

# ===== 2. Maven (阿里云镜像) =====
echo ""
echo "【2】Maven - 从阿里云镜像下载 guava 32.1.3-jre..."
HTTP2=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
echo "HTTP: ${HTTP2}"
sleep 2

echo "Maven版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'maven' LIMIT 3;"

# ===== 3. PyPI (清华镜像) =====
echo ""
echo "【3】PyPI - 从清华镜像下载 requests..."
# 先获取simple页面
curl -s -o /dev/null "${BASE}/repo/pypi-proxy-tuna/simple/requests/"
sleep 1
# 下载一个wheel包
HTTP3=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/pypi-proxy-tuna/packages/requests-2.31.0-py3-none-any.whl")
echo "HTTP: ${HTTP3}"
sleep 2

echo "PyPI版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'pypi' LIMIT 3;"

# ===== 4. Go (goproxy.cn) =====
echo ""
echo "【4】Go - 从goproxy.cn下载 gin v1.9.1..."
HTTP4=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/go-proxy-goproxy-cn/github.com/gin-gonic/gin/@v/v1.9.1.mod")
echo "HTTP: ${HTTP4}"
sleep 2

echo "Go版本记录:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'go' LIMIT 3;"

# ===== 汇总 =====
echo ""
echo "=========================================="
echo "  版本号汇总"
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
echo "=========================================="
echo "  版本号协议规则验证"
echo "=========================================="
echo "NPM: 应为语义化版本号（如 4.17.21）"
echo "Maven: 可包含后缀（如 32.1.3-jre）"
echo "PyPI: 应为语义化版本号（如 2.31.0）"
echo "Go: 应带v前缀（如 v1.9.1）"
echo ""
