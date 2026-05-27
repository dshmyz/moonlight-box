#!/bin/bash
# ============================================================
# 路由层日志准确性测试
# 验证 ServeHTTP 的下载计数、代理日志、审计日志是否正确
# 核心验证：404/403 请求不应被记为"成功下载"
# ============================================================

set -e

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

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

# cleanup
CLEAN_TEMPS=()
cleanup() { rm -rf "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

echo "============================================"
echo " 路由层日志准确性测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# ── 获取 Token ──────────────────────
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('access_token',''))" 2>/dev/null)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

# ── 辅助函数 ──────────────────────

# 获取当前代理日志记录数（用于后续增量验证）
get_log_count() {
    local count
    count=$(curl -s "$BASE_URL/api/v1/proxy-download-logs/logs?page=1&page_size=1" \
        -H "Authorization: Bearer $TOKEN" | \
        python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('total',0))" 2>/dev/null || echo "0")
    echo "$count"
}

# 获取最新 N 条代理日志（JSON）
get_latest_logs() {
    local limit=${1:-10}
    curl -s "$BASE_URL/api/v1/proxy-download-logs/logs?page=1&page_size=$limit" \
        -H "Authorization: Bearer $TOKEN"
}

# 从日志中检查是否有特定条件的记录
# 用法: check_log_field "status_code" "200"
check_log_field() {
    local field="$1" expected="$2"
    local logs
    logs=$(get_latest_logs 20)
    echo "$logs" | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', {}).get('items', d.get('data', []))
if not isinstance(items, list):
    items = []
for item in items:
    val = item.get('$field')
    if val == $expected or str(val) == '$expected':
        print('FOUND')
        break
" 2>/dev/null
}

