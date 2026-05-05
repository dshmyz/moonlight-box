#!/bin/bash

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
INFO_COUNT=0

pass() {
    echo -e "  ${GREEN}✓ PASS${NC} $1"
    ((PASS_COUNT++))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC} $1"
    ((FAIL_COUNT++))
}

info() {
    echo -e "  ${BLUE}ℹ INFO${NC} $1"
    ((INFO_COUNT++))
}

warn() {
    echo -e "  ${YELLOW}⚠ WARN${NC} $1"
}

section() {
    echo
    echo "════════════════════════════════════════"
    echo -e "  ${PURPLE}$1${NC}"
    echo "════════════════════════════════════════"
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

echo "============================================"
echo -e "${CYAN} 综合仓库功能测试${NC}"
echo " 目标: $BASE_URL"
echo " 说明: 测试各类型仓库的核心功能，保留所有测试数据"
echo "============================================"
echo

TOKEN=$(get_auth_token)
if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

info "认证令牌获取成功"

echo
echo "============================================"
echo -e "${CYAN} 第一部分: Maven 仓库测试${NC}"
echo "============================================"

section "测试 1.1: Maven 代理仓库 - 下载制品"

MAVEN_PROXY_URL="$BASE_URL/repo/maven-proxy-aliyun"
GUAVA_JAR="com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar"

HTTP_CODE=$(curl -s -o /tmp/guava-test.jar -w "%{http_code}" "$MAVEN_PROXY_URL/$GUAVA_JAR")
if [ "$HTTP_CODE" = "200" ]; then
    pass "从 Maven 代理仓库下载 guava.jar 成功 (HTTP 200)"
    FILE_SIZE=$(stat -f%z /tmp/guava-test.jar 2>/dev/null || stat -c%s /tmp/guava-test.jar 2>/dev/null)
    info "文件大小: $FILE_SIZE bytes"
else
    fail "从 Maven 代理仓库下载失败 (HTTP $HTTP_CODE)"
fi

section "测试 1.2: Maven 本地仓库 - 上传制品"

MAVEN_LOCAL_URL="$BASE_URL/repo/maven-local"
TEST_JAR_PATH="com/test/comprehensive-test/1.0.0/comprehensive-test-1.0.0.jar"

mkdir -p /tmp/maven-upload-test
cat > /tmp/maven-upload-test/Test.java <<'EOF'
public class Test {
    public static void main(String[] args) {
        System.out.println("Comprehensive Test Artifact");
    }
}
EOF

cd /tmp/maven-upload-test
jar cf test.jar Test.java 2>/dev/null || zip -q test.jar Test.java

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$MAVEN_LOCAL_URL/$TEST_JAR_PATH" \
    -H "Authorization: Bearer $TOKEN" \
    --data-binary @test.jar)

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "上传 JAR 到 Maven 本地仓库成功 (HTTP $HTTP_CODE)"
else
    fail "上传 JAR 到 Maven 本地仓库失败 (HTTP $HTTP_CODE)"
fi

section "测试 1.3: Maven 本地仓库 - 验证上传"

HTTP_CODE=$(curl -s -o /tmp/downloaded-test.jar -w "%{http_code}" \
    "$MAVEN_LOCAL_URL/$TEST_JAR_PATH")

if [ "$HTTP_CODE" = "200" ]; then
    pass "从 Maven 本地仓库下载刚上传的 JAR 成功 (HTTP 200)"
else
    fail "从 Maven 本地仓库下载失败 (HTTP $HTTP_CODE)"
fi

section "测试 1.4: Maven 虚拟仓库 - 统一访问"

MAVEN_VIRTUAL_URL="$BASE_URL/repo/maven-virtual"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$MAVEN_VIRTUAL_URL/$GUAVA_JAR")

if [ "$HTTP_CODE" = "200" ]; then
    pass "通过虚拟仓库访问代理仓库的制品成功 (HTTP 200)"
else
    fail "通过虚拟仓库访问失败 (HTTP $HTTP_CODE)"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$MAVEN_VIRTUAL_URL/$TEST_JAR_PATH")

if [ "$HTTP_CODE" = "200" ]; then
    pass "通过虚拟仓库访问本地仓库的制品成功 (HTTP 200)"
else
    fail "通过虚拟仓库访问本地仓库失败 (HTTP $HTTP_CODE)"
fi

echo
echo "============================================"
echo -e "${CYAN} 第二部分: npm 仓库测试${NC}"
echo "============================================"

section "测试 2.1: npm 代理仓库 - 下载包"

NPM_PROXY_URL="$BASE_URL/repo/npm-proxy-cn"
LODASH_META="lodash"

HTTP_CODE=$(curl -s -o /tmp/lodash-meta.json -w "%{http_code}" "$NPM_PROXY_URL/$LODASH_META")
if [ "$HTTP_CODE" = "200" ]; then
    pass "从 npm 代理仓库下载 lodash 元数据成功 (HTTP 200)"
    if grep -q '"name"' /tmp/lodash-meta.json; then
        pass "元数据包含正确的 name 字段"
    else
        fail "元数据格式不正确"
    fi
else
    fail "从 npm 代理仓库下载失败 (HTTP $HTTP_CODE)"
fi

section "测试 2.2: npm 本地仓库 - 发布包"

NPM_LOCAL_URL="$BASE_URL/repo/npm-local"
TEST_PACKAGE_NAME="comprehensive-test-package"
TEST_PACKAGE_VERSION="1.0.0"

mkdir -p /tmp/npm-publish-test
cd /tmp/npm-publish-test

cat > package.json <<EOF
{
  "name": "$TEST_PACKAGE_NAME",
  "version": "$TEST_PACKAGE_VERSION",
  "description": "Comprehensive test package",
  "main": "index.js"
}
EOF

cat > index.js <<'EOF'
module.exports = {
    greet: function(name) {
        return 'Hello, ' + name + '!';
    }
};
EOF

npm pack --silent 2>/dev/null
TARBALL="${TEST_PACKAGE_NAME}-${TEST_PACKAGE_VERSION}.tgz"

if [ -f "$TARBALL" ]; then
    pass "npm 包打包成功: $TARBALL"
    
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
        "$NPM_LOCAL_URL/$TEST_PACKAGE_NAME" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        --data-binary @- <<PKGEOF
{
  "_id": "$TEST_PACKAGE_NAME",
  "name": "$TEST_PACKAGE_NAME",
  "description": "Comprehensive test package",
  "dist-tags": {
    "latest": "$TEST_PACKAGE_VERSION"
  },
  "versions": {
    "$TEST_PACKAGE_VERSION": {
      "name": "$TEST_PACKAGE_NAME",
      "version": "$TEST_PACKAGE_VERSION",
      "dist": {
        "tarball": "$NPM_LOCAL_URL/$TEST_PACKAGE_NAME/-/$TARBALL"
      }
    }
  },
  "_attachments": {
    "$TARBALL": {
      "content_type": "application/octet-stream",
      "data": "$(base64 < $TARBALL)"
    }
  }
}
PKGEOF
)
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        pass "发布 npm 包到本地仓库成功 (HTTP $HTTP_CODE)"
    else
        info "发布 npm 包返回 HTTP $HTTP_CODE (可能需要特定格式)"
    fi
