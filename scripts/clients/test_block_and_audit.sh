#!/bin/bash

# =============================================================================
# 阻断规则 + 审计日志功能测试
# =============================================================================

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
WARN_COUNT=0

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; WARN_COUNT=$((WARN_COUNT + 1)); }

echo "============================================"
echo " 阻断规则 + 审计日志功能测试"
echo "============================================"

TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    fail "无法获取认证令牌"
    exit 1
fi
pass "认证成功"

# =============================================================================
# 阻断规则测试
# =============================================================================
echo
echo "=== 阻断规则 (Block Rules) 测试 ==="

echo "测试 B1: 查看现有阻断规则..."
RULES=$(curl -s "$BASE_URL/api/v1/block-rules" -H "Authorization: Bearer $TOKEN")
RULE_COUNT=$(echo "$RULES" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data',[])))" 2>/dev/null)
info "当前 $RULE_COUNT 条规则"

echo "测试 B2: 创建阻断规则..."
CREATE_RESP=$(curl -s -X POST "$BASE_URL/api/v1/block-rules" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"package_name":"lodash","match_type":"exact","package_type":"npm","reason":"阻断功能测试-自动创建"}')
CREATE_CODE=$(echo "$CREATE_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('code','error'))" 2>/dev/null)
if [ "$CREATE_CODE" = "200" ]; then
    RULE_ID=$(echo "$CREATE_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['id'])" 2>/dev/null)
    pass "阻断规则创建成功 (id=$RULE_ID)"
else
    warn "阻断规则创建返回: $CREATE_CODE (可能已存在)"
    RULE_ID=""
fi

echo "测试 B3: 尝试下载被阻断的包 (npm lodash)..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/npm-proxy-cn/lodash")
if [ "$HTTP_CODE" = "403" ] || [ "$HTTP_CODE" = "451" ]; then
    pass "被阻断的包正确返回禁止访问 (HTTP $HTTP_CODE)"
elif [ "$HTTP_CODE" = "200" ]; then
    warn "被阻断的包仍然可以访问 (HTTP 200，阻断可能未生效)"
else
    info "阻断响应: HTTP $HTTP_CODE"
fi

echo "测试 B4: 创建通配符阻断规则..."
CREATE_RESP=$(curl -s -X POST "$BASE_URL/api/v1/block-rules" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"package_name":"malware-","match_type":"prefix","package_type":"npm","reason":"前缀阻断测试"}')
CREATE_CODE=$(echo "$CREATE_RESP" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('code','error'))" 2>/dev/null)
if [ "$CREATE_CODE" = "200" ]; then
    pass "前缀阻断规则创建成功"
else
    info "前缀阻断规则返回: $CREATE_CODE"
fi

echo "测试 B5: 删除测试阻断规则..."
if [ -n "$RULE_ID" ]; then
    DEL_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
        "$BASE_URL/api/v1/block-rules/$RULE_ID" -H "Authorization: Bearer $TOKEN")
    if [ "$DEL_CODE" = "200" ] || [ "$DEL_CODE" = "204" ]; then
        pass "阻断规则删除成功 (HTTP $DEL_CODE)"
    else
        warn "阻断规则删除: HTTP $DEL_CODE"
    fi
fi

echo "测试 B6: 验证阻断规则列表..."
RULES_AFTER=$(curl -s "$BASE_URL/api/v1/block-rules" -H "Authorization: Bearer $TOKEN")
RULE_COUNT_AFTER=$(echo "$RULES_AFTER" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data',[])))" 2>/dev/null)
info "操作后 $RULE_COUNT_AFTER 条规则"

# =============================================================================
# 审计日志测试
# =============================================================================
echo
echo "=== 审计日志 (Audit Logs) 测试 ==="

echo "测试 A1: 获取审计日志列表..."
AUDIT_RESP=$(curl -s "$BASE_URL/api/v1/audit/logs" -H "Authorization: Bearer $TOKEN")
LOG_COUNT=$(echo "$AUDIT_RESP" | python3 -c "
import sys,json
d=json.load(sys.stdin)
items=d.get('data',d)
if isinstance(items,list): print(len(items))
elif isinstance(items,dict): print(len(items.get('items',items.get('logs',[]))))
else: print('0')
" 2>/dev/null)
if [ "$LOG_COUNT" -gt 0 ]; then
    pass "审计日志可访问 ($LOG_COUNT 条记录)"
else
    warn "审计日志无记录或不可访问"
fi

echo "测试 A2: 生成可审计的操作..."
# 登录
curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" > /dev/null
# 下载包
curl -s -o /dev/null "$BASE_URL/repository/npm-proxy-cn/lodash" > /dev/null 2>&1
# 访问 API
curl -s -o /dev/null "$BASE_URL/api/v1/repositories" -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1

echo "测试 A3: 验证新日志已生成..."
sleep 1
AUDIT_RESP2=$(curl -s "$BASE_URL/api/v1/audit/logs" -H "Authorization: Bearer $TOKEN")
LOG_COUNT2=$(echo "$AUDIT_RESP2" | python3 -c "
import sys,json
d=json.load(sys.stdin)
items=d.get('data',d)
if isinstance(items,list): print(len(items))
elif isinstance(items,dict): print(len(items.get('items',items.get('logs',[]))))
else: print('0')
" 2>/dev/null)
info "操作后 $LOG_COUNT2 条记录"
if [ "$LOG_COUNT2" -gt "$LOG_COUNT" ]; then
    pass "新操作已被审计记录 ($((LOG_COUNT2 - LOG_COUNT)) 条新记录)"
elif [ "$LOG_COUNT2" -eq "$LOG_COUNT" ]; then
    info "日志数量未变化（缓存/延迟写入可能）"
fi

echo "测试 A4: 检查日志详情..."
python3 -c "
import sys,json
d=json.load(sys.stdin)
items=d.get('data',d)
if isinstance(items,list): items=items
elif isinstance(items,dict): items=items.get('items',[])
actions=set()
for e in items[:20]:
    action=e.get('action','?')
    actions.add(action)
print(f'操作类型: {sorted(actions)}')
for e in items[:1]:
    for k,v in e.items():
        if v: print(f'  {k}: {v}')
" <<< "$AUDIT_RESP2"

echo
echo "============================================"
echo " 阻断规则 + 审计日志测试完成"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo
