#!/bin/bash

set -e

export PATH="/usr/bin:/usr/local/bin:$PATH"

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

echo "============================================"
echo " 异常场景测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

TEST_DIR="/tmp/exception-test-$$"
mkdir -p "$TEST_DIR"

echo "════════════════════════════════════════"
echo "  测试 1: 空文件上传"
echo "════════════════════════════════════════"

EMPTY_FILE="$TEST_DIR/empty-file.txt"
touch "$EMPTY_FILE"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repository/maven-local/com/test/empty-test/1.0.0/empty-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @"$EMPTY_FILE")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "400" ]; then
    pass "空文件上传处理正常 (HTTP $HTTP_CODE)"
else
    fail "空文件上传返回异常状态 (HTTP $HTTP_CODE)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 超大文件上传（超过限制）"
echo "════════════════════════════════════════"

LARGE_FILE="$TEST_DIR/large-file-500mb.bin"
info "生成 500MB 测试文件..."
dd if=/dev/zero of="$LARGE_FILE" bs=1M count=500 2>/dev/null

if [ -f "$LARGE_FILE" ]; then
    info "上传超大文件（可能触发限制）..."
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
        "$BASE_URL/repository/maven-local/com/test/large-test/1.0.0/large-test-1.0.0.bin" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary @"$LARGE_FILE" 2>&1 || echo "000")
    
    if [ "$HTTP_CODE" = "413" ]; then
        pass "超大文件被正确拒绝 (HTTP 413 Payload Too Large)"
    elif [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
        warn "超大文件上传成功（可能未设置大小限制）"
    elif [ "$HTTP_CODE" = "000" ]; then
        warn "超大文件上传连接被拒绝或超时"
    else
        info "超大文件上传返回 HTTP $HTTP_CODE"
    fi
fi

rm -f "$LARGE_FILE"

echo
echo "════════════════════════════════════════"
echo "  测试 3: 并发上传同一 SNAPSHOT 版本"
echo "════════════════════════════════════════"

CONCURRENT_COUNT=10
info "启动 $CONCURRENT_COUNT 个并发上传到同一 SNAPSHOT 版本..."

for i in $(seq 1 $CONCURRENT_COUNT); do
    (
        CONCURRENT_FILE="$TEST_DIR/snapshot-concurrent-$i.jar"
        echo "Snapshot content from client $i - $(date)" > "$CONCURRENT_FILE"
        
        HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
            "$BASE_URL/repository/maven-snapshots/com/test/concurrent-snapshot/1.0-SNAPSHOT/concurrent-snapshot-1.0-$(date +%Y%m%d.%H%M%S)-$i.jar" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/octet-stream" \
            --data-binary @"$CONCURRENT_FILE")
        
        echo "$HTTP_CODE" > "$TEST_DIR/snapshot-result-$i.txt"
    ) &
done

wait

SUCCESS_COUNT=0
FAIL_COUNT_SNAPSHOT=0

for i in $(seq 1 $CONCURRENT_COUNT); do
    if [ -f "$TEST_DIR/snapshot-result-$i.txt" ]; then
        CODE=$(cat "$TEST_DIR/snapshot-result-$i.txt")
        if [ "$CODE" = "200" ] || [ "$CODE" = "201" ]; then
            SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
        else
            FAIL_COUNT_SNAPSHOT=$((FAIL_COUNT_SNAPSHOT + 1))
        fi
    fi
done

if [ "$SUCCESS_COUNT" -gt 0 ]; then
    pass "并发 SNAPSHOT 上传 - $SUCCESS_COUNT 个成功"
else
    fail "并发 SNAPSHOT 上传 - 全部失败"
fi

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/maven-snapshots/com/test/concurrent-snapshot/1.0-SNAPSHOT/maven-metadata.xml")

if [ "$HTTP_CODE" = "200" ]; then
    pass "并发上传后 maven-metadata.xml 仍可访问"
    
    METADATA=$(curl -s "$BASE_URL/repository/maven-snapshots/com/test/concurrent-snapshot/1.0-SNAPSHOT/maven-metadata.xml")
    if echo "$METADATA" | grep -q "<buildNumber>"; then
        pass "maven-metadata.xml 包含 buildNumber（元数据更新正常）"
    else
        warn "maven-metadata.xml 不包含 buildNumber"
    fi
else
    warn "并发上传后 maven-metadata.xml 不可访问"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 无效路径访问"
echo "════════════════════════════════════════"

INVALID_PATHS=(
    "$BASE_URL/repository/maven-local/../../../etc/passwd"
    "$BASE_URL/repository/maven-local/%2e%2e/%2e%2e/etc/passwd"
    "$BASE_URL/repository/maven-local/..%2F..%2Fetc%2Fpasswd"
)

for TEST_PATH in "${INVALID_PATHS[@]}"; do
    HTTP_CODE=$(curl --path-as-is -s -o /dev/null -w "%{http_code}" "$TEST_PATH")
    
    if [ "$HTTP_CODE" = "400" ] || [ "$HTTP_CODE" = "403" ] || [ "$HTTP_CODE" = "404" ]; then
        pass "路径遍历攻击被阻止 (HTTP $HTTP_CODE)"
    else
        fail "路径遍历攻击未被阻止 (HTTP $HTTP_CODE)"
    fi
done

echo
echo "════════════════════════════════════════"
echo "  测试 5: 特殊字符文件名"
echo "════════════════════════════════════════"

SPECIAL_FILES=(
    "file with spaces-1.0.0.txt"
    "file%20encoded-1.0.0.txt"
    "file+plus-1.0.0.txt"
)

for FILENAME in "${SPECIAL_FILES[@]}"; do
    TEST_FILE="$TEST_DIR/$FILENAME"
    echo "Special character test content" > "$TEST_FILE"

    URL_FILENAME="${FILENAME// /%20}"
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
        "$BASE_URL/repository/maven-local/com/test/special-test/1.0.0/$URL_FILENAME" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: text/plain" \
        --data-binary @"$TEST_FILE" || echo "000")

    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "400" ]; then
        pass "特殊字符文件名处理正常 (HTTP $HTTP_CODE)"
    else
        warn "特殊字符文件名返回 HTTP $HTTP_CODE"
    fi
    
    rm -f "$TEST_FILE"
