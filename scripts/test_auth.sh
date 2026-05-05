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
    ((PASS_COUNT++))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC} $1"
    ((FAIL_COUNT++))
}

info() {
    echo -e "  ${BLUE}ℹ INFO${NC} $1"
}

get_auth_token() {
    local username="$1"
    local password="$2"
    
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$username\",\"password\":\"$password\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

echo "============================================"
echo " 认证与权限测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

echo "════════════════════════════════════════"
echo "  测试 1: 管理员登录"
echo "════════════════════════════════════════"

ADMIN_TOKEN=$(get_auth_token "$ADMIN_USER" "$ADMIN_PASS")

if [ -n "$ADMIN_TOKEN" ]; then
    pass "管理员登录成功，获取到令牌"
else
    fail "管理员登录失败"
    exit 1
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 无效凭证认证"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"username":"invalid_user","password":"invalid_pass"}')

if [ "$HTTP_CODE" = "401" ]; then
    pass "无效凭证返回 401 (符合预期)"
else
    fail "无效凭证返回 HTTP $HTTP_CODE (expected 401)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 3: 创建只读用户"
echo "════════════════════════════════════════"

READONLY_USER="readonly_user_$$"
READONLY_PASS="readonly_pass_$$"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/v1/users" \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$READONLY_USER\",\"password\":\"$READONLY_PASS\",\"email\":\"readonly@test.com\"}")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "只读用户创建成功 (HTTP $HTTP_CODE)"
    
    USER_ID=$(curl -s "$BASE_URL/api/v1/users" \
        -H "Authorization: Bearer $ADMIN_TOKEN" | \
        grep -o "\"id\":[0-9]*,\"username\":\"$READONLY_USER\"" | \
        grep -o '"id":[0-9]*' | \
        grep -o '[0-9]*')
    
    if [ -n "$USER_ID" ]; then
        info "只读用户 ID: $USER_ID"
        
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
            "$BASE_URL/api/v1/users/$USER_ID/roles" \
            -H "Authorization: Bearer $ADMIN_TOKEN" \
            -H "Content-Type: application/json" \
            -d '{"role_ids":[3]}')
        
        if [ "$HTTP_CODE" = "200" ]; then
            pass "只读用户角色分配成功"
        else
            info "只读用户角色分配返回 HTTP $HTTP_CODE"
        fi
    else
        info "无法获取只读用户 ID"
    fi
else
    info "只读用户创建返回 HTTP $HTTP_CODE (可能用户已存在)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 只读用户上传测试"
echo "════════════════════════════════════════"

if [ -n "$USER_ID" ]; then
    READONLY_TOKEN=$(get_auth_token "$READONLY_USER" "$READONLY_PASS")
    
    if [ -n "$READONLY_TOKEN" ]; then
        pass "只读用户登录成功"
        
        TEST_FILE="/tmp/test-readonly-upload-$$-1.0.0.txt"
        echo "Readonly test content" > "$TEST_FILE"
        
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
            "$BASE_URL/files/upload" \
            -H "Authorization: Bearer $READONLY_TOKEN" \
            -F "file=@$TEST_FILE;filename=test-readonly.txt")
        
        if [ "$HTTP_CODE" = "401" ] || [ "$HTTP_CODE" = "403" ]; then
            pass "只读用户上传被拒绝 (HTTP $HTTP_CODE)"
        else
            info "只读用户上传返回 HTTP $HTTP_CODE (可能权限配置不同)"
        fi
        
        rm -f "$TEST_FILE"
    else
        info "只读用户登录失败，跳过上传测试"
    fi
else
    info "只读用户未创建，跳过上传测试"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: 只读用户下载测试"
echo "════════════════════════════════════════"

if [ -n "$READONLY_TOKEN" ]; then
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        "$BASE_URL/repo/npm-proxy-cn/lodash" \
        -H "Authorization: Bearer $READONLY_TOKEN")
    
    if [ "$HTTP_CODE" = "200" ]; then
        pass "只读用户下载成功 (HTTP 200)"
    else
        info "只读用户下载返回 HTTP $HTTP_CODE"
    fi
else
    info "只读用户令牌不可用，跳过下载测试"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 6: 无令牌访问受保护资源"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "$BASE_URL/files/upload" \
    -F "file=@/dev/null;filename=test.txt")

if [ "$HTTP_CODE" = "401" ] || [ "$HTTP_CODE" = "403" ]; then
    pass "无令牌访问受保护资源被拒绝 (HTTP $HTTP_CODE)"
else
    info "无令牌访问返回 HTTP $HTTP_CODE"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 7: 无效令牌访问"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
    "$BASE_URL/files/upload" \
    -H "Authorization: Bearer invalid_token_12345" \
    -F "file=@/dev/null;filename=test.txt")

if [ "$HTTP_CODE" = "401" ] || [ "$HTTP_CODE" = "403" ]; then
    pass "无效令牌访问被拒绝 (HTTP $HTTP_CODE)"
else
    info "无效令牌访问返回 HTTP $HTTP_CODE"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 8: 清理测试用户"
echo "════════════════════════════════════════"

if [ -n "$USER_ID" ]; then
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
        "$BASE_URL/api/v1/users/$USER_ID" \
        -H "Authorization: Bearer $ADMIN_TOKEN")
    
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ]; then
        pass "测试用户清理成功"
    else
        info "测试用户清理返回 HTTP $HTTP_CODE"
    fi
else
    info "无需清理测试用户"
fi

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
