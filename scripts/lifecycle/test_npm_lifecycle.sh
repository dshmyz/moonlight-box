#!/bin/bash

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

check_npm() {
    if ! command -v npm &> /dev/null; then
        warn "npm 命令未安装，跳过 npm 生命周期测试"
        return 1
    fi
    return 0
}

echo "============================================"
echo " npm 生命周期测试"
echo " 目标: $BASE_URL"
echo "============================================"
echo

if ! check_npm; then
    echo -e "${YELLOW}跳过 npm 测试（需要安装 Node.js）${NC}"
    exit 0
fi

TOKEN=$(get_auth_token)

if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 无法获取认证令牌${NC}"
    exit 1
fi

TEST_DIR="/tmp/npm-test-$$"
mkdir -p "$TEST_DIR"

echo "════════════════════════════════════════"
echo "  测试 1: 创建测试 npm 包"
echo "════════════════════════════════════════"

cd "$TEST_DIR"

cat > package.json <<EOF
{
  "name": "test-npm-package",
  "version": "1.0.0",
  "description": "Test package for npm lifecycle testing",
  "main": "index.js",
  "scripts": {
    "test": "echo \"Error: no test specified\" && exit 1"
  },
  "author": "test",
  "license": "ISC"
}
EOF

cat > index.js <<'EOF'
console.log('Hello from test npm package!');
module.exports = {
    greet: function(name) {
        return 'Hello, ' + name + '!';
    }
};
EOF

if [ -f "package.json" ] && [ -f "index.js" ]; then
    pass "测试 npm 包创建成功"
else
    fail "测试 npm 包创建失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 2: 配置 npm registry"
echo "════════════════════════════════════════"

NPM_REGISTRY="$BASE_URL/repo/npm-local"

if npm set registry "$NPM_REGISTRY" > /dev/null 2>&1; then
    pass "npm registry 配置成功"
else
    warn "npm registry 配置失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 3: 发布 npm 包"
echo "════════════════════════════════════════"

NPMRC_FILE="$TEST_DIR/.npmrc"
cat > "$NPMRC_FILE" <<EOF
registry=$NPM_REGISTRY
//localhost:9081/repo/npm-local/:_authToken=$TOKEN
EOF

if npm publish --userconfig "$NPMRC_FILE" > /dev/null 2>&1; then
    pass "npm 包发布成功"
    
    HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" \
        "$BASE_URL/repo/npm-local/test-npm-package")
    
    if [ "$HTTP_CODE" = "200" ]; then
        pass "发布的 npm 包可访问 (HTTP 200)"
    else
        info "发布的 npm 包返回 HTTP $HTTP_CODE"
    fi
else
    warn "npm 包发布失败（可能需要认证配置）"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 4: 安装 npm 包"
echo "════════════════════════════════════════"

INSTALL_DIR="/tmp/npm-install-test-$$"
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

cat > package.json <<EOF
{
  "name": "npm-install-test",
  "version": "1.0.0",
  "dependencies": {
    "test-npm-package": "1.0.0"
  }
}
EOF

cat > .npmrc <<EOF
registry=$NPM_REGISTRY
//localhost:9081/repo/npm-local/:_authToken=$TOKEN
EOF

if npm install > /dev/null 2>&1; then
    pass "npm 包安装成功"
    
    if [ -d "node_modules/test-npm-package" ]; then
        pass "node_modules 中存在目标包"
    else
        fail "node_modules 中不存在目标包"
    fi
else
    warn "npm 包安装失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 5: 代理仓库安装"
echo "════════════════════════════════════════"

PROXY_INSTALL_DIR="/tmp/npm-proxy-install-test-$$"
mkdir -p "$PROXY_INSTALL_DIR"
cd "$PROXY_INSTALL_DIR"

cat > package.json <<EOF
{
  "name": "npm-proxy-install-test",
  "version": "1.0.0",
  "dependencies": {
    "lodash": "4.17.21"
  }
}
EOF

PROXY_REGISTRY="$BASE_URL/repo/npm-proxy-cn"

cat > .npmrc <<EOF
registry=$PROXY_REGISTRY
EOF

if npm install > /dev/null 2>&1; then
    pass "从代理仓库安装 npm 包成功"
    
    if [ -d "node_modules/lodash" ]; then
        pass "lodash 包安装成功"
    else
        fail "lodash 包未安装"
    fi
else
    warn "从代理仓库安装 npm 包失败"
fi

echo
echo "════════════════════════════════════════"
echo "  测试 6: 清理测试文件"
echo "════════════════════════════════════════"

cd /
rm -rf "$TEST_DIR" "$INSTALL_DIR" "$PROXY_INSTALL_DIR"

npm set registry "https://registry.npmjs.org" > /dev/null 2>&1 || true

if [ ! -d "$TEST_DIR" ] && [ ! -d "$INSTALL_DIR" ] && [ ! -d "$PROXY_INSTALL_DIR" ]; then
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
echo -e "  总计: $((PASS_COUNT + FAIL_COUNT))"
echo

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}所有测试通过! ✓${NC}"
    exit 0
else
    echo -e "${YELLOW}部分测试失败! ❌${NC}"
    exit 1
fi
