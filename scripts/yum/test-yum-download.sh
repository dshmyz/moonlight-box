#!/bin/bash
set -euo pipefail

BASE_URL="http://localhost:9081"
API_URL="${BASE_URL}/api/v1"
YUM_REPO="yum-local"

echo "=== 测试下载指定组件类型的包（YUM/RPM）==="

echo ""
echo "步骤 1: 登录获取token"
LOGIN_RESPONSE=$(curl -s -X POST "${API_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}')
TOKEN=$(echo "$LOGIN_RESPONSE" | jq -r '.data.access_token')
echo "✓ 登录成功"

echo ""
echo "步骤 2: 创建YUM本地仓库"
CREATE_REPO=$(curl -s -X POST "${API_URL}/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{
    "name": "'${YUM_REPO}'",
    "display_name": "YUM 本地仓库",
    "description": "测试用YUM本地仓库",
    "type": "local",
    "package_type": "yum",
    "enabled": true
  }')
echo "仓库创建响应: $CREATE_REPO"

echo ""
echo "步骤 3: 创建测试RPM文件"
FAKE_RPM="/tmp/test-nginx-1.20.1-1.el9.x86_64.rpm"
if [ ! -f "$FAKE_RPM" ]; then
  echo "这是一个测试用的假RPM包内容" > "$FAKE_RPM"
  echo "包名: nginx" >> "$FAKE_RPM"
  echo "版本: 1.20.1" >> "$FAKE_RPM"
fi
echo "✓ 测试RPM文件已创建: $FAKE_RPM"

echo ""
echo "步骤 4: 上传RPM包到YUM仓库"
UPLOAD_RESPONSE=$(curl -s -X PUT "${BASE_URL}/repo/${YUM_REPO}/test-nginx-1.20.1-1.el9.x86_64.rpm" \
  -H "Authorization: Bearer ${TOKEN}" \
  --data-binary "@${FAKE_RPM}")
echo "上传响应: $UPLOAD_RESPONSE"

echo ""
echo "步骤 5: 验证包是否上传成功（查询数据库）"
if [ -f "data/registry.db" ]; then
  PKG_COUNT=$(sqlite3 data/registry.db "SELECT COUNT(*) FROM packages WHERE type='yum';" 2>/dev/null || echo "0")
  echo "YUM类型包数量: $PKG_COUNT"
else
  echo "数据库文件不存在"
fi

echo ""
echo "步骤 6: 通过YUM端点下载RPM包"
DOWNLOAD_RESPONSE=$(curl -s -w "\n%{http_code}" "${BASE_URL}/repo/${YUM_REPO}/test-nginx-1.20.1-1.el9.x86_64.rpm" \
  -o /tmp/downloaded_test.rpm)
HTTP_CODE=$(echo "$DOWNLOAD_RESPONSE" | tail -n1)
echo "下载HTTP状态码: $HTTP_CODE"

if [ "$HTTP_CODE" = "200" ]; then
  echo "✓ 下载成功"
  echo "下载文件大小: $(wc -c < /tmp/downloaded_test.rpm) 字节"
  echo "文件内容:"
  cat /tmp/downloaded_test.rpm
else
  echo "✗ 下载失败，状态码: $HTTP_CODE"
fi

echo ""
echo "步骤 7: 清理测试数据"
curl -s -X DELETE "${API_URL}/repositories/${YUM_REPO}" \
  -H "Authorization: Bearer ${TOKEN}" > /dev/null 2>&1 || true
rm -f "$FAKE_RPM" /tmp/downloaded_test.rpm
echo "✓ 清理完成"

echo ""
echo "=== 测试完成 ==="
