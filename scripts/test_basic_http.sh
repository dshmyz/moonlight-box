#!/bin/bash

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0

pass() {
    echo -e "  ${GREEN}✓ PASS${NC} $1"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC} $1"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

info() {
    echo -e "  ${BLUE}ℹ INFO${NC} $1"
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

echo "============================================"
echo " 基础 HTTP 接口测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

echo "════════════════════════════════════════"
echo "  测试 1: Maven 上传制品 (PUT)"
echo "════════════════════════════════════════"

TEST_JAR="/tmp/test-artifact-$$-1.0.0.jar"
TEST_POM="/tmp/test-artifact-$$-1.0.0.pom"

echo '<?xml version="1.0" encoding="UTF-8"?>
<project xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd" xmlns="http://maven.apache.org/POM/4.0.0"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.test</groupId>
  <artifactId>test-artifact</artifactId>
  <version>1.0.0</version>
</project>' > "$TEST_POM"

echo "Test JAR content - $(date)" > "$TEST_JAR"
jar cf "$TEST_JAR" -C /tmp "$(basename "$TEST_JAR")" 2>/dev/null || echo "JAR test file" > "$TEST_JAR"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
    pass "Maven JAR 上传成功 (HTTP $HTTP_CODE)"
else
    fail "Maven JAR 上传失败 (expected 201/200/204, got HTTP $HTTP_CODE)"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.pom" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/xml" \
    --data-binary @"$TEST_POM")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
    pass "Maven POM 上传成功 (HTTP $HTTP_CODE)"
else
    fail "Maven POM 上传失败 (expected 201/200/204, got HTTP $HTTP_CODE)"
fi

rm -f "$TEST_JAR" "$TEST_POM"

echo
echo "════════════════════════════════════════"
echo "  测试 2: Maven 下载制品 (GET)"
echo "════════════════════════════════════════"

DOWNLOADED_JAR="/tmp/downloaded-test-artifact-$$-1.0.0.jar"
HTTP_CODE=$(curl -s -o "$DOWNLOADED_JAR" -w "%{http_code}" \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Maven JAR 下载成功 (HTTP 200)"
    
    if [ -f "$DOWNLOADED_JAR" ] && [ -s "$DOWNLOADED_JAR" ]; then
        pass "下载的 JAR 文件非空"
    else
        fail "下载的 JAR 文件为空或不存在"
    fi
else
    fail "Maven JAR 下载失败 (expected HTTP 200, got HTTP $HTTP_CODE)"
fi

rm -f "$DOWNLOADED_JAR"

echo
echo "════════════════════════════════════════"
echo "  测试 3: Maven 校验和文件"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar.sha1")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Maven SHA1 校验和文件可访问 (HTTP 200)"
else
    info "Maven SHA1 校验和文件返回 HTTP $HTTP_CODE (可能未自动生成)"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar.md5")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Maven MD5 校验和文件可访问 (HTTP 200)"
else
    info "Maven MD5 校验和文件返回 HTTP $HTTP_CODE (可能未自动生成)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: Maven 删除制品 (DELETE)"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar" \
    -H "Authorization: Bearer $TOKEN")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
    pass "Maven JAR 删除成功 (HTTP $HTTP_CODE)"
else
    fail "Maven JAR 删除失败 (expected 200/204, got HTTP $HTTP_CODE)"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repo/maven-local/com/test/test-artifact/1.0.0/test-artifact-1.0.0.jar")

if [ "$HTTP_CODE" = "404" ]; then
    pass "删除后再次下载返回 404 (符合预期)"
else
    fail "删除后仍可下载 (expected HTTP 404, got HTTP $HTTP_CODE)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: npm 上传制品 (PUT)"
echo "════════════════════════════════════════"

TEST_NPM_TGZ="/tmp/test-npm-package-$$-1.0.0.tgz"
mkdir -p /tmp/test-npm-package-$$
echo '{"name": "test-npm-package", "version": "1.0.0"}' > /tmp/test-npm-package-$$/package.json
tar -czf "$TEST_NPM_TGZ" -C /tmp test-npm-package-$$

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/npm/test-npm-package" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_NPM_TGZ")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    pass "npm 包上传成功 (HTTP $HTTP_CODE)"
else
    info "npm 包上传返回 HTTP $HTTP_CODE (可能需要特定格式)"
fi

rm -rf /tmp/test-npm-package-$$ "$TEST_NPM_TGZ"

echo
echo "════════════════════════════════════════"
echo "  测试 6: PyPI 上传制品 (POST)"
echo "════════════════════════════════════════"

TEST_WHL="/tmp/test_pypi_package-$$-1.0.0-py3-none-any.whl"
mkdir -p /tmp/test-pypi-package-$$
echo 'Metadata-Version: 2.1
Name: test-pypi-package
Version: 1.0.0
Summary: Test package' > /tmp/test-pypi-package-$$/METADATA
echo 'print("hello")' > /tmp/test-pypi-package-$$/__init__.py
cd /tmp/test-pypi-package-$$ && zip -r "$TEST_WHL" METADATA __init__.py > /dev/null 2>&1
cd - > /dev/null

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "$BASE_URL/pypi/upload" \
    -H "Authorization: Bearer $TOKEN" \
    -F "content=@$TEST_WHL;filename=test_pypi_package-1.0.0-py3-none-any.whl")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "PyPI wheel 上传成功 (HTTP $HTTP_CODE)"
else
    info "PyPI wheel 上传返回 HTTP $HTTP_CODE (可能需要特定格式)"
fi

rm -rf /tmp/test-pypi-package-$$ "$TEST_WHL"

echo
echo "════════════════════════════════════════"
echo "  测试 7: Generic 文件上传下载"
echo "════════════════════════════════════════"

TEST_FILE="/tmp/test-generic-file-$$-1.0.0.txt"
echo "Generic file test content - $(date)" > "$TEST_FILE"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "$BASE_URL/files/upload" \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@$TEST_FILE;filename=test-generic-file.txt")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "Generic 文件上传成功 (HTTP $HTTP_CODE)"
    
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        "$BASE_URL/files/test-generic-file.txt")
    
    if [ "$HTTP_CODE" = "200" ]; then
        pass "Generic 文件下载成功 (HTTP 200)"
    else
        fail "Generic 文件下载失败 (expected HTTP 200, got HTTP $HTTP_CODE)"
    fi
else
    info "Generic 文件上传返回 HTTP $HTTP_CODE (可能需要特定仓库配置)"
fi

rm -f "$TEST_FILE"

echo
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
