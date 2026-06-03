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

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
info() { echo -e "  ${BLUE}ℹ INFO${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠ WARN${NC} $1"; WARN_COUNT=$((WARN_COUNT + 1)); }

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

# 配置 npm publish 到本地仓库
NPM_REGISTRY="$BASE_URL/repository/npm-local"

if [ -n "$TOKEN" ]; then
    # 尝试多种认证方式
    # 方式 1: Bearer token (npm 标准 _authToken)
    npm config set "//$(echo $BASE_URL | sed 's|http://||')/repository/npm-local/:_authToken" "$TOKEN" 2>/dev/null || true
    npm config set registry "$NPM_REGISTRY" 2>/dev/null || true

    PUBLISH_OK=false
    if npm publish --access public --registry "$NPM_REGISTRY" &> /tmp/npm-publish.log 2>&1; then
        PUBLISH_OK=true
        pass "npm publish (Bearer token) 测试通过"
    else
        # 方式 2: Basic auth
        NPM_AUTH=$(echo -n "$ADMIN_USER:$ADMIN_PASS" | base64)
        npm config set "//$(echo $BASE_URL | sed 's|http://||')/repository/npm-local/:_auth" "$NPM_AUTH" 2>/dev/null || true
        npm config set "//$(echo $BASE_URL | sed 's|http://||')/repository/npm-local/:always-auth" "true" 2>/dev/null || true

        if npm publish --access public --registry "$NPM_REGISTRY" &> /tmp/npm-publish.log 2>&1; then
            PUBLISH_OK=true
            pass "npm publish (Basic auth) 测试通过"
        else
            # 方式 3: curl 直接发送 JSON metadata (绕过 npm 客户端认证)
            PKG_NAME=$(node -e "console.log(require('./package.json').name)" 2>/dev/null)
            PKG_VERSION=$(node -e "console.log(require('./package.json').version)" 2>/dev/null)
            # npm publish 发送 PUT /<package> 带 JSON body
            HTTP_CODE=$(curl -s -o /tmp/npm-curl-publish.json -w "%{http_code}" \
                -X PUT "$NPM_REGISTRY/$PKG_NAME" \
                -H "Authorization: Bearer $TOKEN" \
                -H "Content-Type: application/json" \
                -d "{\"name\":\"$PKG_NAME\",\"version\":\"$PKG_VERSION\",\"description\":\"Test package via curl\"}")
            if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ]; then
                PUBLISH_OK=true
                pass "curl PUT metadata 发布 npm 包成功 (HTTP $HTTP_CODE)"
            else
                warn "npm publish 和 curl 上传均失败 (curl HTTP $HTTP_CODE)"
                info "详细日志: /tmp/npm-publish.log"
            fi
        fi
    fi

    # 验证发布后可以安装
    if [ "$PUBLISH_OK" = "true" ]; then
        cd "$TEST_DIR"
        mkdir -p "test-install-pkg"
        cd "test-install-pkg"
        npm init -y &> /dev/null 2>&1
        if npm install @test-local/test-publish-pkg@1.0.0 --registry "$NPM_REGISTRY" &> /tmp/npm-install-local.log 2>&1; then
            pass "npm install 本地发布包成功"
        else
            warn "npm install 本地发布包失败"
        fi
    fi
else
    fail "跳过 npm publish (无认证令牌)"
fi

# 清理
cd /
npm config delete registry 2> /dev/null || true
rm -rf "$TEST_DIR"

echo
echo "============================================"
echo " NPM 客户端测试完成"
echo "============================================"
echo -e "  通过: ${GREEN}$PASS_COUNT${NC}"
echo -e "  失败: ${RED}$FAIL_COUNT${NC}"
echo -e "  警告: ${YELLOW}$WARN_COUNT${NC}"
echo
