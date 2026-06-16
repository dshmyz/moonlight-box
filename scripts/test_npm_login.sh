#!/bin/bash
# npm login 端点集成测试脚本
# 测试 npm v7+ (POST /-/v1/login) 和 npm v6 (PUT /-/user/org.couchdb.user:{name})

set -e

BASE_URL="${1:-http://localhost:9081}"
REPO_NAME="${2:-npm-hosted}"
TEST_USER="${3:-admin}"
TEST_PASS="${4:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; }
info() { echo -e "${YELLOW}[INFO]${NC} $1"; }

echo "========================================="
echo " npm Login 集成测试"
echo " Server:   $BASE_URL"
echo " Repo:     $REPO_NAME"
echo " User:     $TEST_USER"
echo "========================================="
echo ""

# 检查服务是否可达
info "检查服务是否可达..."
if ! curl -sf -o /dev/null "$BASE_URL/health"; then
    fail "服务不可达: $BASE_URL/health"
    echo "请先启动服务: make run"
    exit 1
fi
pass "服务可达"
echo ""

# ---- 测试 1: POST /-/v1/login (npm v7+) ----
info "测试 1: POST /-/v1/login (npm v7+ 登录端点)"
RESP=$(curl -s -w "\n%{http_code}" -X POST \
    "$BASE_URL/repository/$REPO_NAME/-/v1/login" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$TEST_USER\",\"password\":\"$TEST_PASS\"}")

HTTP_CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')

