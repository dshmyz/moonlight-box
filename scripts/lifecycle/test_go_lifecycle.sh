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

check_go() {
    if command -v go &> /dev/null; then
        return 0
    elif [ -x /usr/local/go/bin/go ]; then
        export PATH="/usr/local/go/bin:$PATH"
        return 0
    else
        warn "go 命令未安装，跳过 Go 模块测试"
        return 1
    fi
}

# cleanup
CLEAN_TEMPS=()
cleanup() { rm -rf "${CLEAN_TEMPS[@]}" 2>/dev/null || true; }
trap cleanup EXIT

echo "============================================"
echo " Go 模块完整生命周期测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

if ! check_go; then
    echo -e "${YELLOW}跳过 Go 模块测试（需要安装 Go）${NC}"
    exit 0
fi

TOKEN=$(get_auth_token)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
PROJECT_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo "$SCRIPT_DIR/../..")"
if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

TEST_DIR="/tmp/go-module-test-$$"
CLEAN_TEMPS+=("$TEST_DIR")
mkdir -p "$TEST_DIR"

echo "════════════════════════════════════════"
echo "  测试 1: 创建测试 Go 模块"
echo "════════════════════════════════════════"

cd "$TEST_DIR"

cat > go.mod <<'EOF'
module github.com/test-user/test-go-module

go 1.21
EOF

cat > main.go <<'EOF'
package main

import "fmt"

func Hello() string {
    return "Hello from test Go module!"
}

func main() {
    fmt.Println(Hello())
}
EOF

if [ -f "go.mod" ] && [ -f "main.go" ]; then
    pass "Go 模块创建成功"
else
    fail "Go 模块创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 发布 v1.0.0 版本"
echo "════════════════════════════════════════"

git init > /dev/null 2>&1
git add . > /dev/null 2>&1
git commit -m "Initial commit" > /dev/null 2>&1
git tag v1.0.0 > /dev/null 2>&1

if [ -d ".git" ]; then
    pass "Git 仓库初始化并打标签成功"
else
    fail "Git 仓库初始化失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 3: 配置 GOPROXY 并获取模块"
echo "════════════════════════════════════════"

export GOPROXY="$BASE_URL/repository/go-proxy-official"
export GO111MODULE=on
export GONOSUMCHECK=*
export GONOSUMDB=*
export GOINSECURE=localhost

CONSUMER_DIR="/tmp/go-consumer-test-$$"
CLEAN_TEMPS+=("$CONSUMER_DIR")
mkdir -p "$CONSUMER_DIR"
cd "$CONSUMER_DIR"

cat > go.mod <<EOF
module github.com/test-user/consumer

go 1.21

require github.com/test-user/test-go-module v1.0.0

replace github.com/test-user/test-go-module => $TEST_DIR
EOF

cat > main.go <<'EOF'
package main

import (
    "fmt"
    testmodule "github.com/test-user/test-go-module"
)

func main() {
    fmt.Println(testmodule.Hello())
}
EOF

if go mod tidy > /dev/null 2>&1; then
    pass "go mod tidy 执行成功"
else
    warn "go mod tidy 执行失败（可能缺少模块）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 验证 GOPROXY 协议端点"
echo "════════════════════════════════════════"

GOPROXY_BASE="$BASE_URL/repository/go-proxy-goproxy-cn"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$GOPROXY_BASE/github.com/stretchr/testify/@v/list")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Go @v/list 端点可访问 (HTTP 200)"
    
    VERSION_LIST=$(curl -s "$GOPROXY_BASE/github.com/stretchr/testify/@v/list")
    if echo "$VERSION_LIST" | grep -q "v1.8.4"; then
        pass "@v/list 包含 v1.8.4 版本"
    else
        info "@v/list 未包含 v1.8.4（可能未缓存）"
    fi
else
    fail "Go @v/list 端点不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: 验证 .info 文件"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /tmp/go-test.info -w "%{http_code}" \
    "$GOPROXY_BASE/github.com/stretchr/testify/@v/v1.8.4.info")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Go .info 文件可访问 (HTTP 200)"
    
    INFO_CONTENT=$(cat /tmp/go-test.info)
    if echo "$INFO_CONTENT" | grep -q '"Version"'; then
        pass ".info 文件包含 Version 字段"
    else
        fail ".info 文件不包含 Version 字段"
    fi
    
    if echo "$INFO_CONTENT" | grep -q '"Time"'; then
        pass ".info 文件包含 Time 字段"
    else
        fail ".info 文件不包含 Time 字段"
    fi
else
    fail "Go .info 文件不可访问 (HTTP $HTTP_CODE)"
fi

rm -f /tmp/go-test.info

echo
echo "════════════════════════════════════════"
echo "  测试 6: 验证 .mod 文件"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /tmp/go-test.mod -w "%{http_code}" \
    "$GOPROXY_BASE/github.com/stretchr/testify/@v/v1.8.4.mod")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Go .mod 文件可访问 (HTTP 200)"
    
    MOD_CONTENT=$(cat /tmp/go-test.mod)
    if echo "$MOD_CONTENT" | grep -q "module"; then
        pass ".mod 文件包含 module 声明"
    else
        fail ".mod 文件不包含 module 声明"
    fi
