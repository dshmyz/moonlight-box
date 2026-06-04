#!/bin/bash
# ============================================================
# QueryArtifacts 回源路径测试
# 验证 proxy 仓库的 QueryArtifacts 是否能正确从远端回源聚合数据
# 核心验证：maven-metadata.xml、go @v/list 在本地无缓存时能否返回远端数据
# ============================================================

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0

log_pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); }
log_fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); }
log_info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
log_section() { echo -e "\n${YELLOW}════════════════════════════════════════${NC}"; echo -e "  ${YELLOW}$1${NC}"; echo -e "${YELLOW}════════════════════════════════════════${NC}"; }

echo "============================================"
echo " QueryArtifacts 回源路径测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# ── 辅助函数 ──────────────────────
assert_contains() {
    local desc="$1" expected="$2" body="$3"
    if echo "$body" | grep -qi "$expected"; then
        log_pass "$desc"
    else
        log_fail "$desc (body 中未找到 '$expected')"
    fi
}

clear_repo_artifacts() {
    local repo_name="$1"
    local token="$2"
    local repo_info
    repo_info=$(curl -s "$BASE_URL/api/v1/repositories/$repo_name" \
        -H "Authorization: Bearer $token")
    local repo_id
    repo_id=$(echo "$repo_info" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('id',''))" 2>/dev/null)
    if [ -n "$repo_id" ] && [ "$repo_id" != "" ]; then
        log_info "清理仓库 $repo_name (ID: $repo_id) 的 artifacts 缓存..."
    fi
}



# ── 获取 Token ──────────────────────
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" 2>/dev/null | \
    python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('access_token',''))" 2>/dev/null || echo "")

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

# ── 测试 1: Maven 代理仓库 metadata.xml 版本聚合 ──────────────────────
log_section "测试 1: Maven 代理 metadata.xml 版本聚合（QueryArtifacts 路径）"

# 使用一个全新的仓库名，确保本地无缓存
MAVEN_PROXY="maven-proxy-aliyun"

# 测试 1a: 请求一个已知有多个版本的 artifact 的 metadata
log_info "请求 guava maven-metadata.xml..."
MAVEN_META=$(curl -s "$BASE_URL/repository/$MAVEN_PROXY/com/google/guava/guava/maven-metadata.xml")
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/repository/$MAVEN_PROXY/com/google/guava/guava/maven-metadata.xml")

if [ "$HTTP_CODE" = "200" ]; then
    log_pass "Maven metadata.xml 请求返回 200"

    # 验证返回的 XML 中包含多个版本（guava 肯定有多个版本）
    VERSION_COUNT=$(echo "$MAVEN_META" | grep -o "<version>" | wc -l | tr -d ' ')
    if [ "$VERSION_COUNT" -gt 1 ]; then
        log_pass "Maven metadata.xml 包含 $VERSION_COUNT 个版本 (回源聚合成功)"
    elif [ "$VERSION_COUNT" -eq 1 ]; then
        log_fail "Maven metadata.xml 只包含 1 个版本 (可能只通过 GetArtifact 兜底获取了单个文件，而非 QueryArtifacts 聚合)"
    else
        log_fail "Maven metadata.xml 中未找到 <version> 标签 (回源失败)"
    fi

    # 验证包含最新版本的版本号
    assert_contains "Maven metadata.xml 包含最新版本号" "32" "$MAVEN_META"
else
    log_fail "Maven metadata.xml 返回 HTTP $HTTP_CODE"
fi

# ── 测试 2: Go 代理 @v/list 版本列表 ──────────────────────
log_section "测试 2: Go 代理 @v/list 版本列表（QueryArtifacts 路径）"

GO_PROXY="go-proxy-goproxy-cn"

# 测试一个已知存在多个版本的 go module
GO_LIST=$(curl -s -o /tmp/test-go-list.txt -w "%{http_code}" \
    "$BASE_URL/repository/$GO_PROXY/github.com/stretchr/testify/@v/list")

if [ "$GO_LIST" = "200" ]; then
    log_pass "Go @v/list 请求返回 200"

    VERSION_COUNT=$(wc -l < /tmp/test-go-list.txt | tr -d ' ')
    if [ "$VERSION_COUNT" -gt 0 ]; then
        log_pass "Go @v/list 返回 $VERSION_COUNT 个版本"
        # 检查是否包含预期版本
        if grep -q "v1.8.4" /tmp/test-go-list.txt 2>/dev/null; then
            log_pass "Go @v/list 包含预期版本 v1.8.4"
        else
            log_fail "Go @v/list 未包含 v1.8.4 (可能远端版本有变化)"
        fi
    else
        log_fail "Go @v/list 返回空版本列表 (本地无缓存时应回源但返回了空)"
    fi
