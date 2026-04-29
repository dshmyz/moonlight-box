#!/bin/bash
# ============================================================
# 多协议代理流程端到端测试脚本
# 测试 NPM / Maven / PyPI / Yum / Go 代理功能是否正常工作
# ============================================================

set -e

BASE_URL="http://localhost:9081"
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

PASS=0
FAIL=0
TOTAL=0

log_pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); }
log_fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); TOTAL=$((TOTAL + 1)); }
log_info() { echo -e "  ${YELLOW}ℹ INFO${NC} $1"; }

assert_status() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$actual" = "$expected" ]; then log_pass "$desc (HTTP $actual)"; else log_fail "$desc (expected HTTP $expected, got HTTP $actual)"; fi
}

assert_body_contains() {
    local desc="$1" expected="$2" body="$3"
    if echo "$body" | grep -qi "$expected"; then log_pass "$desc"; else log_fail "$desc (body missing '$expected')"; fi
}

echo "============================================"
echo " 多协议代理流程端到端测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo ""

# ============================================================
echo "📦 测试 1: NPM 代理回源"
# ============================================================
# 测试 npm 元数据请求
HTTP_CODE=$(curl -s -o /tmp/npm_meta.json -w "%{http_code}" "$BASE_URL/npm/lodash")
assert_status "NPM 元数据请求 (lodash)" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "返回 JSON 包含 name" '"name"' "$(cat /tmp/npm_meta.json)"
fi

# 测试 npm 作用域包
HTTP_CODE=$(curl -s -o /tmp/npm_scope.json -w "%{http_code}" "$BASE_URL/npm/@babel/core")
assert_status "NPM 作用域包 (@babel/core)" "200" "$HTTP_CODE"

# 测试 npm tarball 下载
HTTP_CODE=$(curl -s -o /tmp/npm_tgz.tgz -w "%{http_code}" "$BASE_URL/npm/lodash/-/tarball/lodash-4.17.21.tgz")
assert_status "NPM tarball 下载" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    FILE_SIZE=$(stat -c%s /tmp/npm_tgz.tgz 2>/dev/null || stat -f%z /tmp/npm_tgz.tgz 2>/dev/null || echo "unknown")
    log_info "NPM tarball 文件大小: ${FILE_SIZE} bytes"
fi

echo ""
echo "📦 测试 2: PyPI 代理回源"
# ============================================================
# 测试 PyPI Simple Index
HTTP_CODE=$(curl -s -o /tmp/pypi_simple.html -w "%{http_code}" "$BASE_URL/pypi/simple/requests/")
assert_status "PyPI Simple Index (requests)" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "PyPI 返回 HTML 包含 links" "href=" "$(cat /tmp/pypi_simple.html)"
fi

# 测试 PyPI JSON API
HTTP_CODE=$(curl -s -o /tmp/pypi_json.json -w "%{http_code}" "$BASE_URL/pypi/requests/2.31.0/json")
assert_status "PyPI JSON API (requests 2.31.0)" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "PyPI JSON 包含 info" '"info"' "$(cat /tmp/pypi_json.json)"
fi

echo ""
echo "📦 测试 3: Maven 代理回源"
# ============================================================
# 测试 Maven jar 下载
HTTP_CODE=$(curl -s -o /tmp/maven_jar.jar -w "%{http_code}" "$BASE_URL/maven2/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar")
assert_status "Maven jar 下载 (guava)" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    FILE_SIZE=$(stat -c%s /tmp/maven_jar.jar 2>/dev/null || stat -f%z /tmp/maven_jar.jar 2>/dev/null || echo "unknown")
    log_info "Maven jar 文件大小: ${FILE_SIZE} bytes"
fi

# 测试 Maven metadata.xml
HTTP_CODE=$(curl -s -o /tmp/maven_meta.xml -w "%{http_code}" "$BASE_URL/maven2/com/google/guava/guava/maven-metadata.xml")
assert_status "Maven metadata.xml" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "Maven metadata 包含 version" "version" "$(cat /tmp/maven_meta.xml)"
fi

echo ""
echo "📦 测试 4: Yum 代理回源"
# ============================================================
# 测试 Yum repomd.xml (代理回源)
HTTP_CODE=$(curl -s -o /tmp/yum_repomd.xml -w "%{http_code}" "$BASE_URL/yum/baseos/repodata/repomd.xml")
assert_status "Yum repomd.xml 下载" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "Yum repomd.xml 包含 xml" "repomd" "$(cat /tmp/yum_repomd.xml)"
fi

echo ""
echo "📦 测试 5: Go 代理回源"
# ============================================================
# 测试 Go @v/list
HTTP_CODE=$(curl -s -o /tmp/go_list.txt -w "%{http_code}" "$BASE_URL/go/github.com/stretchr/testify/@v/list")
assert_status "Go @v/list" "200" "$HTTP_CODE"

# 测试 Go @v/VERSION.info
HTTP_CODE=$(curl -s -o /tmp/go_info.json -w "%{http_code}" "$BASE_URL/go/github.com/stretchr/testify/@v/v1.8.4.info")
assert_status "Go .info 文件" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "Go .info 包含 Version" '"Version"' "$(cat /tmp/go_info.json)"
fi

# 测试 Go @v/VERSION.mod
HTTP_CODE=$(curl -s -o /tmp/go_mod.txt -w "%{http_code}" "$BASE_URL/go/github.com/stretchr/testify/@v/v1.8.4.mod")
assert_status "Go .mod 文件" "200" "$HTTP_CODE"
if [ "$HTTP_CODE" = "200" ]; then
    assert_body_contains "Go .mod 包含 module" "module" "$(cat /tmp/go_mod.txt)"
fi

echo ""
echo "📦 测试 6: 数据库验证 - 代理仓库配置"
# ============================================================
for pkg_type in npm maven pypi go yum; do
    LOCAL=$(sqlite3 ./data/registry.db "SELECT COUNT(*) FROM repositories WHERE type='local' AND package_type='$pkg_type';")
    PROXY=$(sqlite3 ./data/registry.db "SELECT COUNT(*) FROM repositories WHERE type='proxy' AND package_type='$pkg_type';")
    VIRTUAL=$(sqlite3 ./data/registry.db "SELECT COUNT(*) FROM repositories WHERE type='virtual' AND package_type='$pkg_type';")
    MEMBERS=$(sqlite3 ./data/registry.db "SELECT COUNT(*) FROM repository_groups rg JOIN repositories r ON rg.member_repo_id=r.id JOIN repositories vr ON rg.virtual_repo_id=vr.id WHERE vr.package_type='$pkg_type';")
    log_info "$pkg_type: local=$LOCAL proxy=$PROXY virtual=$VIRTUAL members=$MEMBERS"
done

echo ""
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo -e "  总计: $TOTAL"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✅${NC}"
    exit 0
else
    echo -e "${RED}部分测试失败! ❌${NC}"
    exit 1
fi
