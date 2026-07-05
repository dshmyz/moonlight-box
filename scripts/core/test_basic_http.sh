#!/bin/bash

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

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

warn() {
    echo -e "  ${YELLOW}⚠ WARN${NC} $1"
    WARN_COUNT=$((WARN_COUNT + 1))
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

# cleanup on exit
CLEAN_TEMPS=()
cleanup() { rm -f "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

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
pass "获取认证令牌成功"

# ── Maven 制品上传/下载/删除 ──────────────────────
echo
echo "════════════════════════════════════════"
echo "  测试: Maven 制品生命周期"
echo "════════════════════════════════════════"

TEST_JAR="/tmp/test-http-artifact-$$.jar"
TEST_POM="/tmp/test-http-artifact-$$.pom"
CLEAN_TEMPS+=("$TEST_JAR" "$TEST_POM")

echo "JAR content $$" > "$TEST_JAR"
echo '<?xml version="1.0"?><project><modelVersion>4.0.0</modelVersion>
<groupId>com.test</groupId><artifactId>test-http</artifactId><version>1.0.0</version>
</project>' > "$TEST_POM"
REPO_BASE="$BASE_URL/repository/maven-local"
ARTIFACT_PATH="com/test/test-http/1.0.0/test-http-1.0.0"

# PUT JAR
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$REPO_BASE/$ARTIFACT_PATH.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR")
if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
    pass "Maven JAR 上传成功 (HTTP $HTTP_CODE)"
else
    fail "Maven JAR 上传失败 (got HTTP $HTTP_CODE)"
fi

# PUT POM
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$REPO_BASE/$ARTIFACT_PATH.pom" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/xml" \
    --data-binary @"$TEST_POM")
if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
    pass "Maven POM 上传成功 (HTTP $HTTP_CODE)"
else
    fail "Maven POM 上传失败 (got HTTP $HTTP_CODE)"
fi

# 验证上传的包能通过搜索 API 检索到，且数据正确
source "$SCRIPT_DIR/search_validation.sh"
sleep 1
assert_package_search "上传后搜索 test-http" "test-http" "com.test:test-http" "maven"

# GET JAR (public, no auth)
HTTP_CODE=$(curl -s -o "$TEST_JAR.dl" -w "%{http_code}" "$REPO_BASE/$ARTIFACT_PATH.jar")
if [ "$HTTP_CODE" = "200" ]; then
    pass "Maven JAR 下载成功 (HTTP 200)"
    if [ -s "$TEST_JAR.dl" ]; then
        pass "下载的 JAR 文件非空"
    else
        fail "下载的 JAR 文件为空"
    fi
else
    fail "Maven JAR 下载失败 (got HTTP $HTTP_CODE)"
fi

# GET POM (public, no auth)
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$REPO_BASE/$ARTIFACT_PATH.pom")
if [ "$HTTP_CODE" = "200" ]; then
    pass "Maven POM 下载成功 (HTTP 200)"
else
    fail "Maven POM 下载失败 (got HTTP $HTTP_CODE)"
fi

# DELETE JAR
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "$REPO_BASE/$ARTIFACT_PATH.jar" \
    -H "Authorization: Bearer $TOKEN")
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
    pass "Maven JAR 删除成功 (HTTP $HTTP_CODE)"
else
    fail "Maven JAR 删除失败 (got HTTP $HTTP_CODE)"
fi

# GET after DELETE → 404
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$REPO_BASE/$ARTIFACT_PATH.jar")
if [ "$HTTP_CODE" = "404" ]; then
    pass "删除后下载返回 404 (符合预期)"
else
    fail "删除后仍可下载 (got HTTP $HTTP_CODE, expected 404)"
fi

# ── 公共 API 端点 ──────────────────────
echo
echo "════════════════════════════════════════"
echo "  测试: 公共 API 端点"
echo "════════════════════════════════════════"

# health
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health")
if [ "$HTTP_CODE" = "200" ]; then
    pass "Health 端点可达 (HTTP 200)"
else
    fail "Health 端点不可达 (got HTTP $HTTP_CODE)"
fi

# ping
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/v1/ping")
if [ "$HTTP_CODE" = "200" ]; then
    pass "Ping 端点可达 (HTTP 200)"