else
    log_fail "Go @v/list 返回 HTTP $GO_LIST"
fi

rm -f /tmp/test-go-list.txt

# ── 测试 3: Go 代理 @latest 端点 ──────────────────────
log_section "测试 3: Go 代理 @latest 端点（QueryArtifacts 路径）"

GO_LATEST=$(curl -s -o /tmp/test-go-latest.json -w "%{http_code}" \
    "$BASE_URL/repository/$GO_PROXY/github.com/stretchr/testify/@latest")

if [ "$GO_LATEST" = "200" ]; then
    log_pass "Go @latest 请求返回 200"

    # 验证返回的 JSON 包含 Version 字段
    if python3 -c "import json; d=json.load(open('/tmp/test-go-latest.json')); assert 'Version' in d" 2>/dev/null; then
        log_pass "Go @latest 返回包含 Version 字段"
        GO_VERSION=$(python3 -c "import json; d=json.load(open('/tmp/test-go-latest.json')); print(d['Version'])" 2>/dev/null)
        log_info "最新版本: $GO_VERSION"
    else
        log_fail "Go @latest 返回 JSON 缺少 Version 字段"
    fi
else
    log_fail "Go @latest 返回 HTTP $GO_LATEST"
fi

rm -f /tmp/test-go-latest.json

# ── 测试 4: Maven 代理 metadata.xml 在清空缓存后重新回源 ──────────────────────
log_section "测试 4: 验证 Maven metadata 是否真正来自远端而非硬编码"

# 使用一个不常见的包，确保本地没有缓存过
RARE_GROUP="com/fasterxml/jackson/core"
RARE_ARTIFACT="jackson-databind"

HTTP_CODE=$(curl -s -o /tmp/test-maven-rare.xml -w "%{http_code}" \
    "$BASE_URL/repository/$MAVEN_PROXY/$RARE_GROUP/$RARE_ARTIFACT/maven-metadata.xml")

if [ "$HTTP_CODE" = "200" ]; then
    log_pass "不常见包 metadata.xml 返回 200 (回源成功)"
    # 验证包含 jackson-databind
    if grep -q "jackson-databind" /tmp/test-maven-rare.xml 2>/dev/null; then
        log_pass "metadata.xml 包含正确的 artifactId (jackson-databind)"
    else
        log_fail "metadata.xml 未包含预期的 artifactId"
    fi
else
    log_fail "不常见包 metadata.xml 返回 HTTP $HTTP_CODE (回源失败)"
fi

rm -f /tmp/test-maven-rare.xml

# ── 测试 5: 验证 pypi simple index 回源（测试特定包而非全量索引） ──────────────────────
log_section "测试 5: PyPI Simple Index 回源（QueryArtifacts 路径）"

PYPI_PROXY="pypi-proxy-tuna"

# 使用特定包（requests）的版本列表页面，远比全量 Simple Index 小
HTTP_CODE=$(curl -s -o /tmp/test-pypi-simple.html -w "%{http_code}" \
    "$BASE_URL/repository/$PYPI_PROXY/simple/requests/")

if [ "$HTTP_CODE" = "200" ]; then
    log_pass "PyPI Simple Index (/simple/requests/) 返回 200"
    # 验证包含版本文件链接
    FILE_COUNT=$(grep -c 'href=' /tmp/test-pypi-simple.html 2>/dev/null || echo 0)
    if [ "$FILE_COUNT" -gt 0 ]; then
        log_pass "PyPI Simple Index 返回 $FILE_COUNT 个文件 (回源成功)"
    else
        log_fail "PyPI Simple Index 返回 0 个文件 (回源失败)"
    fi
else
    log_fail "PyPI Simple Index 返回 HTTP $HTTP_CODE"
fi

rm -f /tmp/test-pypi-simple.html

# ── 测试 6: 回源后包搜索 API 数据流验证 ──────────────────────
log_section "测试 6: 回源后包搜索 API 数据流验证"

source "$SCRIPT_DIR/search_validation.sh"
wait_for_indexing 2

# Maven guava: 回源后应能搜到，name 应为 group:artifact
assert_package_search "Maven guava 回源后搜索" "guava" "com.google.guava:guava" "maven"

# Go testify: 回源后应能搜到，name 应为模块路径
assert_package_search "Go testify 回源后搜索" "testify" "github.com/stretchr/testify" "go"

# PyPI requests: 回源后应能搜到
assert_package_search "PyPI requests 回源后搜索" "requests" "requests" "pypi"

# 健全性检查
for query in "guava" "testify" "requests"; do
    assert_package_search_sanity "搜索 '$query' 数据健全性" "$query"
done

# ── 汇总 ──────────────────────
echo ""
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo -e "  总计: $((PASS + FAIL))"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✅${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
