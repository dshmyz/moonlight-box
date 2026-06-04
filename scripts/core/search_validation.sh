#!/bin/bash
# ============================================================
# 包搜索数据验证函数库
# 供其他测试脚本 source 引用
# 用法: source "$SCRIPT_DIR/search_validation.sh"
# ============================================================

# 兼容各脚本不同的输出函数命名
# test_basic_http.sh: pass/fail/warn/info
# test_queryartifacts.sh / test_all_proxy.sh: log_pass/log_fail/log_info
_sv_pass() { if type -t pass &>/dev/null; then pass "$@"; elif type -t log_pass &>/dev/null; then log_pass "$@"; else echo "  ✓ $1"; fi; }
_sv_fail() { if type -t fail &>/dev/null; then fail "$@"; elif type -t log_fail &>/dev/null; then log_fail "$@"; else echo "  ✗ $1"; fi; }
_sv_warn() { if type -t warn &>/dev/null; then warn "$@"; elif type -t log_warn &>/dev/null; then log_warn "$@"; else echo "  ⚠ $1"; fi; }
_sv_info() { if type -t info &>/dev/null; then info "$@"; elif type -t log_info &>/dev/null; then log_info "$@"; else echo "  ℹ $1"; fi; }

# 验证包搜索 API 能找到指定包，并且 name 字段正确
# 参数：
#   $1 - 描述
#   $2 - 搜索关键词
#   $3 - 期望的 name
#   $4 - (可选) 期望的 format
assert_package_search() {
    local desc="$1"
    local query="$2"
    local expected_name="$3"
    local expected_format="${4:-}"

    local encoded_query
    encoded_query=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$query'))")
    local body
    body=$(curl -s "$BASE_URL/api/v1/packages/search?q=$encoded_query&page_size=50")

    local result
    result=$(echo "$body" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    if not items:
        print('EMPTY_LIST')
        sys.exit(0)
    for item in items:
        name = item.get('name', '')
        if name == '$expected_name':
            fmt = item.get('format', '')
            if '$expected_format' and fmt != '$expected_format':
                print('FORMAT_MISMATCH:' + fmt)
                sys.exit(1)
            print('OK:' + name)
            sys.exit(0)
    names = [item.get('name', '') for item in items[:3]]
    print('NOT_FOUND:' + '|'.join(names))
except Exception as e:
    print('ERROR:' + str(e))
")

    case "$result" in
        OK:*)
            _sv_pass "$desc: name='${expected_name}' 可搜索到"
            ;;
        EMPTY_LIST)
            _sv_warn "$desc: 搜索结果为空（可能需要等待索引）"
            ;;
        NOT_FOUND:*)
            local found="${result#NOT_FOUND:}"
            _sv_fail "$desc: 期望 name='${expected_name}', 搜到: ${found}"
            ;;
        FORMAT_MISMATCH:*)
            local actual="${result#FORMAT_MISMATCH:}"
            _sv_fail "$desc: format 不匹配, 期望 '${expected_format}', 实际 '${actual}'"
            ;;
        *)
            _sv_fail "$desc: 搜索 API 返回异常: $result"
            ;;
    esac
}

# 验证包搜索 API 结果中 name 字段不为空且不像版本号
assert_package_search_sanity() {
    local desc="$1"
    local query="$2"

    local encoded_query
    encoded_query=$(python3 -c "import urllib.parse; print(urllib.parse.quote('$query'))")
    local body
    body=$(curl -s "$BASE_URL/api/v1/packages/search?q=$encoded_query&page_size=10")

    local result
    result=$(echo "$body" | python3 -c "
import sys, json, re
try:
    data = json.load(sys.stdin)
    items = data.get('data', {}).get('list', [])
    if not items:
        print('EMPTY')
        sys.exit(0)
    issues = []
    for item in items:
        name = item.get('name', '')
        if not name:
            issues.append('name为空')
            continue
        if re.match(r'^[0-9]+\.[0-9]+', name):
            issues.append('name像版本号:' + name)
    if issues:
        print('FAIL:' + '; '.join(issues))
        sys.exit(1)
    print('OK:' + str(len(items)))
except Exception as e:
    print('ERROR:' + str(e))
")

    case "$result" in
        OK:*)
            pass "$desc: 搜索结果数据正常 (${result#OK:} 条)"
            ;;
        EMPTY)
            info "$desc: 搜索结果为空"
            ;;
        FAIL:*)
            fail "$desc: ${result#FAIL:}"
            ;;
        *)
            fail "$desc: 异常: $result"
            ;;
    esac
}

# 等待批量写入落盘（用于代理回源后等待数据可搜索）
wait_for_indexing() {
    local seconds="${1:-3}"
    log_info "等待 ${seconds}s 索引落盘..."
    sleep "$seconds"
}