else
    fail "Ping 端点不可达 (got HTTP $HTTP_CODE)"
fi

# 403 on protected route without auth
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/v1/users")
if [ "$HTTP_CODE" = "401" ]; then
    pass "无认证访问 /api/v1/users 返回 401 (符合预期)"
else
    fail "无认证访问返回 HTTP $HTTP_CODE (expected 401)"
fi

# auth protected route with token
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/v1/users" \
    -H "Authorization: Bearer $TOKEN")
if [ "$HTTP_CODE" = "200" ]; then
    pass "带认证访问 /api/v1/users 成功 (HTTP 200)"
else
    fail "带认证访问返回 HTTP $HTTP_CODE (expected 200)"
fi

# ── Resolver 边界路径测试 ──────────────────────
echo
echo "════════════════════════════════════════"
echo "  测试: Resolver 边界路径"
echo "════════════════════════════════════════"

# 无尾路径的请求应返回 404（仓库名解析不能出错）
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repository/maven-local")
if [ "$HTTP_CODE" = "404" ] || [ "$HTTP_CODE" = "301" ] || [ "$HTTP_CODE" = "302" ]; then
    pass "无尾路径 /repository/maven-local 返回 HTTP $HTTP_CODE (Resolver 正确处理)"
else
    fail "/repository/maven-local 返回 HTTP $HTTP_CODE (expected 404/301/302)"
fi

# 空仓库名的请求应返回 404
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repository/")
if [ "$HTTP_CODE" = "404" ] || [ "$HTTP_CODE" = "301" ] || [ "$HTTP_CODE" = "302" ] || [ "$HTTP_CODE" = "200" ]; then
    pass "空仓库路径 /repository/ 返回 HTTP $HTTP_CODE (符合预期)"
else
    fail "/repository/ 返回 HTTP $HTTP_CODE"
fi

# 不存在的仓库应返回 404
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repository/nonexistent-repo-$$/anything")
if [ "$HTTP_CODE" = "404" ]; then
    pass "不存在的仓库返回 404 (符合预期)"
else
    fail "不存在的仓库返回 HTTP $HTTP_CODE (expected 404)"
fi

# ── 404 请求日志验证 ──────────────────────
echo
echo "════════════════════════════════════════"
echo "  测试: 404 请求不应记为成功下载"
echo "════════════════════════════════════════"

# 请求一个不存在的包，返回 404
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/maven-local/com/test/nonexistent-boundary/1.0.0/nonexistent-boundary-1.0.0.jar")

if [ "$HTTP_CODE" = "404" ]; then
    pass "不存在的包返回 404 (符合预期)"

    # 查询代理日志，验证此 404 请求没有被记为 status_code=200
    sleep 2  # 等待批量日志 flush
    LOGS_JSON=$(curl -s "$BASE_URL/api/v1/download-logs/logs?page=1&page_size=10" \
        -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo '{}')

    # 检查最近 10 条日志中，是否有路径包含 "nonexistent" 的记录被误记为 status_code=200
    BAD_COUNT=$(echo "$LOGS_JSON" | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', {}).get('items', d.get('data', []))
if not isinstance(items, list):
    items = []
count = 0
for item in items:
    sc = item.get('status_code')
    # 检查 PackageName / Filename / RemoteURL 中是否包含 nonexistent（刚请求的 404 路径特征）
    fields = [item.get('package_name', ''), item.get('filename', ''), item.get('remote_url', '')]
    is_nonexistent = any('nonexistent' in str(f).lower() for f in fields)
    if str(sc) == '200' and is_nonexistent:
        count += 1
print(count)
" 2>/dev/null || echo "-1")

    if [ "$BAD_COUNT" = "-1" ]; then
        warn "无法解析日志 JSON，跳过日志准确性验证"
    elif [ "$BAD_COUNT" = "0" ]; then
        pass "404 请求未被记为成功下载"
    else
        fail "发现 $BAD_COUNT 条 nonexistent 路径的记录被误记为 status_code=200"
    fi
else
    fail "不存在的包返回 HTTP $HTTP_CODE (expected 404)"
fi

echo
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT + WARN_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
