#!/bin/bash
set -euo pipefail

DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"
BASE="http://localhost:9081"

echo ""
echo "=========================================="
echo "  修复后测试所有类型版本号"
echo "=========================================="

# 清理
sqlite3 "$DB" "DELETE FROM package_files; DELETE FROM package_versions; DELETE FROM packages; DELETE FROM cache_entries;"
echo "✓ DB已清空"

# ===== 1. Maven 上传 =====
echo ""
echo "【1】Maven - 上传guava pom..."
HTTP1=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
echo "HTTP: ${HTTP1}"
sleep 2

echo "Maven版本:"
sqlite3 -header -column "$DB" "SELECT p.name, pv.version FROM packages p JOIN package_versions pv ON p.id = pv.package_id WHERE p.type = 'maven' LIMIT 3;"

# ===== 2. Go 下载 =====
echo ""
echo "【2】Go - 下载gin go.mod..."
HTTP2=$(curl -s -o /dev/null -w "%{http_code}" "${BASE}/repo/go-proxy-goproxy-cn/github.com/gin-gonic/gin/@v/v1.9.1.mod")
echo "HTTP: ${HTTP2}"
sleep 2

echo "Go版本:"
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
echo "期望结果:"
echo "- Maven: 32.1.3-jre ✓"
echo "- Go: v1.9.1 ✓"
echo ""