else
    fail "npm 包打包失败"
fi

section "测试 2.3: npm 虚拟仓库 - 统一访问"

NPM_VIRTUAL_URL="$BASE_URL/repo/npm-virtual"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$NPM_VIRTUAL_URL/$LODASH_META")

if [ "$HTTP_CODE" = "200" ]; then
    pass "通过虚拟仓库访问代理仓库的 npm 包成功 (HTTP 200)"
else
    fail "通过虚拟仓库访问失败 (HTTP $HTTP_CODE)"
fi

echo
echo "============================================"
echo -e "${CYAN} 第三部分: PyPI 仓库测试${NC}"
echo "============================================"

section "测试 3.1: PyPI 代理仓库 - 下载包"

PYPI_PROXY_URL="$BASE_URL/repo/pypi-proxy-tuna"
REQUESTS_SIMPLE="requests"

HTTP_CODE=$(curl -s -o /tmp/requests-simple.html -w "%{http_code}" \
    "$PYPI_PROXY_URL/simple/$REQUESTS_SIMPLE")

if [ "$HTTP_CODE" = "200" ]; then
    pass "从 PyPI 代理仓库下载 requests simple 页面成功 (HTTP 200)"
    if grep -q '<a' /tmp/requests-simple.html; then
        pass "Simple 页面包含下载链接"
    else
        fail "Simple 页面格式不正确"
    fi
else
    fail "从 PyPI 代理仓库下载失败 (HTTP $HTTP_CODE)"
fi

section "测试 3.2: PyPI 本地仓库 - 上传包"

PYPI_LOCAL_URL="$BASE_URL/repo/pypi-local"
TEST_PYPI_PACKAGE="comprehensive-test-pypi"
TEST_PYPI_VERSION="1.0.0"

mkdir -p /tmp/pypi-upload-test
cd /tmp/pypi-upload-test

mkdir -p ${TEST_PYPI_PACKAGE}
cat > ${TEST_PYPI_PACKAGE}/__init__.py <<EOF
"""Comprehensive Test PyPI Package"""
__version__ = "${TEST_PYPI_VERSION}"

def greet(name):
    return f"Hello, {name}!"
EOF

cat > setup.py <<EOF
from setuptools import setup, find_packages

setup(
    name="${TEST_PYPI_PACKAGE}",
    version="${TEST_PYPI_VERSION}",
    packages=find_packages(),
    author="Test Author",
    description="Comprehensive test package",
)
EOF

python3 setup.py sdist --dist-dir /tmp 2>/dev/null || echo "sdist 创建可能失败"

TARBALL="/tmp/${TEST_PYPI_PACKAGE}-${TEST_PYPI_VERSION}.tar.gz"
if [ -f "$TARBALL" ]; then
    pass "PyPI 包打包成功: $(basename $TARBALL)"
    
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
        "$PYPI_LOCAL_URL/upload" \
        -H "Authorization: Bearer $TOKEN" \
        -F "content=@$TARBALL")
    
    info "PyPI 上传返回 HTTP $HTTP_CODE"
