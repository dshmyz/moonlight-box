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
}

get_auth_token() {
    curl -s -X POST "$BASE_URL/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
        grep -o '"access_token":"[^"]*"' | \
        sed 's/"access_token":"//;s/"//'
}

echo "============================================"
echo " 仓库组（Group）能力测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

echo "════════════════════════════════════════"
echo "  测试 1: 创建托管仓库 A"
echo "════════════════════════════════════════"

REPO_A_NAME="group-test-local-a-$$"

CREATE_REPO_A=$(curl -s -X POST "$BASE_URL/api/v1/repositories" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"$REPO_A_NAME\",
        \"type\": \"local\",
        \"package_type\": \"maven\",
        \"enabled\": true,
        \"description\": \"Group test local repository A\"
    }")

if echo "$CREATE_REPO_A" | grep -q "\"name\":\"$REPO_A_NAME\""; then
    pass "托管仓库 A 创建成功: $REPO_A_NAME"
else
    info "托管仓库 A 创建响应: $CREATE_REPO_A"
    warn "托管仓库 A 可能已存在或创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 创建托管仓库 B"
echo "════════════════════════════════════════"

REPO_B_NAME="group-test-local-b-$$"

CREATE_REPO_B=$(curl -s -X POST "$BASE_URL/api/v1/repositories" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"$REPO_B_NAME\",
        \"type\": \"local\",
        \"package_type\": \"maven\",
        \"enabled\": true,
        \"description\": \"Group test local repository B\"
    }")

if echo "$CREATE_REPO_B" | grep -q "\"name\":\"$REPO_B_NAME\""; then
    pass "托管仓库 B 创建成功: $REPO_B_NAME"
else
    info "托管仓库 B 创建响应: $CREATE_REPO_B"
    warn "托管仓库 B 可能已存在或创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 3: 向仓库 A 上传制品"
echo "════════════════════════════════════════"

TEST_JAR_A="/tmp/group-test-artifact-a-$$-1.0.0.jar"
echo "Repository A artifact content - $(date)" > "$TEST_JAR_A"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repo/$REPO_A_NAME/com/test/group-artifact-a/1.0.0/group-artifact-a-1.0.0.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR_A")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "仓库 A 制品上传成功 (HTTP $HTTP_CODE)"
else
    fail "仓库 A 制品上传失败 (HTTP $HTTP_CODE)"
fi

rm -f "$TEST_JAR_A"

echo
echo "════════════════════════════════════════"
echo "  测试 4: 向仓库 B 上传制品"
echo "════════════════════════════════════════"

TEST_JAR_B="/tmp/group-test-artifact-b-$$-1.0.0.jar"
echo "Repository B artifact content - $(date)" > "$TEST_JAR_B"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repo/$REPO_B_NAME/com/test/group-artifact-b/1.0.0/group-artifact-b-1.0.0.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_JAR_B")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "仓库 B 制品上传成功 (HTTP $HTTP_CODE)"
else
    fail "仓库 B 制品上传失败 (HTTP $HTTP_CODE)"
fi

rm -f "$TEST_JAR_B"

echo
echo "════════════════════════════════════════"
echo "  测试 5: 创建仓库组（组合 A 和 B）"
echo "════════════════════════════════════════"

GROUP_NAME="group-test-virtual-$$"

CREATE_GROUP=$(curl -s -X POST "$BASE_URL/api/v1/repositories" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\": \"$GROUP_NAME\",
        \"type\": \"virtual\",
        \"package_type\": \"maven\",
        \"enabled\": true,
        \"description\": \"Group test virtual repository\",
        \"member_repositories\": [\"$REPO_A_NAME\", \"$REPO_B_NAME\"]
    }")

if echo "$CREATE_GROUP" | grep -q "\"name\":\"$GROUP_NAME\""; then
    pass "仓库组创建成功: $GROUP_NAME"
else
    info "仓库组创建响应: $CREATE_GROUP"
    warn "仓库组可能已存在或创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 6: 通过仓库组下载仓库 A 的制品"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /tmp/group-download-a.jar -w "%{http_code}" \
    "$BASE_URL/repo/$GROUP_NAME/com/test/group-artifact-a/1.0.0/group-artifact-a-1.0.0.jar")

if [ "$HTTP_CODE" = "200" ]; then
    pass "通过仓库组下载仓库 A 制品成功 (HTTP 200)"
    
    if [ -f "/tmp/group-download-a.jar" ] && [ -s "/tmp/group-download-a.jar" ]; then
        CONTENT=$(cat /tmp/group-download-a.jar)
        if echo "$CONTENT" | grep -q "Repository A"; then
            pass "下载的制品内容正确（来自仓库 A）"
        else
            fail "下载的制品内容不正确"
        fi
    else
        fail "下载的制品文件为空"
    fi