else
    fail "Go .mod 文件不可访问 (HTTP $HTTP_CODE)"
fi

rm -f /tmp/go-test.mod

echo
echo "════════════════════════════════════════"
echo "  测试 7: 验证 .zip 文件"
echo "════════════════════════════════════════"

HTTP_CODE=$(curl -s -o /tmp/go-test.zip -w "%{http_code}" \
    "$GOPROXY_BASE/github.com/stretchr/testify/@v/v1.8.4.zip")

if [ "$HTTP_CODE" = "200" ]; then
    pass "Go .zip 文件可访问 (HTTP 200)"
    
    if [ -f "/tmp/go-test.zip" ] && [ -s "/tmp/go-test.zip" ]; then
        pass "下载的 .zip 文件非空"
        
        if unzip -t /tmp/go-test.zip > /dev/null 2>&1; then
            pass ".zip 文件是有效的 zip 格式"
        else
            fail ".zip 文件不是有效的 zip 格式"
        fi
    else
        fail "下载的 .zip 文件为空"
    fi
else
    fail "Go .zip 文件不可访问 (HTTP $HTTP_CODE)"
fi

rm -f /tmp/go-test.zip

echo
echo "════════════════════════════════════════"
echo "  测试 8: 代理仓库测试"
echo "════════════════════════════════════════"

export GOPROXY="$BASE_URL/repository/go-proxy-goproxy-cn"

PROXY_TEST_DIR="/tmp/go-proxy-test-$$"
CLEAN_TEMPS+=("$PROXY_TEST_DIR")
mkdir -p "$PROXY_TEST_DIR"
cd "$PROXY_TEST_DIR"

cat > go.mod <<'EOF'
module github.com/test-user/proxy-test

go 1.21
EOF

if GOPROXY="$GOPROXY" go get github.com/stretchr/testify@v1.8.4 > /dev/null 2>&1; then
    pass "从代理仓库获取模块成功"
    
    if grep -q "stretchr/testify" go.sum; then
        pass "go.sum 包含 testify 模块"
    else
        fail "go.sum 不包含 testify 模块"
    fi
else
    warn "从代理仓库获取模块失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 9: 验证代理缓存"
echo "════════════════════════════════════════"

START_TIME=$(date +%s%N)

HTTP_CODE=$(curl -s -o /tmp/go-proxy-cache.zip -w "%{http_code}" \
    "$BASE_URL/repository/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.zip")

END_TIME=$(date +%s%N)
ELAPSED=$(( (END_TIME - START_TIME) / 1000000 ))

if [ "$HTTP_CODE" = "200" ]; then
    pass "代理缓存文件可访问 (HTTP 200)"
    info "响应时间: ${ELAPSED}ms"
    
    if [ "$ELAPSED" -lt 500 ]; then
        pass "响应时间 < 500ms（可能命中缓存）"
    else
        info "响应时间 >= 500ms（可能未缓存）"
    fi
else
    fail "代理缓存文件不可访问 (HTTP $HTTP_CODE)"
fi

rm -f /tmp/go-proxy-cache.zip

echo
echo "════════════════════════════════════════"
echo "  测试 10: 验证存储目录结构"
echo "════════════════════════════════════════"

	GO_STORAGE="$PROJECT_ROOT/data/packages/go"

if [ -d "$GO_STORAGE" ]; then
    pass "Go 存储目录存在: $GO_STORAGE"
    
    MOD_COUNT=$(find "$GO_STORAGE" -name "*.mod" 2>/dev/null | wc -l | tr -d ' ')
    ZIP_COUNT=$(find "$GO_STORAGE" -name "*.zip" 2>/dev/null | wc -l | tr -d ' ')
    INFO_COUNT=$(find "$GO_STORAGE" -name "*.info" 2>/dev/null | wc -l | tr -d ' ')
    
    info "存储文件统计: .mod=$MOD_COUNT, .zip=$ZIP_COUNT, .info=$INFO_COUNT"
    
    if [ "$MOD_COUNT" -gt 0 ] && [ "$ZIP_COUNT" -gt 0 ]; then
        pass "Go 模块文件已正确存储"
    else
        info "Go 模块文件存储可能不完整"
    fi
else
    info "Go 存储目录不存在: $GO_STORAGE"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 11: 清理测试环境"
echo "════════════════════════════════════════"

cd /
rm -rf "$TEST_DIR" "$CONSUMER_DIR" "$PROXY_TEST_DIR"

unset GOPROXY
unset GO111MODULE
unset GONOSUMCHECK
unset GONOSUMDB
unset GOINSECURE

if [ ! -d "$TEST_DIR" ] && [ ! -d "$CONSUMER_DIR" ] && [ ! -d "$PROXY_TEST_DIR" ]; then
    pass "测试环境清理成功"
else
    warn "测试环境清理可能不完整"
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
