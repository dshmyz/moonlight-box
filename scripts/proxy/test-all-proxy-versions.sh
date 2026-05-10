#!/bin/bash
set -euo pipefail

BASE_URL="http://localhost:9081"
TOKEN=""

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

echo ""
echo "=================================================="
echo "   测试远程代理拉取包的版本号显示"
echo "=================================================="
echo ""

# 登录获取token
echo "步骤0: 登录获取token..."
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

# 清理之前的测试数据
echo ""
echo "清理数据库中的包数据..."
sqlite3 data/registry.db "DELETE FROM package_files; DELETE FROM package_versions; DELETE FROM packages;"
echo -e "${GREEN}✓ 数据库已清理${NC}"

# ============================================================
# 测试1: NPM 远程代理拉取
# ============================================================
echo ""
echo "=================================================="
echo -e "${YELLOW}测试 1: NPM 包 - 远程代理拉取 (lodash)${NC}"
echo "=================================================="

echo "1.1 通过npm-virtual请求lodash包元数据（触发代理回源）..."
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/npm-virtual/lodash"
echo -e "${GREEN}✓ 请求完成${NC}"

echo ""
echo "1.2 检查数据库中的npm包版本..."
NPM_VERSIONS=$(sqlite3 -header data/registry.db "
  SELECT p.name, pv.version, pv.storage_path 
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  WHERE p.type = 'npm' AND p.name = 'lodash'
  LIMIT 3;
")
echo "$NPM_VERSIONS"

if [ -n "$NPM_VERSIONS" ]; then
  echo -e "${GREEN}✓ NPM包版本记录存在${NC}"
  # 检查版本号格式
  FIRST_NPM_VER=$(sqlite3 data/registry.db "SELECT version FROM package_versions pv JOIN packages p ON p.id = pv.package_id WHERE p.type = 'npm' AND p.name = 'lodash' LIMIT 1;")
  echo "第一个版本号: $FIRST_NPM_VER"
  # NPM版本号应该是纯净的语义化版本号，如 4.17.21
  if echo "$FIRST_NPM_VER" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo -e "${GREEN}✓ NPM版本号格式正确（语义化版本号）${NC}"
  else
    echo -e "${RED}✗ NPM版本号格式异常${NC}"
  fi
else
  echo -e "${YELLOW}⚠ NPM包可能未缓存到数据库（正常，代理模式可能不写库）${NC}"
fi

# ============================================================
# 测试2: Maven 远程代理拉取
# ============================================================
echo ""
echo "=================================================="
echo -e "${YELLOW}测试 2: Maven包 - 远程代理拉取 (guava)${NC}"
echo "=================================================="

echo "2.1 请求guava的pom文件（触发代理回源）..."
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom"
echo -e "${GREEN}✓ 请求完成${NC}"

echo ""
echo "2.2 检查数据库中的maven包版本..."
MAVEN_VERSIONS=$(sqlite3 -header data/registry.db "
  SELECT p.name, pv.version, pv.storage_path 
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  WHERE p.type = 'maven' AND p.name LIKE '%guava%'
  LIMIT 3;
")
echo "$MAVEN_VERSIONS"

if [ -n "$MAVEN_VERSIONS" ]; then
  echo -e "${GREEN}✓ Maven包版本记录存在${NC}"
  FIRST_MAVEN_VER=$(sqlite3 data/registry.db "SELECT version FROM package_versions pv JOIN packages p ON p.id = pv.package_id WHERE p.type = 'maven' AND p.name LIKE '%guava%' LIMIT 1;")
  echo "第一个版本号: $FIRST_MAVEN_VER"
  # Maven版本号应该是如 32.1.3-jre
  if [ -n "$FIRST_MAVEN_VER" ]; then
    echo -e "${GREEN}✓ Maven版本号已记录${NC}"
  else
    echo -e "${RED}✗ Maven版本号为空${NC}"
  fi
else
  echo -e "${YELLOW}⚠ Maven包可能未缓存到数据库${NC}"
fi

# ============================================================
# 测试3: PyPI 远程代理拉取
# ============================================================
echo ""
echo "=================================================="
echo -e "${YELLOW}测试 3: PyPI包 - 远程代理拉取 (requests)${NC}"
echo "=================================================="

echo "3.1 请求requests包信息（触发代理回源）..."
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/pypi-virtual/simple/requests/"
echo -e "${GREEN}✓ 请求完成${NC}"

echo ""
echo "3.2 检查数据库中的pypi包版本..."
PYPI_VERSIONS=$(sqlite3 -header data/registry.db "
  SELECT p.name, pv.version, pv.storage_path 
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  WHERE p.type = 'pypi' AND p.name = 'requests'
  LIMIT 3;
")
echo "$PYPI_VERSIONS"

if [ -n "$PYPI_VERSIONS" ]; then
  echo -e "${GREEN}✓ PyPI包版本记录存在${NC}"
  FIRST_PYPI_VER=$(sqlite3 data/registry.db "SELECT version FROM package_versions pv JOIN packages p ON p.id = pv.package_id WHERE p.type = 'pypi' AND p.name = 'requests' LIMIT 1;")
  echo "第一个版本号: $FIRST_PYPI_VER"
  # PyPI版本号应该是如 2.31.0
  if echo "$FIRST_PYPI_VER" | grep -qE '^[0-9]'; then
    echo -e "${GREEN}✓ PyPI版本号格式正确${NC}"
  else
    echo -e "${RED}✗ PyPI版本号格式异常${NC}"
  fi
else
  echo -e "${YELLOW}⚠ PyPI包可能未缓存到数据库${NC}"
fi

# ============================================================
# 测试4: Go 远程代理拉取
# ============================================================
echo ""
echo "=================================================="
echo -e "${YELLOW}测试 4: Go模块 - 远程代理拉取 (gin)${NC}"
echo "=================================================="

echo "4.1 请求gin模块的go.mod文件（触发代理回源）..."
curl -s -o /dev/null "${BASE_URL}/api/v1/repository/go-virtual/github.com/gin-gonic/gin/@v/v1.9.1.mod"
echo -e "${GREEN}✓ 请求完成${NC}"

echo ""
echo "4.2 检查数据库中的go模块版本..."
GO_VERSIONS=$(sqlite3 -header data/registry.db "
  SELECT p.name, pv.version, pv.storage_path 
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  WHERE p.type = 'go' AND p.name LIKE '%gin%'
  LIMIT 3;
")
echo "$GO_VERSIONS"

if [ -n "$GO_VERSIONS" ]; then
  echo -e "${GREEN}✓ Go模块版本记录存在${NC}"
  FIRST_GO_VER=$(sqlite3 data/registry.db "SELECT version FROM package_versions pv JOIN packages p ON p.id = pv.package_id WHERE p.type = 'go' AND p.name LIKE '%gin%' LIMIT 1;")
  echo "第一个版本号: $FIRST_GO_VER"
  # Go版本号应该是如 v1.9.1
  if echo "$FIRST_GO_VER" | grep -qE '^v[0-9]'; then
    echo -e "${GREEN}✓ Go版本号格式正确（v前缀）${NC}"
  else
    echo -e "${RED}✗ Go版本号格式异常${NC}"
  fi
else
  echo -e "${YELLOW}⚠ Go模块可能未缓存到数据库${NC}"
fi

# ============================================================
# 汇总
# ============================================================
echo ""
echo "=================================================="
echo -e "${GREEN}测试完成${NC}"
echo "=================================================="
echo ""
echo "所有包类型版本汇总："
sqlite3 -header data/registry.db "
  SELECT 
    p.type as '包类型',
    p.name as '包名',
    pv.version as '版本号',
    substr(pv.storage_path, 1, 50) as '存储路径'
  FROM packages p 
  JOIN package_versions pv ON p.id = pv.package_id 
  ORDER BY p.type, p.name, pv.version
  LIMIT 20;
"
echo ""
echo "注意："
echo "- NPM版本号应该是纯净语义化版本号（如 4.17.21）"
echo "- Maven版本号应该包含后缀（如 32.1.3-jre）"
echo "- PyPI版本号应该是纯净版本号（如 2.31.0）"
echo "- Go版本号应该带v前缀（如 v1.9.1）"
echo ""