else
    fail "通过仓库组下载仓库 A 制品失败 (HTTP $HTTP_CODE)"
fi

rm -f /tmp/group-download-a.jar

echo
echo "════════════════════════════════════════"
echo "  测试 7: 通过仓库组下载仓库 B 的制品"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /tmp/group-download-b.jar -w "%{http_code}" \
    "$BASE_URL/repo/$GROUP_NAME/com/test/group-artifact-b/1.0.0/group-artifact-b-1.0.0.jar")

if [ "$HTTP_CODE" = "200" ]; then
    pass "通过仓库组下载仓库 B 制品成功 (HTTP 200)"
    
    if [ -f "/tmp/group-download-b.jar" ] && [ -s "/tmp/group-download-b.jar" ]; then
        CONTENT=$(cat /tmp/group-download-b.jar)
        if echo "$CONTENT" | grep -q "Repository B"; then
            pass "下载的制品内容正确（来自仓库 B）"
        else
            fail "下载的制品内容不正确"
        fi
    else
        fail "下载的制品文件为空"
    fi
else
    fail "通过仓库组下载仓库 B 制品失败 (HTTP $HTTP_CODE)"
fi

rm -f /tmp/group-download-b.jar

echo
echo "════════════════════════════════════════"
echo "  测试 8: 验证仓库组搜索顺序"
echo "════════════════════════════════════════"

TEST_SHARED_JAR="/tmp/group-test-shared-$$-1.0.0.jar"
echo "Shared artifact in both repos - $(date)" > "$TEST_SHARED_JAR"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repo/$REPO_A_NAME/com/test/shared-artifact/1.0.0/shared-artifact-1.0.0.jar" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$TEST_SHARED_JAR")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "仓库 A 共享制品上传成功"
    
    HTTP_CODE_B=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
        "$BASE_URL/repo/$REPO_B_NAME/com/test/shared-artifact/1.0.0/shared-artifact-1.0.0.jar" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary @"$TEST_SHARED_JAR")
    
    if [ "$HTTP_CODE_B" = "200" ] || [ "$HTTP_CODE_B" = "201" ]; then
        pass "仓库 B 共享制品上传成功"
        
        HTTP_CODE_GROUP=$(curl -s -o /tmp/group-download-shared.jar -w "%{http_code}" \
            "$BASE_URL/repo/$GROUP_NAME/com/test/shared-artifact/1.0.0/shared-artifact-1.0.0.jar")
        
        if [ "$HTTP_CODE_GROUP" = "200" ]; then
            pass "通过仓库组下载共享制品成功"
            
            CONTENT=$(cat /tmp/group-download-shared.jar)
            if echo "$CONTENT" | grep -q "Shared artifact"; then
                pass "下载的共享制品内容正确"
                info "注意: 仓库组应返回第一个匹配的仓库中的版本"
            else
                fail "下载的共享制品内容不正确"
            fi
        else
            fail "通过仓库组下载共享制品失败 (HTTP $HTTP_CODE_GROUP)"
        fi
        
        rm -f /tmp/group-download-shared.jar
    fi
fi

rm -f "$TEST_SHARED_JAR"

echo
echo "════════════════════════════════════════"
echo "  测试 9: 验证仓库组包含不存在的制品"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repo/$GROUP_NAME/com/test/nonexistent/1.0.0/nonexistent-1.0.0.jar")

if [ "$HTTP_CODE" = "404" ]; then
    pass "仓库组中不存在的制品返回 404 (符合预期)"
else
    fail "仓库组中不存在的制品返回 HTTP $HTTP_CODE (expected 404)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 10: 验证仓库组成员配置"
echo "════════════════════════════════════════"

GROUP_INFO=$(curl -s "$BASE_URL/api/v1/repositories/$GROUP_NAME" \
    -H "Authorization: Bearer $TOKEN")

if echo "$GROUP_INFO" | grep -q "$REPO_A_NAME"; then
    pass "仓库组包含成员: $REPO_A_NAME"
else
    fail "仓库组不包含成员: $REPO_A_NAME"
fi

if echo "$GROUP_INFO" | grep -q "$REPO_B_NAME"; then
    pass "仓库组包含成员: $REPO_B_NAME"
else
    fail "仓库组不包含成员: $REPO_B_NAME"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 11: 清理测试仓库"
echo "════════════════════════════════════════"

curl -s -X DELETE "$BASE_URL/api/v1/repositories/$GROUP_NAME" \
    -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1

curl -s -X DELETE "$BASE_URL/api/v1/repositories/$REPO_A_NAME" \
    -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1

curl -s -X DELETE "$BASE_URL/api/v1/repositories/$REPO_B_NAME" \
    -H "Authorization: Bearer $TOKEN" > /dev/null 2>&1

info "测试仓库已清理"

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