done

echo
echo "════════════════════════════════════════"
echo "  测试 6: 重复上传同一制品"
echo "════════════════════════════════════════"

TEST_FILE="$TEST_DIR/duplicate-test-1.0.0.txt"
echo "Duplicate test content - $(date)" > "$TEST_FILE"

HTTP_CODE_1=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repository/maven-local/com/test/duplicate-test/1.0.0/duplicate-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @"$TEST_FILE")

HTTP_CODE_2=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repository/maven-local/com/test/duplicate-test/1.0.0/duplicate-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @"$TEST_FILE")

info "第一次上传: HTTP $HTTP_CODE_1, 第二次上传: HTTP $HTTP_CODE_2"

if [ "$HTTP_CODE_1" = "200" ] || [ "$HTTP_CODE_1" = "201" ]; then
    if [ "$HTTP_CODE_2" = "200" ] || [ "$HTTP_CODE_2" = "409" ]; then
        pass "重复上传处理正常"
    else
        warn "重复上传返回异常状态 (HTTP $HTTP_CODE_2)"
    fi
else
    fail "第一次上传失败 (HTTP $HTTP_CODE_1)"
fi

rm -f "$TEST_FILE"

echo
echo "════════════════════════════════════════"
echo "  测试 7: 删除后再次访问"
echo "════════════════════════════════════════"

TEST_FILE="$TEST_DIR/delete-test-1.0.0.txt"
echo "Delete test content" > "$TEST_FILE"

curl -s -X PUT \
    "$BASE_URL/repository/maven-local/com/test/delete-test/1.0.0/delete-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @"$TEST_FILE" > /dev/null 2>&1

HTTP_CODE_DELETE=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
    "$BASE_URL/repository/maven-local/com/test/delete-test/1.0.0/delete-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN")

if [ "$HTTP_CODE_DELETE" = "200" ] || [ "$HTTP_CODE_DELETE" = "204" ]; then
    pass "制品删除成功 (HTTP $HTTP_CODE_DELETE)"
    
    HTTP_CODE_AFTER=$(curl -s -o /dev/null -w "%{http_code}" \
        "$BASE_URL/repository/maven-local/com/test/delete-test/1.0.0/delete-test-1.0.0.txt")
    
    if [ "$HTTP_CODE_AFTER" = "404" ]; then
        pass "删除后访问返回 404 (符合预期)"
    else
        fail "删除后仍可访问 (HTTP $HTTP_CODE_AFTER)"
    fi
    
    HTTP_CODE_DELETE_AGAIN=$(curl -s -o /dev/null -w "%{http_code}" -X DELETE \
        "$BASE_URL/repository/maven-local/com/test/delete-test/1.0.0/delete-test-1.0.0.txt" \
        -H "Authorization: Bearer $TOKEN")
    
    if [ "$HTTP_CODE_DELETE_AGAIN" = "404" ]; then
        pass "重复删除返回 404 (符合预期)"
    else
        fail "重复删除返回 HTTP $HTTP_CODE_DELETE_AGAIN"
    fi
else
    fail "制品删除失败 (HTTP $HTTP_CODE_DELETE)"
fi

rm -f "$TEST_FILE"

echo
echo "════════════════════════════════════════"
echo "  测试 8: 代理仓库上游不可用"
echo "════════════════════════════════════════"

info "请求代理仓库中不存在的制品..."

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/maven-proxy-aliyun/com/nonexistent/nonexistent-lib/999.999.999/nonexistent-lib-999.999.999.jar")

if [ "$HTTP_CODE" = "404" ] || [ "$HTTP_CODE" = "502" ]; then
    pass "上游不可用制品返回合理错误 (HTTP $HTTP_CODE)"
else
    fail "上游不可用制品返回 HTTP $HTTP_CODE"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 9: 请求头注入测试"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT \
    "$BASE_URL/repository/maven-local/com/test/header-test/1.0.0/header-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: text/plain" \
    -H "X-Custom-Header: test-value" \
    --data-binary "Header injection test")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "自定义请求头处理正常"
else
    fail "自定义请求头返回 HTTP $HTTP_CODE"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 10: 慢速客户端测试"
echo "════════════════════════════════════════"

TEST_FILE="$TEST_DIR/slow-client-test.txt"
echo "Slow client test content" > "$TEST_FILE"

info "模拟慢速上传..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" --limit-rate 1K -X PUT \
    "$BASE_URL/repository/maven-local/com/test/slow-test/1.0.0/slow-test-1.0.0.txt" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: text/plain" \
    --data-binary @"$TEST_FILE")

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
    pass "慢速客户端上传成功"
else
    warn "慢速客户端上传返回 HTTP $HTTP_CODE"
fi

rm -f "$TEST_FILE"

echo
echo "════════════════════════════════════════"
echo "  测试 11: 清理测试文件"
echo "════════════════════════════════════════"

cd /
rm -rf "$TEST_DIR"

if [ ! -d "$TEST_DIR" ]; then
    pass "测试文件清理成功"
else
    warn "测试文件清理可能不完整"
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