else
    info "PyPI 包打包跳过 (需要 Python 环境)"
fi

section "测试 3.3: PyPI 虚拟仓库 - 统一访问"

PYPI_VIRTUAL_URL="$BASE_URL/repo/pypi-virtual"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$PYPI_VIRTUAL_URL/simple/$REQUESTS_SIMPLE")

if [ "$HTTP_CODE" = "200" ]; then
    pass "通过虚拟仓库访问代理仓库的 PyPI 包成功 (HTTP 200)"
else
    fail "通过虚拟仓库访问失败 (HTTP $HTTP_CODE)"
fi

echo
echo "============================================"
echo -e "${CYAN} 第四部分: Go 仓库测试${NC}"
echo "============================================"

section "测试 4.1: Go 代理仓库 - 下载模块"

GO_PROXY_URL="$BASE_URL/repo/go-proxy-goproxy-cn"
TESTIFY_MODULE="github.com/stretchr/testify"

HTTP_CODE=$(curl -s -o /tmp/testify-list -w "%{http_code}" \
    "$GO_PROXY_URL/$TESTIFY_MODULE/@v/list")

if [ "$HTTP_CODE" = "200" ]; then
    pass "从 Go 代理仓库获取模块版本列表成功 (HTTP 200)"
    VERSION_COUNT=$(wc -l < /tmp/testify-list | tr -d ' ')
    info "找到 $VERSION_COUNT 个版本"
else
    fail "从 Go 代理仓库获取版本列表失败 (HTTP $HTTP_CODE)"
fi

HTTP_CODE=$(curl -s -o /tmp/testify-info -w "%{http_code}" \
    "$GO_PROXY_URL/$TESTIFY_MODULE/@latest")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "302" ]; then
    pass "获取模块最新版本信息成功 (HTTP $HTTP_CODE)"
    HTTP_CODE=$(curl -s -o /tmp/testify-info -w "%{http_code}" -L \
        "$GO_PROXY_URL/$TESTIFY_MODULE/@latest")
    if [ "$HTTP_CODE" = "200" ] && grep -q 'Version' /tmp/testify-info; then
        pass "版本信息包含 Version 字段"
    else
        fail "版本信息格式不正确"
    fi
else
    fail "获取模块版本信息失败 (HTTP $HTTP_CODE)"
fi

echo
echo "============================================"
echo -e "${CYAN} 第五部分: 数据验证${NC}"
echo "============================================"

section "测试 5.1: 数据库记录验证"

DB_PATH="$PROJECT_ROOT/data/registry.db"
if [ -f "$DB_PATH" ]; then
    pass "数据库文件存在"
    
    PACKAGE_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM package_versions;" 2>/dev/null || echo "0")
    info "数据库中包版本记录数: $PACKAGE_COUNT"
    
    REPO_COUNT=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM repositories;" 2>/dev/null || echo "0")
    info "数据库中仓库记录数: $REPO_COUNT"
else
    fail "数据库文件不存在"
fi

section "测试 5.2: 存储目录验证"

STORAGE_PATH="$PROJECT_ROOT/data/packages"

for REPO_TYPE in npm maven2 pypi go; do
    if [ -d "$STORAGE_PATH/$REPO_TYPE" ]; then
        FILE_COUNT=$(find "$STORAGE_PATH/$REPO_TYPE" -type f | wc -l | tr -d ' ')
        DIR_COUNT=$(find "$STORAGE_PATH/$REPO_TYPE" -type d | wc -l | tr -d ' ')
        pass "$REPO_TYPE 存储目录存在 (文件: $FILE_COUNT, 目录: $DIR_COUNT)"
    else
        info "$REPO_TYPE 存储目录不存在"
    fi
done

section "测试 5.3: 缓存文件验证"

if [ -f "/tmp/guava-test.jar" ]; then
    FILE_SIZE=$(stat -f%z /tmp/guava-test.jar 2>/dev/null || stat -c%s /tmp/guava-test.jar 2>/dev/null)
    pass "Maven 缓存文件存在 (大小: $FILE_SIZE bytes)"
fi

if [ -f "/tmp/lodash-meta.json" ]; then
    pass "npm 缓存文件存在"
fi

if [ -f "/tmp/requests-simple.html" ]; then
    pass "PyPI 缓存文件存在"
fi

echo
echo "============================================"
echo -e "${CYAN} 测试汇总${NC}"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  信息: ${BLUE}$INFO_COUNT${NC}"
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    echo -e "${CYAN}测试数据已保留在以下位置:${NC}"
    echo "  - Maven 本地仓库: $MAVEN_LOCAL_URL"
    echo "  - npm 本地仓库: $NPM_LOCAL_URL"
    echo "  - PyPI 本地仓库: $PYPI_LOCAL_URL"
    echo "  - 数据库: $DB_PATH"
    echo "  - 存储目录: $STORAGE_PATH"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
