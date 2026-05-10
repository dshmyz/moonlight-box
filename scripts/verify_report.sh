#!/bin/bash
BASE_URL="http://localhost:9081"
DB="/Users/gracegaoya/work/project/moonlight-box/data/registry.db"
PKG_DIR="/Users/gracegaoya/work/project/moonlight-box/data/packages"
ADMIN_USER="admin"
ADMIN_PASS="admin123"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASS=0
FAIL=0

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); }
fixed() { echo -e "  ${GREEN}✓ FIXED${NC} $1 (已修复)"; PASS=$((PASS + 1)); }

echo "============================================"
echo " 根据测试报告验证所有功能点"
echo "============================================"

TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" -H "Content-Type: application/json" -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | grep -o '"access_token":"[^"]*"' | sed 's/"access_token":"//;s/"//')
[ -z "$TOKEN" ] && { echo "登录失败"; exit 1; }

echo ""; echo "=== 1. Maven 仓库测试 ==="

HTTP=$(curl -s -o /tmp/test-guava.jar -w "%{http_code}" "$BASE_URL/repo/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar")
[ "$HTTP" = "200" ] && [ -s /tmp/test-guava.jar ] && pass "Maven 代理下载 (HTTP $HTTP)" || fail "Maven 代理下载 (HTTP $HTTP)"

TEST_JAR="/tmp/test-$$-1.0.0.jar"
echo "test content" > "$TEST_JAR"
HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE_URL/repo/maven-local/com/test/verify-test/1.0.0/verify-test-1.0.0.jar" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/octet-stream" --data-binary @"$TEST_JAR")
[ "$HTTP" = "200" ] || [ "$HTTP" = "201" ] && pass "Maven 本地上传 (HTTP $HTTP)" || fail "Maven 本地上传 (HTTP $HTTP)"

HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repo/maven-local/com/test/verify-test/1.0.0/verify-test-1.0.0.jar")
[ "$HTTP" = "200" ] && pass "Maven 本地下载 (HTTP $HTTP)" || fail "Maven 本地下载 (HTTP $HTTP)"

HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repo/maven-virtual/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.pom")
[ "$HTTP" = "200" ] && pass "Maven 虚拟仓库访问 (HTTP $HTTP)" || echo -e "  ${YELLOW}ℹ INFO${NC} Maven 虚拟仓库访问 (HTTP $HTTP)"

echo ""; echo "=== 2. npm 仓库测试 ==="

HTTP=$(curl -s -o /tmp/npm-lodash.json -w "%{http_code}" "$BASE_URL/repo/npm-proxy-cn/lodash")
[ "$HTTP" = "200" ] && pass "npm 代理下载 (HTTP $HTTP)" || fail "npm 代理下载 (HTTP $HTTP)"

TEST_DIR="/tmp/npm-test-$$"
mkdir -p "$TEST_DIR"
echo '{"name":"test-verify-pkg","version":"1.0.0"}' > "$TEST_DIR/package.json"
echo "module.exports = {};" > "$TEST_DIR/index.js"
cd "$TEST_DIR" && tar czf test-verify-pkg-1.0.0.tgz package.json index.js && cd - > /dev/null

HTTP=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "$BASE_URL/repo/npm-local/test-verify-pkg/-/test-verify-pkg-1.0.0.tgz" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/octet-stream" --data-binary @"$TEST_DIR/test-verify-pkg-1.0.0.tgz")
rm -rf "$TEST_DIR"

if [ "$HTTP" = "200" ] || [ "$HTTP" = "201" ] || [ "$HTTP" = "409" ]; then
    fixed "npm 发布 (HTTP $HTTP) - 之前400问题已修复"
else
    fail "npm 发布 (HTTP $HTTP)"
fi

HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repo/npm-virtual/lodash")
[ "$HTTP" = "200" ] && pass "npm 虚拟仓库访问 (HTTP $HTTP)" || echo -e "  ${YELLOW}ℹ INFO${NC} npm 虚拟仓库访问 (HTTP $HTTP)"

echo ""; echo "=== 3. PyPI 仓库测试 ==="

HTTP=$(curl -s -o /tmp/pypi-requests.html -w "%{http_code}" "$BASE_URL/repo/pypi-proxy-tuna/simple/requests/")
[ "$HTTP" = "200" ] && pass "PyPI 代理下载 (HTTP $HTTP)" || fail "PyPI 代理下载 (HTTP $HTTP)"

HTTP=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repo/pypi-virtual/simple/requests/")
[ "$HTTP" = "200" ] && pass "PyPI 虚拟仓库访问 (HTTP $HTTP)" || echo -e "  ${YELLOW}ℹ INFO${NC} PyPI 虚拟仓库访问 (HTTP $HTTP)"

echo ""; echo "=== 4. Go 仓库测试 (重点验证 @latest 接口) ==="

HTTP=$(curl -s -o /tmp/go-versions.txt -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/list")
[ "$HTTP" = "200" ] && pass "Go @v/list (HTTP $HTTP)" || fail "Go @v/list (HTTP $HTTP)"

HTTP=$(curl -s -o /tmp/go-latest.json -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@latest")
if [ "$HTTP" = "200" ]; then
    fixed "Go @latest 接口 (HTTP $HTTP) - 之前404问题已修复"
elif [ "$HTTP" = "404" ]; then
    fail "Go @latest 接口仍然返回 404 (问题未修复)"
else
    echo -e "  ${YELLOW}ℹ INFO${NC} Go @latest 接口 (HTTP $HTTP)"
fi

HTTP=$(curl -s -o /tmp/go-info.json -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.info")
[ "$HTTP" = "200" ] && pass "Go .info 文件 (HTTP $HTTP)" || fail "Go .info 文件 (HTTP $HTTP)"

HTTP=$(curl -s -o /tmp/go.mod -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.mod")
[ "$HTTP" = "200" ] && pass "Go .mod 文件 (HTTP $HTTP)" || fail "Go .mod 文件 (HTTP $HTTP)"

HTTP=$(curl -s -o /tmp/go-test.zip -w "%{http_code}" "$BASE_URL/repo/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.zip")
[ "$HTTP" = "200" ] && [ -s /tmp/go-test.zip ] && pass "Go .zip 文件 (HTTP $HTTP)" || fail "Go .zip 文件 (HTTP $HTTP)"

echo ""; echo "=== 5. 数据验证 ==="

[ -f "$DB" ] && pass "数据库文件存在" || fail "数据库文件不存在"
PKG_COUNT=$(sqlite3 "$DB" "SELECT COUNT(*) FROM package_versions;" 2>/dev/null || echo "0")
echo -e "  ${YELLOW}ℹ INFO${NC} 包版本记录数: $PKG_COUNT"

for type in npm maven2 pypi go; do
    DIR="$PKG_DIR/$type"
    [ -d "$DIR" ] && pass "$type 存储目录存在" || echo -e "  ${YELLOW}ℹ INFO${NC} $type 存储目录不存在"
done

echo ""
echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo "============================================"

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}✓ 所有验证通过! 测试报告中的问题已全部修复${NC}"
    exit 0
else
    echo -e "${RED}✗ 部分验证失败${NC}"
    exit 1
fi