#!/bin/bash

# =============================================================================
# API Token 客户端鉴权回归测试
# 验证 mlb_ 前缀 API token 在仓库写入路径（CI/CD 机器鉴权）上的完整生命周期：
#   1. 登录 admin → 签发 API token
#   2. 用 Bearer mlb_... 对 hosted 仓库做 PUT 上传（写入路径）→ 应放行
#   3. 用过期 token PUT → 应 401
#   4. 用已撤销 token PUT → 应 401
#   5. 未带 token 的匿名下载 → 应仍可读（证明未改动只读路径）
# 需要服务运行在 $BASE_URL 且 admin 账号可用（ADMIN_USER/ADMIN_PASS）
# =============================================================================

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

CURL_OPTS="--connect-timeout 10 --max-time 30"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

REPO="token-test-local"
VERSION="1.0.0"
FILE_PATH="com/example/demo/$VERSION/demo-$VERSION.json"

PASS=0
FAIL=0

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS=$((PASS + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL=$((FAIL + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }

echo "============================================"
echo " API Token 客户端鉴权回归测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# ── 1. 登录 admin，拿 JWT ───────────────────────────
JWT=$(curl $CURL_OPTS -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$JWT" ]; then
    fail "无法登录获取 JWT (未配置 admin 凭据或服务未就绪)"
    echo -e "\n${YELLOW}通过: $PASS  失败: $FAIL${NC}"
    echo "通过: $PASS"
    echo "失败: $FAIL"
    exit 1
fi
pass "admin 登录成功"

# ── 2. 确保 hosted 仓库存在 ─────────────────────────
REPO_EXISTS=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" \
    -H "Authorization: Bearer $JWT" \
    "$BASE_URL/api/v1/repositories/$REPO")

if [ "$REPO_EXISTS" != "200" ]; then
    CODE=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/v1/repositories" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $JWT" \
        -d "{\"name\":\"$REPO\",\"display_name\":\"API Token 测试仓库\",\"type\":\"local\",\"package_type\":\"generic\",\"enabled\":true}")
    if [ "$CODE" = "200" ] || [ "$CODE" = "201" ]; then
        pass "创建 hosted 仓库 $REPO"
    else
        fail "创建 hosted 仓库失败 (HTTP $CODE)"
    fi
else
    pass "hosted 仓库 $REPO 已存在"
fi

# ── 3. 签发 API token（长期有效）────────────────────
TOKEN_RAW=$(curl $CURL_OPTS -s -X POST "$BASE_URL/api/v1/auth/tokens" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $JWT" \
    -d '{"name":"ci-regression"}' | \
    grep -o '"token":"[^"]*"' | \
    sed 's/"token":"//;s/"//')

if [ -z "$TOKEN_RAW" ]; then
    fail "无法创建 API token"
    echo -e "\n${YELLOW}通过: $PASS  失败: $FAIL${NC}"
    echo "通过: $PASS"
    echo "失败: $FAIL"
    exit 1
fi

case "$TOKEN_RAW" in
    mlb_*) pass "API token 签发且带 mlb_ 前缀" ;;
    *)     fail "API token 缺少 mlb_ 前缀: ${TOKEN_RAW:0:12}..." ;;
esac

# ── 4. 用 API token 上传（写入路径，CI/CD 场景）──────
printf '{"group":"com.example","name":"demo","version":"%s"}\n' "$VERSION" > /tmp/token-test-body.$$.json
echo "$VERSION" > /tmp/token-test-ver.$$.json  # 第二个文件用于之后统计

CODE=$(curl $CURL_OPTS -s -o /tmp/token-upload-resp.$$ -w "%{http_code}" \
    -X PUT "$BASE_URL/repository/$REPO/$FILE_PATH" \
    -H "Authorization: Bearer $TOKEN_RAW" \
    -H "Content-Type: application/json" \
    --data-binary "@/tmp/token-test-body.$$.json")

if [ "$CODE" = "200" ] || [ "$CODE" = "201" ]; then
    pass "API token → PUT 上传成功 (HTTP $CODE)"
else
    fail "API token → PUT 上传失败 (HTTP $CODE)"
fi

# ── 5. 撤销 token 后，同 token PUT 应被拒 ─────────────
TOKEN_ID=$(curl $CURL_OPTS -s -H "Authorization: Bearer $JWT" \
    "$BASE_URL/api/v1/auth/tokens" | \
    python3 -c "import sys,json; d=json.load(sys.stdin)['data']; print([t['id'] for t in d if t.get('name')=='ci-regression'][0])" 2>/dev/null)

if [ -n "$TOKEN_ID" ]; then
    DEL_CODE=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" -X DELETE \
        -H "Authorization: Bearer $JWT" \
        "$BASE_URL/api/v1/auth/tokens/$TOKEN_ID")
    if [ "$DEL_CODE" = "200" ] || [ "$DEL_CODE" = "204" ]; then
        pass "API token 已撤销 (HTTP $DEL_CODE)"
    else
        fail "撤销 token 失败 (HTTP $DEL_CODE)"
    fi

    CODE=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/$REPO/$FILE_PATH" \
        -H "Authorization: Bearer $TOKEN_RAW" \
        -H "Content-Type: application/json" \
        --data-binary "@/tmp/token-test-body.$$.json")
    if [ "$CODE" = "401" ]; then
        pass "已撤销 token → PUT 被拒 (401)"
    else
        fail "已撤销 token → PUT 应返回 401，实际 $CODE"
    fi
else
    info "未找到 ci-regression token，跳过撤销断言"
fi

# ── 6. 过期 token 应被拒 ─────────────────────────────
# 签发一个 1 秒即过期的 token
EXPIRED_TOKEN=$(curl $CURL_OPTS -s -X POST "$BASE_URL/api/v1/auth/tokens" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $JWT" \
    -d '{"name":"expired-test","expires_in":"1s"}' | \
    grep -o '"token":"[^"]*"' | \
    sed 's/"token":"//;s/"//')

if [ -n "$EXPIRED_TOKEN" ]; then
    sleep 2  # 确保越过 1s 有效期
    CODE=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" \
        -X PUT "$BASE_URL/repository/$REPO/$FILE_PATH" \
        -H "Authorization: Bearer $EXPIRED_TOKEN" \
        -H "Content-Type: application/json" \
        --data-binary "@/tmp/token-test-body.$$.json")
    if [ "$CODE" = "401" ]; then
        pass "过期 token → PUT 被拒 (401)"
    else
        fail "过期 token → PUT 应返回 401，实际 $CODE"
    fi
else
    info "无法签发过期 token，跳过该断言"
fi

# ── 7. 匿名下载仍可读（证明只读路径未受影响）────────
CODE=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/$REPO/$FILE_PATH")
if [ "$CODE" = "200" ]; then
    pass "未带 token 匿名下载 → 仍可读 (200)"
else
    fail "匿名下载应仍是 200，实际 $CODE（注意：若仓库默认私有，此断言可能设计为 401，需按部署配置调整）"
fi

# ── 8. 无凭据写路径应被拒（对照基线）────────────────
CODE=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" \
    -X PUT "$BASE_URL/repository/$REPO/$FILE_PATH.unauth" \
    -H "Content-Type: application/json" \
    --data-binary "@/tmp/token-test-body.$$.json" \
    2>/dev/null || true)
if [ "$CODE" = "401" ]; then
    pass "无凭据 → PUT 被拒 (401)"
else
    info "无凭据 PUT 返回 $CODE（若仓库允许匿名写则属正常）"
fi

# 清理
rm -f "/tmp/token-test-body.$$.json" "/tmp/token-test-ver.$$.json" "/tmp/token-upload-resp.$$"

echo
echo "============================================"
echo " API Token 测试结果"
echo "============================================"
echo -e "${GREEN}通过: $PASS   ${RED}失败: $FAIL${NC}"
echo "通过: $PASS"
echo "失败: $FAIL"

if [ "$FAIL" -eq 0 ]; then
    exit 0
else
    exit 1
fi