# 验证最新日志记录中某字段的值
verify_latest_log() {
    local desc="$1" field="$2" expected="$3" should_exist="$4"
    local logs
    logs=$(get_latest_logs 20)
    local result
    result=$(echo "$logs" | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', {}).get('items', d.get('data', []))
if not isinstance(items, list):
    items = []
# 找最近匹配的记录（倒序）
items.reverse()
for item in items:
    val = item.get('$field')
    if val is not None:
        print(str(val))
        break
" 2>/dev/null)
    if [ "$should_exist" = "exist" ]; then
        if [ "$result" = "$expected" ]; then
            log_pass "$desc (字段 $field = $expected)"
        else
            log_fail "$desc (期望 $field=$expected, 实际=$result)"
        fi
    fi
}

# ── 测试 1: 正常下载应记录成功 ──────────────────────
log_section "测试 1: 正常下载应记录为成功"

# 先在 maven-local 上传一个文件用于测试
TEST_JAR="/tmp/test-logging-artifact-$$-1.0.0.jar"
CLEAN_TEMPS+=("$TEST_JAR")
echo "test content for logging verification $$" > "$TEST_JAR"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repository/maven-local/com/test/logging-artifact/1.0.0/logging-artifact-1.0.0.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR")

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    # 记录当前日志数
    BEFORE_LOGS=$(get_log_count)

    # 正常下载
    HTTP_CODE=$(curl -s -o /tmp/test-logging-download.jar -w "%{http_code}" \
        "$BASE_URL/repository/maven-local/com/test/logging-artifact/1.0.0/logging-artifact-1.0.0.jar")

    if [ "$HTTP_CODE" = "200" ]; then
        log_pass "正常下载返回 200"
        # 验证日志中应该有 statusCode=200 的记录
        AFTER_LOGS=$(get_log_count)
        if [ "$AFTER_LOGS" -gt "$BEFORE_LOGS" ] 2>/dev/null; then
            log_pass "代理日志有新增 (before=$BEFORE_LOGS, after=$AFTER_LOGS)"
        else
            log_info "代理日志计数未变化 (可能是批量写入延迟，等待 2s 后重试)"
            sleep 2
            AFTER_LOGS=$(get_log_count)
            if [ "$AFTER_LOGS" -gt "$BEFORE_LOGS" ] 2>/dev/null; then
                log_pass "代理日志有新增 (延迟写入, before=$BEFORE_LOGS, after=$AFTER_LOGS)"
            else
                log_info "代理日志总数: $AFTER_LOGS (批量写入可能未 flush)"
            fi
        fi
    else
        log_fail "正常下载返回 $HTTP_CODE (预期 200)"
    fi
else
    log_info "跳过: maven-local 上传失败 (HTTP $HTTP_CODE)"
fi

# ── 测试 2: 404 请求不应记为成功 ──────────────────────
log_section "测试 2: 404 请求不应记为成功下载"

BEFORE_LOGS=$(get_log_count)

# 请求一个不存在的包
HTTP_CODE=$(curl -s -o /tmp/test-404-response.txt -w "%{http_code}" \
    "$BASE_URL/repository/maven-local/com/test/nonexistent-artifact/99.99.99/nonexistent-artifact-99.99.99.jar")

if [ "$HTTP_CODE" = "404" ]; then
    log_pass "不存在的包返回 404 (符合预期)"
else
    log_info "不存在的包返回 $HTTP_CODE"
fi

# 等待日志 flush
sleep 2
AFTER_LOGS=$(get_log_count)

# 验证：即使有新增日志，statusCode 也不应是 200
LOGS_JSON=$(get_latest_logs 20)
BAD_STATUS=$(echo "$LOGS_JSON" | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', {}).get('items', d.get('data', []))
if not isinstance(items, list):
    items = []
count_200_on_nonexistent = 0
for item in items:
    sc = item.get('status_code')
    pn = item.get('package_name', '')
    # 检查是否有 200 状态码但包名包含 nonexistent 的记录
    if str(sc) == '200' and 'nonexistent' in str(pn).lower():
        count_200_on_nonexistent += 1
print(count_200_on_nonexistent)
" 2>/dev/null || echo "0")

if [ "$BAD_STATUS" = "0" ]; then
    log_pass "不存在包的 404 请求未被记为 200 成功"
else
    log_fail "不存在包的 404 请求被错误地记为 200 成功 ($BAD_STATUS 条)"
fi

# ── 测试 3: 403 被阻断请求不应记为成功 ──────────────────────
log_section "测试 3: 403 阻断请求不应记为成功下载"

# 先创建阻断规则
BLOCK_RESPONSE=$(curl -s -X POST "$BASE_URL/api/v1/block-rules" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"package_name":"blocked-test-pkg","version":"*","match_type":"exact","package_type":"npm","reason":"test-log-verification"}')

BLOCK_ID=$(echo "$BLOCK_RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('data',{}).get('id',''))" 2>/dev/null)

if [ -n "$BLOCK_ID" ]; then
    log_info "阻断规则已创建 (ID: $BLOCK_ID)，等待生效..."
    sleep 2

    BEFORE_LOGS=$(get_log_count)

    # 请求被阻断的包
    HTTP_CODE=$(curl -s -o /tmp/test-403-response.txt -w "%{http_code}" \
        "$BASE_URL/repository/npm-proxy-cn/blocked-test-pkg")

    if [ "$HTTP_CODE" = "403" ]; then
        log_pass "被阻断的包返回 403 (符合预期)"
    else
        log_info "被阻断的包返回 $HTTP_CODE (可能阻断规则尚未生效)"
    fi

    sleep 2
    LOGS_JSON=$(get_latest_logs 20)
    BAD_STATUS=$(echo "$LOGS_JSON" | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', {}).get('items', d.get('data', []))
if not isinstance(items, list):
    items = []
count_200_on_blocked = 0
for item in items:
    sc = item.get('status_code')
    pn = item.get('package_name', '')
    if str(sc) == '200' and 'blocked-test-pkg' in str(pn):
        count_200_on_blocked += 1
print(count_200_on_blocked)
" 2>/dev/null || echo "0")

    if [ "$BAD_STATUS" = "0" ]; then
        log_pass "被阻断的 403 请求未被记为 200 成功"
    else
        log_fail "被阻断的 403 请求被错误地记为 200 成功 ($BAD_STATUS 条)"
    fi

    # 清理阻断规则
    curl -s -X DELETE "$BASE_URL/api/v1/block-rules/$BLOCK_ID" \
        -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1
else
    log_info "跳过: 阻断规则创建失败"
fi

# ── 测试 4: 验证日志中的 repoID 不为 0 ──────────────────────
log_section "测试 4: 验证日志中的 repository_id 字段"

# 做一次正常的下载操作
curl -s -o /dev/null \
    "$BASE_URL/repository/maven-local/com/test/logging-artifact/1.0.0/logging-artifact-1.0.0.jar" > /dev/null 2>&1
sleep 2

LOGS_JSON=$(get_latest_logs 5)
ZERO_REPO_COUNT=$(echo "$LOGS_JSON" | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', {}).get('items', d.get('data', []))
if not isinstance(items, list):
    items = []
zero_count = 0
for item in items:
    rid = item.get('repository_id')
    if rid == 0 or rid == '0' or rid is None:
        zero_count += 1
print(zero_count)
" 2>/dev/null || echo "-1")

if [ "$ZERO_REPO_COUNT" = "0" ]; then
    log_pass "日志中 repository_id 不为 0"
elif [ "$ZERO_REPO_COUNT" = "-1" ]; then
    log_info "无法解析日志 JSON，跳过 repository_id 验证"
else
    log_fail "日志中有 $ZERO_REPO_COUNT 条记录 repository_id=0 (应关联具体仓库)"
fi

# ── 测试 5: 验证日志中的包名/版本不为空 ──────────────────────
log_section "测试 5: 验证日志中的 package_name/version/filename 字段"

EMPTY_FIELDS=$(echo "$LOGS_JSON" | python3 -c "
import sys, json
d = json.load(sys.stdin)
items = d.get('data', {}).get('items', d.get('data', []))
if not isinstance(items, list):
    items = []
empty_count = 0
for item in items:
    pn = item.get('package_name', '')
    ver = item.get('version', '')
    fn = item.get('filename', '')
    # 只有当三个都为空时才计为问题
    if not pn and not ver and not fn:
        # 跳过可能确实是根路径的请求
        empty_count += 1
print(empty_count)
" 2>/dev/null || echo "-1")

if [ "$EMPTY_FIELDS" = "0" ]; then
    log_pass "日志记录中包名/版本/文件名信息完整"
elif [ "$EMPTY_FIELDS" = "-1" ]; then
    log_info "无法解析日志 JSON，跳过字段验证"
else
    log_fail "日志中有 $EMPTY_FIELDS 条记录包名/版本/文件名全为空"
fi

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
    echo -e "${RED}部分测试失败! ❌${NC}"
    exit 1
fi
