#!/bin/bash
# ============================================================
# 数据准确性测试
# 验证代理回源后，包搜索 API 返回的数据是否正确
# 重点：name 字段不为空、不被误填为 version
# ============================================================

set -e

BASE_URL="${1:-http://localhost:9081}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASS=0
FAIL=0
WARN=0
TOTAL=0

log_pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); TOTAL=$((TOTAL + 1)); }
log_fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); TOTAL=$((TOTAL + 1)); }
log_warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; WARN=$((WARN + 1)); }
log_info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }

# 等待批量写入落盘
sleep 2

# ============================================================
search_packages() {
    local query="$1"
    curl -s "$BASE_URL/api/v1/packages/search?q=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$query'))")&page_size=50"
}

# 验证搜索结果中包含正确的 name
# 参数：查询词、期望的 name、协议类型、描述
assert_search_name() {
    local query="$1"
    local expected_name="$2"
    local pkg_type="$3"
    local desc="$4"

    local body
    body=$(search_packages "$query")

    # 解析 list 中的 name 字段
    local found_name
    found_name=$(echo "$body" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    for item in items:
        if item.get('name') == '$expected_name':
            print(item['name'])
            sys.exit(0)
    # 没找到精确匹配，列出所有 name 供调试
    names = [item.get('name', '') for item in items]
    print('NOT_FOUND:' + '|'.join(names), file=sys.stderr)
    sys.exit(1)
except Exception as e:
    print('ERROR:' + str(e), file=sys.stderr)
    sys.exit(2)
" 2>/tmp/search_debug.log)

    if [ $? -eq 0 ] && [ "$found_name" = "$expected_name" ]; then
        log_pass "$desc: name = '$expected_name'"
    else
        local debug_info=$(cat /tmp/search_debug.log 2>/dev/null || echo "")
        log_fail "$desc: 期望 name='$expected_name', 实际: $debug_info"
    fi
}

# 验证搜索结果中 name 不为空且不是版本号格式
# 参数：查询词、协议类型、描述
assert_search_name_not_empty() {
    local query="$1"
    local pkg_type="$2"
    local desc="$3"

    local body
    body=$(search_packages "$query")

    local result
    result=$(echo "$body" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    if not items:
        print('EMPTY_LIST')
        sys.exit(1)
    for item in items:
        name = item.get('name', '')
        if not name:
            print('NAME_IS_EMPTY')
            sys.exit(1)
        # 检查 name 是否看起来像纯版本号（常见 bug）
        import re
        if re.match(r'^[0-9]+\.[0-9]+', name):
            print('NAME_LOOKS_LIKE_VERSION:' + name)
            sys.exit(1)
    print('OK')
    sys.exit(0)
except Exception as e:
    print('ERROR:' + str(e), file=sys.stderr)
    sys.exit(2)
" 2>/tmp/search_debug.log)

    if [ "$result" = "OK" ]; then
        log_pass "$desc: name 字段正确"
    elif [ "$result" = "EMPTY_LIST" ]; then
        log_warn "$desc: 搜索结果为空（可能包尚未被索引）"
    else
        log_fail "$desc: $result"
    fi
}

# ============================================================
log_section() {
    echo -e "\n${YELLOW}════════════════════════════════════════${NC}"
    echo -e "  ${YELLOW}$1${NC}"
    echo -e "${YELLOW}════════════════════════════════════════${NC}"
}

echo "============================================"
echo " 数据准确性测试"
echo " 验证包搜索 API 的数据完整性"
echo " 目标: $BASE_URL"
echo "============================================"

# 先触发一次代理回源，确保数据已写入
log_info "触发代理回源，确保数据已落盘..."

# Maven
curl -s -o /dev/null "$BASE_URL/repository/maven-proxy-aliyun/com/google/guava/guava/32.1.3-jre/guava-32.1.3-jre.jar" 2>/dev/null || true
curl -s -o /dev/null "$BASE_URL/repository/maven-proxy-aliyun/com/google/guava/guava/maven-metadata.xml" 2>/dev/null || true

# NPM
curl -s -o /dev/null "$BASE_URL/repository/npm-proxy-cn/lodash" 2>/dev/null || true

# PyPI
curl -s -o /dev/null "$BASE_URL/repository/pypi-proxy-tuna/simple/requests/" 2>/dev/null || true

# Go
curl -s -o /dev/null "$BASE_URL/repository/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/list" 2>/dev/null || true

sleep 3

# ============================================================
log_section "测试 1: Maven 包搜索数据准确性"
# ============================================================

# 核心测试：guava 的 name 应该是 group:artifact，不是 version
assert_search_name "guava" "com.google.guava:guava" "maven" \
    "Maven guava: name 应为 'com.google.guava:guava'"

# 验证 jacoco-maven-plugin（之前出 bug 的案例）
assert_search_name "jacoco" "org.jacoco:jacoco-maven-plugin" "maven" \
    "Maven jacoco: name 应为 'org.jacoco:jacoco-maven-plugin'"

# ============================================================
log_section "测试 2: NPM 包搜索数据准确性"
# ============================================================

assert_search_name "lodash" "lodash" "npm" \
    "NPM lodash: name 应为 'lodash'"

# ============================================================
log_section "测试 3: PyPI 包搜索数据准确性"
# ============================================================

assert_search_name "requests" "requests" "pypi" \
    "PyPI requests: name 应为 'requests'"

# ============================================================
log_section "测试 4: Go 包搜索数据准确性"
# ============================================================

assert_search_name "testify" "github.com/stretchr/testify" "go" \
    "Go testify: name 应为 'github.com/stretchr/testify'"

# ============================================================
log_section "测试 5: 跨协议 name 格式通用性检查"
# ============================================================

# 对所有能搜到的包，检查 name 不为空且不像版本号
for query in "guava" "lodash" "requests" "testify"; do
    assert_search_name_not_empty "$query" "" \
        "搜索 '$query' 的结果"
done

# ============================================================
log_section "测试 6: 搜索结果数据完整性"
# ============================================================

# 验证搜索结果的必要字段不为空
BODY=$(curl -s "$BASE_URL/api/v1/packages/search?q=guava&page_size=5")
INTEGRITY=$(echo "$BODY" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    if not items:
        print('WARN:EMPTY')
        sys.exit(0)
    item = items[0]
    issues = []
    if not item.get('name'):
        issues.append('name is empty')
    if not item.get('format'):
        issues.append('format is empty')
    if not item.get('updated_at'):
        issues.append('updated_at is empty')
    if issues:
        print('FAIL:' + ', '.join(issues))
        sys.exit(1)
    print('OK:name=' + item['name'] + ',format=' + item['format'])
    sys.exit(0)
except Exception as e:
    print('ERROR:' + str(e))
    sys.exit(2)
")

if echo "$INTEGRITY" | grep -q "^OK:"; then
    log_pass "搜索结果数据完整性: $INTEGRITY"
elif echo "$INTEGRITY" | grep -q "^WARN:"; then
    log_warn "搜索结果为空（可能需要等待索引）"
else
    log_fail "搜索结果数据完整性: $INTEGRITY"
fi

# ============================================================
# 汇总
# ============================================================
echo ""
echo "============================================"
echo " 测试汇总"
echo "============================================"
echo -e "  ${GREEN}通过: $PASS${NC}"
echo -e "  ${RED}失败: $FAIL${NC}"
echo -e "  ${YELLOW}警告: $WARN${NC}"
echo -e "  总计: $TOTAL"
echo ""

# 输出统计供 run_all_tests.sh 解析
echo "通过: $PASS"
echo "失败: $FAIL"
echo "警告: $WARN"

if [ $FAIL -eq 0 ]; then
    echo -e "\n${GREEN}数据准确性测试全部通过!${NC}"
    exit 0
else
    echo -e "\n${RED}数据准确性测试有失败项!${NC}"
    exit 1
fi
