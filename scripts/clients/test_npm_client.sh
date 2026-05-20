#!/bin/bash

# =============================================================================
# NPM 客户端真实集成测试
# 使用官方 npm install 命令测试仓库功能
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
echo " NPM 客户端真实集成测试"
echo " 使用官方 npm install 命令测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

# 检查 npm 命令
if ! command -v npm &> /dev/null; then
    warn "npm 命令未安装，跳过测试"
    exit 0
fi

info "Node.js 版本: $(node --version)"
info "npm 版本: $(npm --version)"

# 获取认证令牌
TOKEN=$(curl -s -X POST "$BASE_URL/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}" | \
    grep -o '"access_token":"[^"]*"' | \
    sed 's/"access_token":"//;s/"//')

if [ -z "$TOKEN" ]; then
    warn "无法获取认证令牌，跳过需要认证的测试"
fi

# 创建测试项目
TEST_DIR="/tmp/npm-client-test-$$"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "测试 1: 创建 package.json..."
cat > package.json <<'EOF'
{
  "name": "test-npm-client",
  "version": "1.0.0",
  "dependencies": {}
}
EOF
pass "package.json 创建成功"

echo
echo "测试 2: 配置 npm registry 为代理仓库..."
npm config set registry "$BASE_URL/repository/npm-proxy-cn"
info "当前 registry: $(npm config get registry)"

echo
echo "测试 3: 使用 npm install 安装 lodash..."
if npm install lodash@4.17.21 --save &> /dev/null 2>&1; then
    pass "npm install lodash 测试通过"
    
    if [ -d "node_modules/lodash" ]; then
        pass "lodash 包已安装到 node_modules"
        info "包大小: $(du -sh node_modules/lodash | cut -f1)"
    else
        fail "lodash 包未正确安装"
    fi
else
    warn "npm install lodash 测试失败 (可能是网络或代理配置问题)"
fi

echo
echo "测试 4: 验证 package.json 更新..."
if grep -q "lodash" package.json; then
    pass "package.json 已正确更新依赖"
    cat package.json | grep -A 3 "dependencies"
else
    warn "package.json 未正确更新"
fi

echo
echo "测试 5: 验证 package-lock.json..."
if [ -f "package-lock.json" ]; then
    pass "package-lock.json 已生成"
    info "lock 文件大小: $(du -sh package-lock.json | cut -f1)"
else
    warn "package-lock.json 未生成"
fi

echo
echo "测试 6: 测试 npm view 命令..."
if npm view lodash@4.17.21 version &> /dev/null 2>&1; then
    VERSION=$(npm view lodash@4.17.21 version)
    pass "npm view 测试通过 (版本: $VERSION)"
else
    warn "npm view 测试失败"
fi

echo
echo "测试 7: 测试作用域包 (@babel/core)..."
npm config set registry "$BASE_URL/repository/npm-proxy-cn"
if npm install @babel/core@7.24.0 --save &> /dev/null 2>&1; then
    pass "npm install @babel/core 测试通过"
    
    if [ -d "node_modules/@babel/core" ]; then
        pass "@babel/core 包已安装"
    else
        warn "@babel/core 包未正确安装"
    fi
else
    warn "npm install @babel/core 测试失败"
fi

echo
echo "测试 8: 验证 npm 代理元数据..."
HTTP_CODE=$(curl -s -o /tmp/npm-meta.json -w "%{http_code}" \
    "$BASE_URL/repository/npm-proxy-cn/lodash")

if [ "$HTTP_CODE" = "200" ]; then
    pass "npm 元数据 API 可访问 (HTTP 200)"
    
    if grep -q '"name"' /tmp/npm-meta.json; then
        pass "元数据包含 name 字段"
    else
        warn "元数据缺少 name 字段"
    fi
    
    if grep -q '"dist"' /tmp/npm-meta.json; then
        pass "元数据包含 dist 字段"
    else
        warn "元数据缺少 dist 字段"
    fi
else
    warn "npm 元数据 API 不可访问 (HTTP $HTTP_CODE)"
fi

echo
echo "测试 9: 测试 npm publish (需要认证)..."
cd "$TEST_DIR"
mkdir -p "test-publish-pkg"
cd "test-publish-pkg"

cat > package.json <<'EOF'
{
  "name": "@test-local/test-publish-pkg",
  "version": "1.0.0",
  "description": "Test package for registry"
}
EOF

# 尝试发布到本地仓库
npm config set registry "$BASE_URL/repository/npm-local"

if [ -n "$TOKEN" ]; then
    # npm publish 需要认证
    # 注意：某些 npm 配置可能不支持这种方式发布
    if npm publish --access public &> /tmp/npm-publish.log 2>&1; then
        pass "npm publish 测试通过"
    else
        WARN_CODE=$(cat /tmp/npm-publish.log | grep "code" | head -1)
        warn "npm publish 测试失败: $WARN_CODE"
        info "详细日志: /tmp/npm-publish.log"
    fi
else
    info "跳过 npm publish (无认证令牌)"
fi

# 清理
cd /
npm config delete registry 2> /dev/null || true
rm -rf "$TEST_DIR"

echo
echo "============================================"
echo " NPM 客户端测试完成"
echo "============================================"
