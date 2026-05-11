#!/bin/bash
# 测试 Maven 本地仓库下载
# 用于复现和诊断问题

BASE_URL="http://localhost:9081"

echo "============================================"
echo " Maven 本地仓库下载测试"
echo "============================================"
echo

echo "1. 上传测试文件到 maven-local"
echo "----------------------------------------"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar" \
    -H "Content-Type: application/java-archive" \
    --data-binary @/dev/null)

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    echo "✓ 上传成功 (HTTP $HTTP_CODE)"
else
    echo "⚠ 上传返回 HTTP $HTTP_CODE (可能已存在)"
fi

echo
echo "2. 从 maven-local 下载文件"
echo "----------------------------------------"
HTTP_CODE=$(curl -s -o /tmp/test-maven-local.jar -w "%{http_code}" \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar")

if [ "$HTTP_CODE" = "200" ]; then
    echo "✓ 下载成功 (HTTP 200)"
    if [ -f /tmp/test-maven-local.jar ]; then
        SIZE=$(stat -f%z /tmp/test-maven-local.jar 2>/dev/null || stat -c%s /tmp/test-maven-local.jar 2>/dev/null || echo "unknown")
        echo "  文件大小: $SIZE bytes"
    fi
else
    echo "✗ 下载失败 (HTTP $HTTP_CODE)"
fi

echo
echo "3. 从 maven-virtual 下载文件（测试虚拟仓库）"
echo "----------------------------------------"
HTTP_CODE=$(curl -s -o /tmp/test-maven-virtual.jar -w "%{http_code}" \
    "$BASE_URL/repo/maven-virtual/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar")

if [ "$HTTP_CODE" = "200" ]; then
    echo "✓ 下载成功 (HTTP 200)"
    if [ -f /tmp/test-maven-virtual.jar ]; then
        SIZE=$(stat -f%z /tmp/test-maven-virtual.jar 2>/dev/null || stat -c%s /tmp/test-maven-virtual.jar 2>/dev/null || echo "unknown")
        echo "  文件大小: $SIZE bytes"
    fi
else
    echo "✗ 下载失败 (HTTP $HTTP_CODE)"
fi

echo
echo "4. 检查数据库中的错误日志"
echo "----------------------------------------"
sqlite3 /Users/gracegaoya/work/project/moonlight-box/data/registry.db \
    "SELECT id, status, error_message, created_at 
     FROM proxy_download_logs 
     WHERE package_type = 'maven' 
     ORDER BY created_at DESC 
     LIMIT 5;"

echo
echo "============================================"
echo " 测试完成"
echo "============================================"
echo
echo "请检查服务日志以获取详细信息："
echo "  tail -f /Users/gracegaoya/work/project/moonlight-box/logs/registry.log"
