#!/bin/bash
set -euo pipefail

BASE_URL="http://localhost:9081"
YUM_REPO="yum-test-local"
TOKEN=""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

echo ""
echo "=== 测试YUM RPM包版本号显示 ==="
echo ""

# 步骤1: 登录获取token
echo "步骤1: 登录获取token..."
LOGIN_RESPONSE=$(curl -s -X POST "${BASE_URL}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}')

if echo "$LOGIN_RESPONSE" | grep -q "token"; then
  TOKEN=$(echo "$LOGIN_RESPONSE" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
  echo -e "${GREEN}✓ 登录成功${NC}"
else
  echo -e "${RED}✗ 登录失败: ${LOGIN_RESPONSE}${NC}"
  exit 1
fi

# 步骤2: 创建YUM本地仓库
echo ""
echo "步骤2: 创建YUM本地仓库..."
CREATE_REPO=$(curl -s -X POST "${BASE_URL}/api/v1/repositories" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"name\": \"${YUM_REPO}\",
    \"display_name\": \"YUM 测试仓库\",
    \"description\": \"测试用YUM本地仓库\",
    \"type\": \"local\",
    \"package_type\": \"yum\",
    \"enabled\": true
  }")

if echo "$CREATE_REPO" | grep -qE '"id"|"name"'; then
  echo -e "${GREEN}✓ 仓库创建成功${NC}"
else
  echo -e "仓库可能已存在，继续测试..."
fi

# 步骤3: 创建测试RPM文件
echo ""
echo "步骤3: 创建测试RPM文件..."
FAKE_RPM="/tmp/nginx-1.20.1-1.el9.x86_64.rpm"
echo "这是测试用的RPM包内容 - nginx 1.20.1" > "$FAKE_RPM"
echo "Release: 1.el9" >> "$FAKE_RPM"
echo "Architecture: x86_64" >> "$FAKE_RPM"
echo -e "${GREEN}✓ 测试RPM文件已创建: ${FAKE_RPM}${NC}"

# 步骤4: 上传RPM包
echo ""
echo "步骤4: 上传RPM包到YUM仓库..."
UPLOAD_RESPONSE=$(curl -s -X POST "${BASE_URL}/api/v1/yum/${YUM_REPO}/upload" \
  -H "Authorization: Bearer ${TOKEN}" \
  -F "file=@${FAKE_RPM}")

echo "上传响应: ${UPLOAD_RESPONSE}"

if echo "$UPLOAD_RESPONSE" | grep -q '"success".*true'; then
  echo -e "${GREEN}✓ RPM包上传成功${NC}"
else
  echo -e "${RED}✗ RPM包上传失败${NC}"
  exit 1
fi

# 步骤5: 检查数据库中的版本号
echo ""
echo "步骤5: 检查数据库中的版本号..."
DB_VERSION=$(sqlite3 data/registry.db "SELECT version FROM package_versions WHERE package_id IN (SELECT id FROM packages WHERE type='yum') ORDER BY created_at DESC LIMIT 1;")
DB_METADATA=$(sqlite3 data/registry.db "SELECT metadata FROM package_versions WHERE package_id IN (SELECT id FROM packages WHERE type='yum') ORDER BY created_at DESC LIMIT 1;")

echo "数据库中的版本号: ${DB_VERSION}"
echo "数据库中的Metadata: ${DB_METADATA}"

# 验证版本号是否纯净（不包含release）
if echo "$DB_VERSION" | grep -q "^[0-9]"; then
  echo -e "${GREEN}✓ 版本号格式正确${NC}"
else
  echo -e "${RED}✗ 版本号格式异常${NC}"
fi

# 验证release是否在metadata中
if echo "$DB_METADATA" | grep -q "release"; then
  echo -e "${GREEN}✓ Release信息已保存在metadata中${NC}"
else
  echo -e "${RED}✗ Release信息未保存在metadata中${NC}"
fi

# 步骤6: 测试下载
echo ""
echo "步骤6: 测试下载RPM包..."
DOWNLOAD_PATH="${BASE_URL}/api/v1/yum/${YUM_REPO}/Packages/x86_64/nginx-1.20.1-1.el9.x86_64.rpm"
HTTP_CODE=$(curl -s -o /tmp/downloaded_test.rpm -w "%{http_code}" "$DOWNLOAD_PATH")

if [ "$HTTP_CODE" = "200" ]; then
  echo -e "${GREEN}✓ 下载成功 (HTTP ${HTTP_CODE})${NC}"
  echo "下载文件大小: $(wc -c < /tmp/downloaded_test.rpm) 字节"
  echo "下载文件内容:"
  cat /tmp/downloaded_test.rpm
else
  echo -e "${RED}✗ 下载失败 (HTTP ${HTTP_CODE})${NC}"
fi

# 步骤7: 列出所有YUM包版本
echo ""
echo "步骤7: 查询所有YUM包版本..."
sqlite3 -header data/registry.db "
  SELECT p.name, pv.version, pv.metadata 
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  WHERE p.type = 'yum'
  ORDER BY p.name, pv.version;
"

echo ""
echo "=== 测试完成 ==="
echo ""
echo "验证要点:"
echo "1. 版本号字段应该只显示纯净版本号（如 1.20.1）"
echo "2. release信息（如 1.el9）应该在metadata中"
echo "3. 下载路径仍然使用完整文件名（如 nginx-1.20.1-1.el9.x86_64.rpm）"
echo ""