if [ "$HTTP_CODE" = "200" ]; then
    TOKEN=$(echo "$BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    OK=$(echo "$BODY" | grep -o '"ok":true')
    if [ -n "$TOKEN" ] && [ -n "$OK" ]; then
        pass "登录成功，获取到 token (长度: ${#TOKEN})"
    else
        fail "HTTP 200 但响应格式异常: $BODY"
    fi
else
    fail "HTTP $HTTP_CODE, 响应: $BODY"
fi
echo ""

# ---- 测试 2: POST /-/v1/login 错误密码 ----
info "测试 2: POST /-/v1/login 错误密码应返回 401"
RESP=$(curl -s -w "\n%{http_code}" -X POST \
    "$BASE_URL/repository/$REPO_NAME/-/v1/login" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$TEST_USER\",\"password\":\"wrongpassword\"}")

HTTP_CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')

if [ "$HTTP_CODE" = "401" ]; then
    pass "错误密码正确返回 401"
else
    fail "期望 401, 实际 HTTP $HTTP_CODE, 响应: $BODY"
fi
echo ""

# ---- 测试 3: POST /-/v1/login 缺少字段 ----
info "测试 3: POST /-/v1/login 缺少必填字段应返回 400"
RESP=$(curl -s -w "\n%{http_code}" -X POST \
    "$BASE_URL/repository/$REPO_NAME/-/v1/login" \
    -H "Content-Type: application/json" \
    -d '{"name":"","password":""}')

HTTP_CODE=$(echo "$RESP" | tail -1)

if [ "$HTTP_CODE" = "400" ]; then
    pass "缺少字段正确返回 400"
else
    fail "期望 400, 实际 HTTP $HTTP_CODE"
fi
echo ""

# ---- 测试 4: PUT /-/user/org.couchdb.user:{name} (npm v6 adduser) ----
info "测试 4: PUT /-/user/org.couchdb.user:{name} (npm v6 adduser 端点)"
RESP=$(curl -s -w "\n%{http_code}" -X PUT \
    "$BASE_URL/repository/$REPO_NAME/-/user/org.couchdb.user:$TEST_USER" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$TEST_USER\",\"password\":\"$TEST_PASS\",\"email\":\"$TEST_USER@test.com\"}")

HTTP_CODE=$(echo "$RESP" | tail -1)
BODY=$(echo "$RESP" | sed '$d')

if [ "$HTTP_CODE" = "201" ]; then
    TOKEN=$(echo "$BODY" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
    ID=$(echo "$BODY" | grep -o '"id":"[^"]*"' | cut -d'"' -f4)
    REV=$(echo "$BODY" | grep -o '"rev":"[^"]*"' | cut -d'"' -f4)
    OK=$(echo "$BODY" | grep -o '"ok":true')
    if [ -n "$TOKEN" ] && [ -n "$ID" ] && [ -n "$REV" ] && [ -n "$OK" ]; then
        pass "adduser 成功, id=$ID, rev=$REV, token 长度=${#TOKEN}"
    else
        fail "HTTP 201 但响应格式异常: $BODY"
    fi
else
    fail "期望 201, 实际 HTTP $HTTP_CODE, 响应: $BODY"
fi
echo ""

# ---- 测试 5: PUT adduser 错误密码 ----
info "测试 5: PUT /-/user/org.couchdb.user:{name} 错误密码应返回 401"
RESP=$(curl -s -w "\n%{http_code}" -X PUT \
    "$BASE_URL/repository/$REPO_NAME/-/user/org.couchdb.user:$TEST_USER" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$TEST_USER\",\"password\":\"wrongpassword\"}")

HTTP_CODE=$(echo "$RESP" | tail -1)

if [ "$HTTP_CODE" = "401" ]; then
    pass "错误密码正确返回 401"
else
    fail "期望 401, 实际 HTTP $HTTP_CODE"
fi
echo ""

# ---- 测试 6: 用获取的 token 访问包列表 ----
info "测试 6: 用获取的 token 访问仓库 (验证 token 有效性)"
# 先重新登录获取 token
RESP=$(curl -s -X POST \
    "$BASE_URL/repository/$REPO_NAME/-/v1/login" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$TEST_USER\",\"password\":\"$TEST_PASS\"}")
TOKEN=$(echo "$RESP" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -n "$TOKEN" ]; then
    RESP2=$(curl -s -w "\n%{http_code}" \
        "$BASE_URL/repository/$REPO_NAME/-/all" \
        -H "Authorization: Bearer $TOKEN")
    HTTP_CODE2=$(echo "$RESP2" | tail -1)
    if [ "$HTTP_CODE2" = "200" ]; then
        pass "Token 有效，成功访问包列表"
    else
        # 仓库可能为空返回 404，但不是 401 就说明 token 有效
        if [ "$HTTP_CODE2" != "401" ]; then
            pass "Token 有效 (HTTP $HTTP_CODE2, 非 401)"
        else
            fail "Token 无效，返回 401"
        fi
    fi
else
    fail "无法获取 token，跳过 token 验证"
fi
echo ""

# ---- 测试 7: Nexus2 兼容路由 ----
info "测试 7: Nexus2 兼容路由 /content/repositories/:name/-/v1/login"
RESP=$(curl -s -w "\n%{http_code}" -X POST \
    "$BASE_URL/content/repositories/$REPO_NAME/-/v1/login" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$TEST_USER\",\"password\":\"$TEST_PASS\"}")

HTTP_CODE=$(echo "$RESP" | tail -1)

if [ "$HTTP_CODE" = "200" ]; then
    pass "Nexus2 路由登录成功"
else
    BODY=$(echo "$RESP" | sed '$d')
    fail "Nexus2 路由登录失败, HTTP $HTTP_CODE, 响应: $BODY"
fi
echo ""

# ---- 测试 8: Group 兼容路由 ----
info "测试 8: Group 兼容路由 /content/groups/:name/-/v1/login"
RESP=$(curl -s -w "\n%{http_code}" -X POST \
    "$BASE_URL/content/groups/$REPO_NAME/-/v1/login" \
    -H "Content-Type: application/json" \
    -d "{\"name\":\"$TEST_USER\",\"password\":\"$TEST_PASS\"}")

HTTP_CODE=$(echo "$RESP" | tail -1)

if [ "$HTTP_CODE" = "200" ]; then
    pass "Group 路由登录成功"
else
    BODY=$(echo "$RESP" | sed '$d')
    # Group 路由可能没有对应仓库，404 是合理的
    if [ "$HTTP_CODE" = "404" ]; then
        pass "Group 路由可达 (仓库不存在返回 404，符合预期)"
    else
        fail "Group 路由异常, HTTP $HTTP_CODE, 响应: $BODY"
    fi
fi
echo ""

echo "========================================="
echo " 测试完成"
echo "========================================="
