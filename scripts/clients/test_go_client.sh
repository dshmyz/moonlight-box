#!/bin/bash

# =============================================================================
# Go 客户端真实集成测试
# 使用官方 go get 命令测试 GOPROXY 功能
# =============================================================================

BASE_URL="${1:-http://localhost:9081}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; }

echo "============================================"
echo " Go 客户端真实集成测试"
echo " 使用官方 go get 命令测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 检查 go 命令
if ! command -v go &> /dev/null; then
    warn "go 命令未安装，跳过测试"
    exit 0
fi

info "Go 版本: $(go version | awk '{print $3}')"

# 创建测试项目
TEST_DIR="/tmp/go-client-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

info "创建测试项目..."

cat > go.mod <<'EOF'
module test-go-client

go 1.21
EOF

echo "测试 1: 配置 GOPROXY..."
export GOPROXY="$BASE_URL/repository/go-proxy-goproxy-cn,direct"
export GOPROXY_ON=off

if go list -m github.com/stretchr/testify@v1.8.4 &> /dev/null; then
    pass "go list -m 测试通过 (成功获取模块信息)"
else
    fail "go list -m 测试失败 (无法获取模块信息)"
fi

echo
echo "测试 2: 使用 go get 获取依赖..."
if GOPROXY="$BASE_URL/repository/go-proxy-goproxy-cn,direct" \
   go get github.com/stretchr/testify@v1.8.4 &> /dev/null 2>&1; then
    pass "go get 测试通过 (成功下载依赖)"
else
    warn "go get 测试失败 (可能是代理配置问题)"
fi

echo
echo "测试 3: 验证 go.sum 文件..."
if [ -f "go.sum" ] && grep -q "github.com/stretchr/testify" go.sum; then
    pass "go.sum 文件生成正确"
else
    warn "go.sum 文件未正确生成"
fi

echo
echo "测试 4: 验证模块缓存..."
CACHE_DIR=$(go env GOMODCACHE)
info "模块缓存目录: $CACHE_DIR"

if [ -d "$CACHE_DIR/github.com/!stretchr/testify@v1.8.4" ]; then
    pass "模块已缓存到本地"
    ls -la "$CACHE_DIR/github.com/!stretchr/testify@v1.8.4" | head -5
else
    warn "模块未缓存"
fi

echo
echo "测试 5: 使用 go mod download 测试..."
GOPROXY="$BASE_URL/repository/go-proxy-goproxy-cn,direct" \
    go mod download github.com/stretchr/testify@v1.8.4 &> /dev/null 2>&1

if [ $? -eq 0 ]; then
    pass "go mod download 测试通过"
else
    warn "go mod download 测试失败"
fi

echo
echo "测试 6: 验证 @v/list 端点..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
    "$BASE_URL/repository/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/list")

if [ "$HTTP_CODE" = "200" ]; then
    pass "@v/list 端点可访问 (HTTP 200)"
    curl -s "$BASE_URL/repository/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/list" | head -5
else
    fail "@v/list 端点不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 7: 验证 @v/info 端点..."
HTTP_CODE=$(curl -s -o /tmp/go-test-info.json -w "%{http_code}" \
    "$BASE_URL/repository/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.info")

if [ "$HTTP_CODE" = "200" ]; then
    pass "@v/info 端点可访问 (HTTP 200)"
    
    if grep -q '"Version"' /tmp/go-test-info.json; then
        pass "info 文件包含 Version 字段"
    else
        warn "info 文件缺少 Version 字段"
    fi
    
    if grep -q '"Time"' /tmp/go-test-info.json; then
        pass "info 文件包含 Time 字段"
    else
        warn "info 文件缺少 Time 字段"
    fi
else
    fail "@v/info 端点不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 8: 验证 @v/mod 端点..."
HTTP_CODE=$(curl -s -o /tmp/go-test-mod -w "%{http_code}" \
    "$BASE_URL/repository/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.mod")

if [ "$HTTP_CODE" = "200" ]; then
    pass "@v/mod 端点可访问 (HTTP 200)"
    
    if grep -q "module github.com/stretchr/testify" /tmp/go-test-mod; then
        pass "mod 文件包含 module 声明"
    else
        warn "mod 文件缺少 module 声明"
    fi
else
    fail "@v/mod 端点不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 9: 验证 @v.zip 端点..."
HTTP_CODE=$(curl -s -o /tmp/go-test.zip -w "%{http_code}" \
    "$BASE_URL/repository/go-proxy-goproxy-cn/github.com/stretchr/testify/@v/v1.8.4.zip")

if [ "$HTTP_CODE" = "200" ]; then
    pass "@v/zip 端点可访问 (HTTP 200)"
    
    if [ -s "/tmp/go-test.zip" ]; then
        pass "下载的 zip 文件非空"
        
        if unzip -t /tmp/go-test.zip > /dev/null 2>&1; then
            pass "zip 文件格式正确"
        else
            fail "zip 文件格式无效"
        fi
    else
        warn "下载的 zip 文件为空"
    fi
else
    fail "@v/zip 端点不可访问 (HTTP $HTTP_CODE)"
fi

# 清理
cd /
rm -rf "$TEST_DIR"

echo
echo "============================================"
echo " Go 客户端测试完成"
echo "============================================"
